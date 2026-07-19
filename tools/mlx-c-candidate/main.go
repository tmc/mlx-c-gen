// Command mlx-c-candidate prepares a reviewable generated-source candidate.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tmc/mlx-c-gen/internal/mlxcgen/plan"
	"github.com/tmc/mlx-c-gen/internal/mlxcgen/revision"
)

type candidatePlan struct {
	RequestedMLXRef   string `json:"requested_mlx_ref"`
	ResolvedMLXSHA    string `json:"resolved_mlx_sha"`
	ExpectedMLXRef    string `json:"expected_mlx_ref"`
	BaseMLXSHA        string `json:"base_mlx_sha"`
	CoreVersion       string `json:"core_version"`
	ReleaseRevision   int    `json:"release_revision"`
	GeneratorIdentity string `json:"generator_identity"`
	LibraryIdentity   string `json:"library_identity"`
	CandidateBranch   string `json:"candidate_branch"`
	Preview           bool   `json:"preview"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mlx-c-candidate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	root := flag.String("root", ".", "mlx-c-gen repository root")
	mlxSrc := flag.String("mlx-src", "", "resolved MLX checkout")
	requestedRef := flag.String("mlx-ref", "", "requested MLX branch, tag, or commit")
	generatorChanged := flag.Bool("generator-changed", false, "the candidate changes generator-owned source independently of the MLX ref")
	output := flag.String("output", "", "write the candidate plan as JSON")
	githubOutput := flag.String("github-output", "", "append candidate fields to a GitHub Actions output file")
	flag.Parse()

	if *mlxSrc == "" {
		return fmt.Errorf("missing -mlx-src")
	}
	if *requestedRef == "" {
		return fmt.Errorf("missing -mlx-ref")
	}
	candidate, err := prepare(*root, *mlxSrc, *requestedRef, *generatorChanged)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal candidate plan: %w", err)
	}
	data = append(data, '\n')
	if *output == "" || *output == "-" {
		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("write candidate plan: %w", err)
		}
	} else if err := os.WriteFile(*output, data, 0o666); err != nil {
		return fmt.Errorf("write candidate plan: %w", err)
	}
	if *githubOutput != "" {
		if err := appendGitHubOutput(*githubOutput, candidate); err != nil {
			return err
		}
	}
	return nil
}

func prepare(root, mlxSrc, requestedRef string, generatorChanged bool) (candidatePlan, error) {
	manifestPath := filepath.Join(root, "codegen", "manifest.yaml")
	manifest, err := plan.LoadPath(manifestPath)
	if err != nil {
		return candidatePlan{}, err
	}
	if manifest.MLX.ReleaseRevision < 1 {
		return candidatePlan{}, fmt.Errorf("manifest mlx release_revision = %d, want a positive integer", manifest.MLX.ReleaseRevision)
	}

	targetSHA, err := gitOutput(mlxSrc, "rev-parse", "HEAD^{commit}")
	if err != nil {
		return candidatePlan{}, fmt.Errorf("resolve target MLX checkout: %w", err)
	}
	targetCore, err := revision.CoreVersion(mlxSrc)
	if err != nil {
		return candidatePlan{}, err
	}
	baseSHA, err := gitOutput(mlxSrc, "rev-parse", manifest.MLX.ExpectedGitRef+"^{commit}")
	if err != nil {
		return candidatePlan{}, fmt.Errorf("resolve manifest MLX ref %s: %w", manifest.MLX.ExpectedGitRef, err)
	}
	baseHeader, err := gitOutput(mlxSrc, "show", baseSHA+":mlx/version.h")
	if err != nil {
		return candidatePlan{}, fmt.Errorf("read base MLX version: %w", err)
	}
	baseCore, err := revision.ParseCoreVersion(strings.NewReader(baseHeader))
	if err != nil {
		return candidatePlan{}, err
	}
	next, err := revision.Next(
		revision.State{Core: baseCore, Revision: manifest.MLX.ReleaseRevision},
		targetCore,
		generatorChanged || baseSHA != targetSHA,
	)
	if err != nil {
		return candidatePlan{}, err
	}

	stableRef := "v" + targetCore
	stableSHA, stableErr := gitOutput(mlxSrc, "rev-parse", stableRef+"^{commit}")
	stable := stableErr == nil && stableSHA == targetSHA
	expectedRef := targetSHA
	previewSHA := targetSHA
	if stable {
		expectedRef = stableRef
		previewSHA = ""
	}
	identity, err := revision.NewIdentity(next, previewSHA)
	if err != nil {
		return candidatePlan{}, err
	}

	if err := replaceManifestPolicy(manifestPath, manifest.MLX, expectedRef, next.Revision); err != nil {
		return candidatePlan{}, err
	}
	if err := replaceCMakeRef(filepath.Join(root, "CMakeLists.txt"), manifest.MLX.ExpectedGitRef, expectedRef); err != nil {
		return candidatePlan{}, err
	}
	updated, err := plan.LoadPath(manifestPath)
	if err != nil {
		return candidatePlan{}, err
	}
	if err := updated.CheckCMakeMLXRef(root); err != nil {
		return candidatePlan{}, err
	}

	return candidatePlan{
		RequestedMLXRef:   requestedRef,
		ResolvedMLXSHA:    targetSHA,
		ExpectedMLXRef:    expectedRef,
		BaseMLXSHA:        baseSHA,
		CoreVersion:       targetCore,
		ReleaseRevision:   next.Revision,
		GeneratorIdentity: identity.GeneratorTag(),
		LibraryIdentity:   identity.LibraryTag(),
		CandidateBranch:   identity.CandidateBranch(),
		Preview:           !stable,
	}, nil
}

func replaceManifestPolicy(path string, old plan.MLXPolicy, ref string, revision int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	data, err = replaceOnce(data,
		[]byte("  expected_git_ref: "+old.ExpectedGitRef),
		[]byte("  expected_git_ref: "+ref),
	)
	if err != nil {
		return fmt.Errorf("update manifest MLX ref: %w", err)
	}
	data, err = replaceOnce(data,
		[]byte(fmt.Sprintf("  release_revision: %d", old.ReleaseRevision)),
		[]byte(fmt.Sprintf("  release_revision: %d", revision)),
	)
	if err != nil {
		return fmt.Errorf("update manifest release revision: %w", err)
	}
	if err := os.WriteFile(path, data, 0o666); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func replaceCMakeRef(path, old, ref string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read CMakeLists.txt: %w", err)
	}
	data, err = replaceOnce(data, []byte("GIT_TAG "+old), []byte("GIT_TAG "+ref))
	if err != nil {
		return fmt.Errorf("update CMake MLX ref: %w", err)
	}
	if err := os.WriteFile(path, data, 0o666); err != nil {
		return fmt.Errorf("write CMakeLists.txt: %w", err)
	}
	return nil
}

func replaceOnce(data, old, new []byte) ([]byte, error) {
	if bytes.Count(data, old) != 1 {
		return nil, fmt.Errorf("found %d occurrences of %q, want 1", bytes.Count(data, old), old)
	}
	return bytes.Replace(data, old, new, 1), nil
}

func appendGitHubOutput(path string, candidate candidatePlan) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666)
	if err != nil {
		return fmt.Errorf("open GitHub output: %w", err)
	}
	defer f.Close()
	for _, item := range []struct{ key, value string }{
		{"candidate_branch", candidate.CandidateBranch},
		{"core_version", candidate.CoreVersion},
		{"expected_mlx_ref", candidate.ExpectedMLXRef},
		{"generator_identity", candidate.GeneratorIdentity},
		{"library_identity", candidate.LibraryIdentity},
		{"release_revision", fmt.Sprint(candidate.ReleaseRevision)},
		{"resolved_mlx_sha", candidate.ResolvedMLXSHA},
	} {
		if _, err := fmt.Fprintf(f, "%s=%s\n", item.key, item.value); err != nil {
			return fmt.Errorf("write GitHub output: %w", err)
		}
	}
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
