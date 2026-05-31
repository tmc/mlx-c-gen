package regenreport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompare(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "mlx", "c")
	write(t, root, "mlx/c/ops.h", "same\n")
	write(t, out, "ops.h", "same\n")
	write(t, root, "mlx/c/ops.cpp", "old\n")
	write(t, out, "ops.cpp", "new\n")
	write(t, root, "mlx/c/missing_generated.h", "checked\n")
	write(t, out, "extra.h", "extra\n")

	report, err := Compare(root, out, []string{
		"mlx/c/ops.h",
		"mlx/c/ops.cpp",
		"mlx/c/missing_generated.h",
		"mlx/c/missing_checked_in.h",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Equal != 1 ||
		report.Summary.Different != 1 ||
		report.Summary.MissingGenerated != 1 ||
		report.Summary.MissingCheckedIn != 1 {
		t.Fatalf("summary = %#v", report.Summary)
	}
	if len(report.GeneratedOnly) != 1 || report.GeneratedOnly[0] != "mlx/c/extra.h" {
		t.Fatalf("generated only = %#v", report.GeneratedOnly)
	}
	if report.Clean() {
		t.Fatalf("dirty report marked clean")
	}
}

func TestCleanReport(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "mlx", "c")
	write(t, root, "mlx/c/ops.h", "same\n")
	write(t, out, "ops.h", "same\n")

	report, err := Compare(root, out, []string{"mlx/c/ops.h"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Clean() {
		t.Fatalf("clean report marked dirty: %#v", report)
	}
}

func TestGeneratorArgsIncludesCompileCommands(t *testing.T) {
	args := generatorArgs(Options{
		MLXSrc:              "/tmp/mlx",
		CompileCommandsPath: "/tmp/build/compile_commands.json",
		Generator:           []string{"go", "run", "./tools/mlx-c-gen"},
		NoFormat:            true,
	}, "/tmp/out", "/tmp/meta.yaml")
	want := []string{
		"run",
		"./tools/mlx-c-gen",
		"--mlx-src",
		"/tmp/mlx",
		"--output-dir",
		"/tmp/out",
		"--metadata",
		"/tmp/meta.yaml",
		"--compile-commands",
		"/tmp/build/compile_commands.json",
		"--no-format",
	}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
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
