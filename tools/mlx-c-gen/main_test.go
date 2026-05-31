package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/generators"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/ir"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/parser"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/plan"
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

func TestNormalizedParseCommandArgs(t *testing.T) {
	got := normalizedParseCommandArgs([]string{
		"--manifest=codegen/manifest.yaml",
		"--out",
		"/tmp/ir1.json",
		"--report=/tmp/report1.json",
		"--mlx-src",
		"/repo/mlx",
	})
	want := []string{
		"--manifest=codegen/manifest.yaml",
		"--out",
		"<path>",
		"--report=<path>",
		"--mlx-src",
		"/repo/mlx",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized args = %#v, want %#v", got, want)
	}
}

func TestGenerateOutputDir(t *testing.T) {
	tests := []struct {
		name       string
		outputDir  string
		outputRoot string
		want       string
	}{
		{name: "default", outputRoot: ".", want: filepath.Join("mlx", "c")},
		{name: "root", outputRoot: "/repo", want: filepath.Join("/repo", "mlx", "c")},
		{name: "empty root", want: filepath.Join("mlx", "c")},
		{name: "explicit dir", outputDir: "/tmp/out", outputRoot: "/repo", want: "/tmp/out"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generateOutputDir(tt.outputDir, tt.outputRoot); got != tt.want {
				t.Fatalf("generateOutputDir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizedGenerateCommandArgs(t *testing.T) {
	got := normalizedGenerateCommandArgs([]string{
		"--manifest=codegen/manifest.yaml",
		"--output-root",
		".",
		"--report=/tmp/report.json",
	})
	want := []string{
		"--manifest=codegen/manifest.yaml",
		"--output-root",
		".",
		"--report=<path>",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized args = %#v, want %#v", got, want)
	}
}

func TestNewGenerateReport(t *testing.T) {
	report := newGenerateReport(generateReportOptions{
		Args:         []string{"--output-root", ".", "--report", "/tmp/report.json"},
		OutputRoot:   ".",
		OutputDir:    filepath.Join("mlx", "c"),
		MLXSrc:       "/missing/mlx",
		ManifestPath: "codegen/manifest.yaml",
		Manifest: plan.Manifest{
			SchemaVersion: plan.SchemaVersion,
			Headers: []plan.HeaderMapping{{
				Name:    "ops",
				Headers: []string{"mlx/ops.h"},
			}},
			Standalone: []string{"vector"},
		},
		NoFormat: true,
	})
	if report.SchemaVersion != 1 ||
		report.OutputRoot != "." ||
		report.OutputDir != filepath.Join("mlx", "c") ||
		len(report.Modules) != 1 ||
		len(report.GeneratedFiles) != 5 ||
		report.Summary.HeaderModules != 1 ||
		report.Summary.Standalone != 1 ||
		!report.Summary.NoFormat {
		t.Fatalf("report = %#v", report)
	}
	wantCommand := []string{"mlx-c-gen", "generate", "--output-root", ".", "--report", "<path>"}
	if !reflect.DeepEqual(report.Command, wantCommand) {
		t.Fatalf("command = %#v, want %#v", report.Command, wantCommand)
	}
}

func TestNewGenerateReportNormalizesVolatilePaths(t *testing.T) {
	report := newGenerateReport(generateReportOptions{
		Args: []string{
			"--output-root",
			"/tmp/root",
			"--metadata=/tmp/metadata.yaml",
			"--report",
			"/tmp/report.json",
		},
		OutputRoot:   "/tmp/root",
		OutputDir:    "/tmp/root/mlx/c",
		MetadataPath: "/tmp/metadata.yaml",
		MLXSrc:       "/missing/mlx",
		Manifest: plan.Manifest{
			SchemaVersion: plan.SchemaVersion,
			Headers: []plan.HeaderMapping{{
				Name:    "ops",
				Headers: []string{"mlx/ops.h"},
			}},
			Standalone: []string{"vector"},
		},
	})
	if report.OutputRoot != "<path>" ||
		report.OutputDir != "<path>" ||
		report.MetadataPath != "<path>" {
		t.Fatalf("paths = root %q dir %q metadata %q", report.OutputRoot, report.OutputDir, report.MetadataPath)
	}
	wantCommand := []string{
		"mlx-c-gen",
		"generate",
		"--output-root",
		"<path>",
		"--metadata=<path>",
		"--report",
		"<path>",
	}
	if !reflect.DeepEqual(report.Command, wantCommand) {
		t.Fatalf("command = %#v, want %#v", report.Command, wantCommand)
	}
}

func TestParseVariantDecisions(t *testing.T) {
	base := ""
	axis := "axis"
	decisions, summary := parseVariantDecisions(plan.Manifest{
		VariantMappings: map[string]map[string][]plan.Variant{
			"mlx_core": {
				"export_to_dot": {
					{Signature: "void(std::ostream&, NodeNamer, std::vector<array>)", Suffix: &base},
				},
				"sum": {
					{Signature: "array(array, int, bool, StreamOrDevice)", Suffix: &axis},
					{Signature: "array(array, bool, StreamOrDevice)", Suffix: &base},
					{Signature: "array(array, StreamOrDevice)", Skip: true},
				},
			},
			"mlx_core_fft": {
				"fftn": {
					{Signature: "array(array, std::vector<int>, StreamOrDevice)", Suffix: &base},
				},
			},
		},
	})
	want := []parseDecision{
		{
			Source:    "variant_mapping",
			Namespace: "mlx_core",
			Function:  "export_to_dot",
			Signature: "void(std::ostream&, NodeNamer, std::vector<array>)",
			Action:    "hook",
			CName:     "mlx_export_to_dot",
			Reason:    "custom_hook",
		},
		{
			Source:    "variant_mapping",
			Namespace: "mlx_core",
			Function:  "sum",
			Signature: "array(array, int, bool, StreamOrDevice)",
			Action:    "emit",
			CName:     "mlx_sum_axis",
			Suffix:    "axis",
		},
		{
			Source:    "variant_mapping",
			Namespace: "mlx_core",
			Function:  "sum",
			Signature: "array(array, bool, StreamOrDevice)",
			Action:    "emit",
			CName:     "mlx_sum",
		},
		{
			Source:    "variant_mapping",
			Namespace: "mlx_core",
			Function:  "sum",
			Signature: "array(array, StreamOrDevice)",
			Action:    "skip",
			Reason:    "variant_mapping",
		},
		{
			Source:    "variant_mapping",
			Namespace: "mlx_core_fft",
			Function:  "fftn",
			Signature: "array(array, std::vector<int>, StreamOrDevice)",
			Action:    "emit",
			CName:     "mlx_fft_fftn",
		},
		{
			Source:   "hook_registry",
			Function: "fast_cuda_kernel",
			Action:   "hook",
			CName:    "mlx_fast_cuda_kernel",
			Reason:   "custom_hook_unmatched",
		},
		{
			Source:   "hook_registry",
			Function: "fast_metal_kernel",
			Action:   "hook",
			CName:    "mlx_fast_metal_kernel",
			Reason:   "custom_hook_unmatched",
		},
		{
			Source:   "hook_registry",
			Function: "load_gguf",
			Action:   "hook",
			CName:    "mlx_load_gguf",
			Reason:   "custom_hook_unmatched",
		},
		{
			Source:   "hook_registry",
			Function: "print_graph",
			Action:   "hook",
			CName:    "mlx_print_graph",
			Reason:   "custom_hook_unmatched",
		},
		{
			Source:   "hook_registry",
			Function: "save_gguf",
			Action:   "hook",
			CName:    "mlx_save_gguf",
			Reason:   "custom_hook_unmatched",
		},
	}
	if !reflect.DeepEqual(decisions, want) {
		t.Fatalf("decisions = %#v, want %#v", decisions, want)
	}
	if summary.Emits != 3 || summary.Hooks != 6 || summary.Skips != 1 {
		t.Fatalf("summary = %#v, want 3 emits, 6 hooks, and 1 skip", summary)
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

func TestGeneratorDiagnosticsConvertsUnsupportedTypes(t *testing.T) {
	diagnostics := generatorDiagnostics(generators.New(), &parser.ParseResult{
		Functions: map[string][]*parser.Function{
			"mlx::core::bad_return": {{
				Name:       "bad_return",
				Namespace:  "mlx::core",
				ReturnType: "MissingReturn",
				File:       "mlx/ops.h",
				Line:       9,
				Col:        4,
			}},
		},
		Enums: map[string]*parser.Enum{},
	})
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one", diagnostics)
	}
	got := diagnostics[0]
	if got.Code != "skip_unsupported_return_type" ||
		got.File != "mlx/ops.h" ||
		got.Line != 9 ||
		got.Col != 4 {
		t.Fatalf("diagnostic = %#v", got)
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
