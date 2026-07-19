// Command mlx-c-revision-check validates release revision progression.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/tmc/mlx-c-gen/internal/mlxcgen/plan"
	"github.com/tmc/mlx-c-gen/internal/mlxcgen/revision"
	"gopkg.in/yaml.v3"
)

var stableRefRE = regexp.MustCompile(`^v([0-9]+\.[0-9]+\.[0-9]+)$`)

type manifestFile struct {
	MLX plan.MLXPolicy `yaml:"mlx"`
}

type provenanceFile struct {
	ExpectedMLXRef string `json:"expected_mlx_ref"`
	CoreVersion    string `json:"core_version"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mlx-c-revision-check: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	root := flag.String("root", ".", "mlx-c-gen repository root")
	base := flag.String("base", "origin/main", "base commit or branch")
	flag.Parse()
	return check(*root, *base)
}

func check(root, base string) error {
	currentData, err := os.ReadFile(root + "/codegen/manifest.yaml")
	if err != nil {
		return fmt.Errorf("read current manifest: %w", err)
	}
	current, err := parseManifest(currentData)
	if err != nil {
		return err
	}
	baseData, err := gitOutput(root, "show", base+":codegen/manifest.yaml")
	if err != nil {
		return fmt.Errorf("read base manifest: %w", err)
	}
	previous, err := parseManifest([]byte(baseData))
	if err != nil {
		return fmt.Errorf("parse base manifest: %w", err)
	}

	changed, err := generatorContentChanged(root, base)
	if err != nil {
		return err
	}
	changed = changed || previous.MLX.ExpectedGitRef != current.MLX.ExpectedGitRef
	currentCore, err := manifestCore(root, "", current.MLX)
	if err != nil {
		return err
	}
	baseCore, err := manifestCore(root, base, previous.MLX)
	if err != nil {
		return err
	}

	if previous.MLX.ReleaseRevision == 0 {
		if changed || baseCore != currentCore || current.MLX.ReleaseRevision != 1 {
			return fmt.Errorf("legacy manifest migration must preserve MLX content and set release_revision to 1")
		}
		return nil
	}
	return revision.ValidateTransition(
		revision.State{Core: baseCore, Revision: previous.MLX.ReleaseRevision},
		revision.State{Core: currentCore, Revision: current.MLX.ReleaseRevision},
		changed,
	)
}

func parseManifest(data []byte) (manifestFile, error) {
	var manifest manifestFile
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return manifestFile{}, fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.MLX.ExpectedGitRef == "" {
		return manifestFile{}, fmt.Errorf("manifest has empty mlx expected_git_ref")
	}
	if manifest.MLX.ReleaseRevision < 0 {
		return manifestFile{}, fmt.Errorf("manifest release_revision = %d, want a non-negative integer", manifest.MLX.ReleaseRevision)
	}
	return manifest, nil
}

func manifestCore(root, ref string, policy plan.MLXPolicy) (string, error) {
	if match := stableRefRE.FindStringSubmatch(policy.ExpectedGitRef); match != nil {
		return match[1], nil
	}
	data, err := readProvenance(root, ref)
	if err != nil {
		return "", fmt.Errorf("MLX ref %s is not a release tag and has no candidate provenance: %w", policy.ExpectedGitRef, err)
	}
	var provenance provenanceFile
	if err := json.Unmarshal(data, &provenance); err != nil {
		return "", fmt.Errorf("parse candidate provenance: %w", err)
	}
	if provenance.ExpectedMLXRef != policy.ExpectedGitRef || provenance.CoreVersion == "" {
		return "", fmt.Errorf("candidate provenance does not identify MLX ref %s", policy.ExpectedGitRef)
	}
	return provenance.CoreVersion, nil
}

func readProvenance(root, ref string) ([]byte, error) {
	if ref == "" {
		return os.ReadFile(root + "/codegen/candidate-provenance.json")
	}
	data, err := gitOutput(root, "show", ref+":codegen/candidate-provenance.json")
	return []byte(data), err
}

func generatorContentChanged(root, base string) (bool, error) {
	args := []string{
		"diff", "--quiet", base, "--",
		"mlx/c",
		"codegen/custom",
		"codegen/modules",
		"codegen/mlxc-capi.lock.json",
		"codegen/types.yaml",
	}
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	err := cmd.Run()
	if err == nil {
		return false, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, fmt.Errorf("check generator content diff: %w", err)
}

func gitOutput(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
