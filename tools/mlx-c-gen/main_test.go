package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/ir"
)

func TestPrepareOutputDirCreatesPrivateDir(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "mlx", "c")
	if err := prepareOutputDir(outDir, false); err != nil {
		t.Fatalf("prepareOutputDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "private")); err != nil {
		t.Fatalf("stat private dir: %v", err)
	}
}

func TestPrepareOutputDirDryRunDoesNotCreate(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "mlx", "c")
	if err := prepareOutputDir(outDir, true); err != nil {
		t.Fatalf("prepareOutputDir dry run: %v", err)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("stat output dir after dry run: %v, want not exist", err)
	}
}

func TestPrepareOutputDirRejectsEmptyOutputDir(t *testing.T) {
	if err := prepareOutputDir("", false); err == nil || !strings.Contains(err.Error(), "missing output directory") {
		t.Fatalf("prepareOutputDir empty = %v, want missing output directory", err)
	}
}

func TestPrepareOutputDirReportsCreateError(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(outDir, []byte("not a directory"), 0o666); err != nil {
		t.Fatalf("write output path file: %v", err)
	}
	err := prepareOutputDir(outDir, false)
	if err == nil || !strings.Contains(err.Error(), "create output directory") {
		t.Fatalf("prepareOutputDir file path = %v, want create output directory error", err)
	}
}

func TestParseCheckOptions(t *testing.T) {
	opts, err := parseCheckOptions([]string{
		"--root", "/repo",
		"--mlx-src", "/mlx",
		"--manifest", "/repo/codegen/manifest.yaml",
		"--custom-dir", "/repo/codegen/custom",
		"--types", "/repo/codegen/types.yaml",
		"--compile-commands", "/build/compile_commands.json",
		"--inventory", "/repo/codegen/generated-files.txt",
		"--lock", "/repo/codegen/mlxc-capi.lock.json",
		"--lock-tu", "/repo/codegen/lock.c",
		"--report", "/tmp/report.json",
		"--work-dir", "/tmp/work",
		"--ast-cache", "/tmp/cache",
		"--generator", "go run ./tools/mlx-c-gen",
		"--nm", "llvm-nm",
		"--symbol", "mlxc=/tmp/libmlxc.dylib",
		"--no-format",
		"--keep-work",
		"--strict-generated",
	})
	if err != nil {
		t.Fatalf("parseCheckOptions: %v", err)
	}
	got := opts.Options
	if got.RepoRoot != "/repo" ||
		got.MLXSrc != "/mlx" ||
		got.ManifestPath != "/repo/codegen/manifest.yaml" ||
		got.CustomDir != "/repo/codegen/custom" ||
		got.TypePolicyPath != "/repo/codegen/types.yaml" ||
		got.CompileCommandsPath != "/build/compile_commands.json" ||
		got.InventoryPath != "/repo/codegen/generated-files.txt" ||
		opts.LockPath != "/repo/codegen/mlxc-capi.lock.json" ||
		opts.LockTUPath != "/repo/codegen/lock.c" ||
		opts.ReportPath != "/tmp/report.json" ||
		opts.NM != "llvm-nm" ||
		got.WorkDir != "/tmp/work" ||
		got.ASTCacheDir != "/tmp/cache" ||
		got.NoASTCache ||
		!got.NoFormat ||
		!got.KeepWork ||
		!opts.StrictGenerated {
		t.Fatalf("options = %#v check = %#v", got, opts)
	}
	wantGenerator := []string{"go", "run", "./tools/mlx-c-gen"}
	if !reflect.DeepEqual(got.Generator, wantGenerator) {
		t.Fatalf("generator = %#v, want %#v", got.Generator, wantGenerator)
	}
	if len(opts.Symbols) != 1 || opts.Symbols[0].Target != "mlxc" || opts.Symbols[0].Path != "/tmp/libmlxc.dylib" {
		t.Fatalf("symbols = %#v", opts.Symbols)
	}
}

func TestParseOptionsFromArgs(t *testing.T) {
	opts, err := parseOptionsFromArgs([]string{
		"--root", "/repo",
		"--mlx-src", "/mlx",
		"--manifest", "/repo/codegen/manifest.yaml",
		"--custom-dir", "/repo/codegen/custom",
		"--types", "/repo/codegen/types.yaml",
		"--compile-commands", "/build/compile_commands.json",
		"--ast-cache", "/tmp/cache",
		"--out", "/tmp/ir.json",
		"--report", "/tmp/report.json",
	})
	if err != nil {
		t.Fatalf("parseOptionsFromArgs: %v", err)
	}
	if opts.RepoRoot != "/repo" ||
		opts.MLXSrc != "/mlx" ||
		opts.ManifestPath != "/repo/codegen/manifest.yaml" ||
		opts.CustomDir != "/repo/codegen/custom" ||
		opts.TypePolicyPath != "/repo/codegen/types.yaml" ||
		opts.CompileCommandsPath != "/build/compile_commands.json" ||
		opts.ASTCacheDir != "/tmp/cache" ||
		opts.NoASTCache ||
		opts.OutPath != "/tmp/ir.json" ||
		opts.ReportPath != "/tmp/report.json" {
		t.Fatalf("options = %#v", opts)
	}
}

func TestParseOptionsFromArgsDefaults(t *testing.T) {
	t.Setenv("MLX_C_AST_CACHE", "/tmp/mlx-c-default-cache")
	opts, err := parseOptionsFromArgs(nil)
	if err != nil {
		t.Fatalf("parseOptionsFromArgs defaults: %v", err)
	}
	if opts.RepoRoot != "." ||
		opts.MLXSrc != "" ||
		opts.TypePolicyPath != filepath.Join(".", "codegen", "types.yaml") ||
		opts.ASTCacheDir != "/tmp/mlx-c-default-cache" ||
		opts.NoASTCache ||
		opts.OutPath != "-" ||
		opts.ReportPath != "" {
		t.Fatalf("defaults = %#v", opts)
	}
}

func TestParseOptionsFromArgsNoASTCache(t *testing.T) {
	t.Setenv("MLX_C_AST_CACHE", "/tmp/mlx-c-default-cache")
	opts, err := parseOptionsFromArgs([]string{"--no-ast-cache"})
	if err != nil {
		t.Fatalf("parseOptionsFromArgs no cache: %v", err)
	}
	if opts.ASTCacheDir != "" || !opts.NoASTCache {
		t.Fatalf("cache options = %#v", opts)
	}
}

func TestParseCheckOptionsDefaults(t *testing.T) {
	t.Setenv("MLX_C_AST_CACHE", "/tmp/mlx-c-default-cache")
	opts, err := parseCheckOptions(nil)
	if err != nil {
		t.Fatalf("parseCheckOptions defaults: %v", err)
	}
	got := opts.Options
	if got.RepoRoot != "." ||
		got.InventoryPath != "codegen/generated-files.txt" ||
		got.MLXSrc != "" ||
		got.ASTCacheDir != "/tmp/mlx-c-default-cache" ||
		got.NoASTCache ||
		got.NoFormat ||
		got.KeepWork ||
		opts.LockPath != "codegen/mlxc-capi.lock.json" ||
		opts.LockTUPath != "codegen/lock.c" ||
		opts.ReportPath != "" ||
		opts.NM != "nm" ||
		opts.StrictGenerated {
		t.Fatalf("defaults = %#v check = %#v", got, opts)
	}
	wantGenerator := strings.Fields(defaultGeneratorCommand())
	if !reflect.DeepEqual(got.Generator, wantGenerator) {
		t.Fatalf("generator = %#v, want %#v", got.Generator, wantGenerator)
	}
}

func TestParseCheckOptionsSynthesisAliases(t *testing.T) {
	t.Setenv("MLX_C_AST_CACHE", "/tmp/mlx-c-default-cache")
	opts, err := parseCheckOptions([]string{
		"--output-root", "/repo",
		"--generated-files", "/repo/codegen/generated-files.txt",
		"--manifest", "/repo/codegen/manifest.yaml",
		"--custom-dir", "/repo/codegen/custom",
		"--report", "/repo/build/report.json",
	})
	if err != nil {
		t.Fatalf("parseCheckOptions aliases: %v", err)
	}
	if opts.Options.RepoRoot != "/repo" ||
		opts.Options.InventoryPath != "/repo/codegen/generated-files.txt" ||
		opts.Options.ManifestPath != "/repo/codegen/manifest.yaml" ||
		opts.Options.CustomDir != "/repo/codegen/custom" ||
		opts.ReportPath != "/repo/build/report.json" {
		t.Fatalf("options = %#v check = %#v", opts.Options, opts)
	}
}

func TestParseCheckOptionsNoASTCache(t *testing.T) {
	t.Setenv("MLX_C_AST_CACHE", "/tmp/mlx-c-default-cache")
	opts, err := parseCheckOptions([]string{"--no-ast-cache"})
	if err != nil {
		t.Fatalf("parseCheckOptions no ast cache: %v", err)
	}
	if opts.Options.ASTCacheDir != "" || !opts.Options.NoASTCache {
		t.Fatalf("cache options = %#v, want disabled", opts.Options)
	}
}

func TestResolveASTCacheDir(t *testing.T) {
	t.Setenv("MLX_C_AST_CACHE", "/tmp/mlx-c-env-cache")
	if got := resolveASTCacheDir("", false); got != "/tmp/mlx-c-env-cache" {
		t.Fatalf("default cache dir = %q, want env", got)
	}
	if got := resolveASTCacheDir("/tmp/mlx-c-explicit-cache", false); got != "/tmp/mlx-c-explicit-cache" {
		t.Fatalf("explicit cache dir = %q, want explicit", got)
	}
	if got := resolveASTCacheDir("/tmp/mlx-c-explicit-cache", true); got != "" {
		t.Fatalf("disabled cache dir = %q, want empty", got)
	}
}

func TestParseTypePolicyReportsMissingTypes(t *testing.T) {
	summary, missing, diagnostics, err := parseTypePolicy(parseOptions{
		RepoRoot:       ".",
		TypePolicyPath: "",
	}, ir.Result{
		Functions: []ir.FuncDecl{{
			ID:        "ops|mlx/ops.h|mlx::core|bad|Missing(array)",
			Module:    "ops",
			Header:    "mlx/ops.h",
			Namespace: "mlx::core",
			Name:      "bad",
			Return:    "Missing",
			Params:    []ir.Param{{Name: "x", Type: "array"}},
			Loc:       ir.SourceLoc{File: "mlx/ops.h", Line: 7, Col: 2},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.MissingTypes != 1 || len(missing) != 1 || len(diagnostics) != 1 {
		t.Fatalf("summary=%#v missing=%#v diagnostics=%#v", summary, missing, diagnostics)
	}
	if diagnostics[0].Code != "missing_type_policy" ||
		diagnostics[0].File != "mlx/ops.h" ||
		diagnostics[0].Line != 7 ||
		diagnostics[0].Col != 2 {
		t.Fatalf("diagnostic = %#v", diagnostics[0])
	}
}

func TestRepoPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	relative := filepath.Join("codegen", "mlxc-capi.lock.json")
	if got := repoPath(root, relative); got != filepath.Join(root, relative) {
		t.Fatalf("repoPath relative = %q", got)
	}
	absolute := filepath.Join(root, "absolute")
	if got := repoPath(root, absolute); got != absolute {
		t.Fatalf("repoPath absolute = %q", got)
	}
}

func TestWriteCheckReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report", "check.json")
	if err := writeCheckReport(path, []byte("report\n")); err != nil {
		t.Fatalf("writeCheckReport: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if string(data) != "report\n" {
		t.Fatalf("report = %q, want report", string(data))
	}
}

func TestDiscoverMLXSourceUsesExplicitPath(t *testing.T) {
	got, err := discoverMLXSource("/tmp/mlx")
	if err != nil {
		t.Fatalf("discoverMLXSource explicit: %v", err)
	}
	if got != "/tmp/mlx" {
		t.Fatalf("discoverMLXSource explicit = %q", got)
	}
}

func TestDiscoverMLXSourceFindsBuildTree(t *testing.T) {
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	want := filepath.Join("build-Release", "_deps", "mlx-src")
	if err := os.MkdirAll(want, 0o777); err != nil {
		t.Fatal(err)
	}
	got, err := discoverMLXSource("")
	if err != nil {
		t.Fatalf("discoverMLXSource fallback: %v", err)
	}
	if got != want {
		t.Fatalf("discoverMLXSource fallback = %q, want %q", got, want)
	}
}
