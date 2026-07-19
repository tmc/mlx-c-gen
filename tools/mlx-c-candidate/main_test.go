package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareStableAndPreview(t *testing.T) {
	mlx := makeMLXRepo(t)
	base := git(t, mlx, "rev-parse", "HEAD")
	git(t, mlx, "tag", "v0.32.0")

	t.Run("stable", func(t *testing.T) {
		root := makeGeneratorRepo(t, "v0.32.0")
		got, err := prepare(root, mlx, "v0.32.0", false)
		if err != nil {
			t.Fatal(err)
		}
		if got.GeneratorIdentity != "mlx-v0.32.0-rev1" || got.CandidateBranch != "build/mlx-v0.32.0-rev1" || got.Preview {
			t.Fatalf("candidate = %#v", got)
		}
	})

	write(t, mlx, "change.txt", "preview\n")
	git(t, mlx, "add", "change.txt")
	git(t, mlx, "commit", "-m", "preview")
	preview := git(t, mlx, "rev-parse", "HEAD")
	t.Run("preview", func(t *testing.T) {
		root := makeGeneratorRepo(t, "v0.32.0")
		got, err := prepare(root, mlx, "topic", false)
		if err != nil {
			t.Fatal(err)
		}
		want := "mlx-v0.32.0-rev2-dev." + preview[:12]
		if got.GeneratorIdentity != want || got.ReleaseRevision != 2 || !got.Preview || got.BaseMLXSHA != base {
			t.Fatalf("candidate = %#v, want identity %s", got, want)
		}
		manifest := read(t, root, "codegen/manifest.yaml")
		if !strings.Contains(manifest, "expected_git_ref: "+preview) || !strings.Contains(manifest, "release_revision: 2") {
			t.Fatalf("manifest = %s", manifest)
		}
	})
}

func TestPublishCandidatePushesBranchAndCreatesPR(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	git(t, filepath.Dir(remote), "init", "--bare", remote)
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")
	git(t, repo, "remote", "add", "origin", remote)
	write(t, repo, "CMakeLists.txt", "base\n")
	write(t, repo, "codegen/manifest.yaml", "base\n")
	write(t, repo, "mlx/c/ops.h", "base\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	write(t, repo, "codegen/manifest.yaml", "candidate\n")
	write(t, repo, "pr-body.md", "candidate body\n")

	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh.log")
	write(t, bin, "gh", `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >>"$GH_LOG"
if [[ "$1 $2" == "pr create" ]]; then
  echo https://example.invalid/pr/1
fi
`)
	if err := os.Chmod(filepath.Join(bin, "gh"), 0o755); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs(filepath.Join("..", "mlx-c-publish-candidate"))
	if err != nil {
		t.Fatal(err)
	}
	branch := "build/mlx-v0.32.0-rev1"
	cmd := exec.Command("bash", script, branch, "main", "mlx-v0.32.0-rev1", "pr-body.md")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GH_LOG="+logPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("publish candidate: %v\n%s", err, out)
	}
	got := git(t, repo, "--git-dir", remote, "rev-parse", "refs/heads/"+branch)
	if got == "" {
		t.Fatal("candidate branch was not pushed")
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "pr create --base main --head "+branch) {
		t.Fatalf("gh log = %s", logData)
	}
}

func makeMLXRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Test")
	write(t, root, "mlx/version.h", `
#define MLX_VERSION_MAJOR 0
#define MLX_VERSION_MINOR 32
#define MLX_VERSION_PATCH 0
`)
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "base")
	return root
}

func makeGeneratorRepo(t *testing.T, ref string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "CMakeLists.txt", "GIT_TAG "+ref+")\n")
	write(t, root, "codegen/manifest.yaml", `
schema_version: 1
mlx:
  expected_git_ref: `+ref+`
  release_revision: 1
headers:
  - name: ops
    headers:
      - mlx/ops.h
standalone:
  - vector
`)
	return root
}

func write(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o666); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
