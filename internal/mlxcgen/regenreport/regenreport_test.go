package regenreport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/inventory"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/ir"
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

func TestReportManifest(t *testing.T) {
	got := reportManifest(plan.Manifest{
		SchemaVersion: plan.SchemaVersion,
		MLX: plan.MLXPolicy{
			ExpectedGitRef: "v0.31.2",
		},
		Report: plan.ReportPolicy{
			RequireCleanGenerated: true,
			RequireAPILock:        true,
			IncludeInventory:      true,
		},
		GeneratedMarkers: plan.GeneratedMarkerPolicy{
			ForbidVolatileData: true,
		},
	})
	if got.SchemaVersion != plan.SchemaVersion ||
		got.MLX.ExpectedGitRef != "v0.31.2" ||
		!got.Report.RequireCleanGenerated ||
		!got.Report.RequireAPILock ||
		!got.Report.IncludeInventory ||
		!got.GeneratedMarkers.ForbidVolatileData {
		t.Fatalf("manifest = %#v", got)
	}
}

func TestReportInventory(t *testing.T) {
	got := reportInventory([]inventory.Entry{
		{Kind: "handwritten_runtime", Target: "jacclc", Path: "mlx/c/jaccl.cpp"},
		{Kind: "generated_header_api", Target: "mlxc", Path: "mlx/c/ops.h"},
	})
	want := []Inventory{
		{Kind: "handwritten_runtime", Target: "jacclc", Path: "mlx/c/jaccl.cpp"},
		{Kind: "generated_header_api", Target: "mlxc", Path: "mlx/c/ops.h"},
	}
	if len(got) != len(want) {
		t.Fatalf("inventory = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("inventory = %#v, want %#v", got, want)
		}
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

func TestRepoPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	relative := filepath.Join("codegen", "generated-files.txt")
	if got := repoPath(root, relative); got != filepath.Join(root, relative) {
		t.Fatalf("repoPath relative = %q", got)
	}
	absolute := filepath.Join(root, "absolute")
	if got := repoPath(root, absolute); got != absolute {
		t.Fatalf("repoPath absolute = %q", got)
	}
}

func TestReadMetadataDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.yaml")
	write(t, filepath.Dir(path), filepath.Base(path), `ir:
  functions:
    - id: ops|mlx/ops.h|mlx::core|add|array(array, array)
      module: ops
      header: mlx/ops.h
      namespace: mlx::core
      name: add
      return: array
functions: []
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

	metadata, err := readMetadata(path)
	if err != nil {
		t.Fatalf("readMetadata: %v", err)
	}
	wantIR := ir.Result{
		Functions: []ir.FuncDecl{{
			ID:        "ops|mlx/ops.h|mlx::core|add|array(array, array)",
			Module:    "ops",
			Header:    "mlx/ops.h",
			Namespace: "mlx::core",
			Name:      "add",
			Return:    "array",
		}},
	}
	if len(metadata.IR.Functions) != 1 ||
		metadata.IR.Functions[0].ID != wantIR.Functions[0].ID ||
		metadata.IR.Functions[0].Module != wantIR.Functions[0].Module ||
		metadata.IR.Functions[0].Header != wantIR.Functions[0].Header ||
		metadata.IR.Functions[0].Namespace != wantIR.Functions[0].Namespace ||
		metadata.IR.Functions[0].Name != wantIR.Functions[0].Name ||
		metadata.IR.Functions[0].Return != wantIR.Functions[0].Return {
		t.Fatalf("IR = %#v, want %#v", metadata.IR, wantIR)
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
