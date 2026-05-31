package regenreport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/plan"
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

func TestReportModules(t *testing.T) {
	got := reportModules(plan.Manifest{
		Headers: []plan.HeaderMapping{
			{Name: "ops", Headers: []string{"mlx/ops.h", "mlx/einsum.h"}},
			{Name: "fft", Headers: []string{"mlx/fft.h"}},
		},
	})
	if len(got) != 2 {
		t.Fatalf("modules = %#v, want two", got)
	}
	if got[0].Name != "ops" ||
		got[0].Headers[0] != "mlx/ops.h" ||
		got[0].Outputs[0] != "mlx/c/ops.cpp" ||
		got[0].Outputs[1] != "mlx/c/ops.h" {
		t.Fatalf("first module = %#v", got[0])
	}
}

func TestGeneratorArgsIncludesCompileCommands(t *testing.T) {
	args := generatorArgs(Options{
		MLXSrc:              "/tmp/mlx",
		CompileCommandsPath: "/tmp/build/compile_commands.json",
		ASTCacheDir:         "/tmp/cache",
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
		"--ast-cache",
		"/tmp/cache",
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

func TestGeneratorArgsCanDisableASTCache(t *testing.T) {
	args := generatorArgs(Options{
		MLXSrc:     "/tmp/mlx",
		Generator:  []string{"go", "run", "./tools/mlx-c-gen"},
		NoASTCache: true,
	}, "/tmp/out", "/tmp/meta.yaml")
	want := "--no-ast-cache"
	for _, arg := range args {
		if arg == want {
			return
		}
	}
	t.Fatalf("args = %#v, missing %s", args, want)
}

func TestReadMetadataDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.yaml")
	write(t, filepath.Dir(path), filepath.Base(path), `functions: []
enums: []
diagnostics:
  - code: skip_variant_mapping
    message: skipped by variant mapping
    file: mlx/metal.h
    line: 22
    col: 5
`)

	diagnostics, err := readMetadataDiagnostics(path)
	if err != nil {
		t.Fatalf("readMetadataDiagnostics: %v", err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one", diagnostics)
	}
	got := diagnostics[0]
	want := Diagnostic{
		Code:    "skip_variant_mapping",
		Message: "skipped by variant mapping",
		File:    "mlx/metal.h",
		Line:    22,
		Col:     5,
	}
	if got != want {
		t.Fatalf("diagnostic = %#v, want %#v", got, want)
	}
}

func TestReadMetadataDiagnosticsAllowsMissingMetadata(t *testing.T) {
	diagnostics, err := readMetadataDiagnostics(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("readMetadataDiagnostics missing: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
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
