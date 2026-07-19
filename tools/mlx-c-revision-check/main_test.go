package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCandidateReport(t *testing.T) {
	tests := []struct {
		name       string
		matches    bool
		wrongHash  bool
		omitReport bool
		wantError  bool
	}{
		{"match", true, false, false, false},
		{"mismatch", false, false, false, true},
		{"wrong digest", true, true, false, true},
		{"missing report", true, false, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			report := []byte(fmt.Sprintf("{\"mlx_ref\":{\"matches_expected\":%t}}\n", tt.matches))
			digest := fmt.Sprintf("%x", sha256.Sum256(report))
			if tt.wrongHash {
				digest = strings.Repeat("0", 64)
			}
			provenance := fmt.Sprintf("{\"input_digests\":{\"regen_report_sha256\":%q}}\n", digest)
			writeFile(t, root, "codegen/candidate-provenance.json", provenance)
			if !tt.omitReport {
				writeFile(t, root, "codegen/candidate-regen-report.json", string(report))
			}
			err := checkCandidateReport(root)
			if tt.wantError && err == nil {
				t.Fatal("checkCandidateReport succeeded")
			}
			if !tt.wantError && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckRevisionProgression(t *testing.T) {
	tests := []struct {
		name        string
		baseRef     string
		baseRev     int
		currentRef  string
		currentRev  int
		changeFiles bool
		wantError   bool
	}{
		{"legacy migration", "v0.32.0", 0, "v0.32.0", 1, false, false},
		{"reuse", "v0.32.0", 1, "v0.32.0", 1, true, true},
		{"increment", "v0.32.0", 1, "v0.32.0", 2, true, false},
		{"decrement", "v0.32.0", 2, "v0.32.0", 1, false, true},
		{"new core resets", "v0.32.0", 4, "v0.33.0", 1, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, base := makeRepo(t, tt.baseRef, tt.baseRev)
			writeManifest(t, root, tt.currentRef, tt.currentRev)
			if tt.changeFiles {
				writeFile(t, root, "mlx/c/ops.h", "changed\n")
			}
			err := check(root, base)
			if tt.wantError && err == nil {
				t.Fatal("check succeeded")
			}
			if !tt.wantError && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func makeRepo(t *testing.T, ref string, revision int) (string, string) {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	writeManifest(t, root, ref, revision)
	writeFile(t, root, "mlx/c/ops.h", "base\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	return root, runGit(t, root, "rev-parse", "HEAD")
}

func writeManifest(t *testing.T, root, ref string, revision int) {
	t.Helper()
	revisionLine := ""
	if revision > 0 {
		revisionLine = fmt.Sprintf("  release_revision: %d\n", revision)
	}
	writeFile(t, root, "codegen/manifest.yaml", "mlx:\n  expected_git_ref: "+ref+"\n"+revisionLine)
}

func writeFile(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o666); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
