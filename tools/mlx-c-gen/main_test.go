package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/customspec"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/doccoverage"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/generators"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/ir"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/parser"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/plan"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/regenreport"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/types"
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
		"--strict-docs",
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
		got.NoFormatCache ||
		!got.NoFormat ||
		!got.KeepWork ||
		!opts.StrictGenerated ||
		!opts.StrictDocs {
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

func TestParseCheckOptionsFormatCache(t *testing.T) {
	opts, err := parseCheckOptions([]string{"--format-cache", "/tmp/format-cache"})
	if err != nil {
		t.Fatalf("parseCheckOptions format cache: %v", err)
	}
	if opts.Options.FormatCacheDir != "/tmp/format-cache" || opts.Options.NoFormatCache {
		t.Fatalf("cache options = %#v", opts.Options)
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
		"--inventory", "/repo/codegen/generated-files.txt",
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
		opts.InventoryPath != "/repo/codegen/generated-files.txt" ||
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
		opts.InventoryPath != "codegen/generated-files.txt" ||
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
		"--format-cache=/tmp/format-cache",
		"--report=/tmp/report.json",
	})
	want := []string{
		"--manifest=codegen/manifest.yaml",
		"--output-root",
		".",
		"--format-cache=<path>",
		"--report=<path>",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized args = %#v, want %#v", got, want)
	}
}

func TestNewGenerateReport(t *testing.T) {
	report := newGenerateReport(generateReportOptions{
		Args:           []string{"--output-root", ".", "--report", "/tmp/report.json"},
		OutputRoot:     ".",
		OutputDir:      filepath.Join("mlx", "c"),
		MLXSrc:         "/missing/mlx",
		ManifestPath:   "codegen/manifest.yaml",
		FormatCacheDir: "/tmp/format-cache",
		Manifest: plan.Manifest{
			SchemaVersion: plan.SchemaVersion,
			Headers: []plan.HeaderMapping{{
				Name:    "ops",
				Headers: []string{"mlx/ops.h"},
			}},
			Standalone: []string{"vector"},
		},
		CustomSpecs: []customspec.Spec{{
			Name:     "jaccl",
			Header:   "mlx/c/jaccl.h",
			Generate: customspec.GenerateSpec{Header: true},
		}},
		NoFormat: true,
	})
	if report.SchemaVersion != 1 ||
		report.OutputRoot != "." ||
		report.OutputDir != filepath.Join("mlx", "c") ||
		report.FormatCacheDir != "<path>" ||
		len(report.Modules) != 1 ||
		len(report.GeneratedFiles) != 6 ||
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

func TestCustomHeaderOutputPath(t *testing.T) {
	got, err := customHeaderOutputPath(filepath.Join("/tmp", "out"), "mlx/c/jaccl.h")
	if err != nil {
		t.Fatalf("customHeaderOutputPath: %v", err)
	}
	if got != filepath.Join("/tmp", "out", "jaccl.h") {
		t.Fatalf("path = %q, want jaccl header under output dir", got)
	}
	if _, err := customHeaderOutputPath("/tmp/out", "include/jaccl.h"); err == nil {
		t.Fatal("customHeaderOutputPath accepted header outside mlx/c")
	}
	if _, err := customHeaderOutputPath("/tmp/out", "mlx/c/../jaccl.h"); err == nil {
		t.Fatal("customHeaderOutputPath accepted escaping header")
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
	manifest := plan.Manifest{
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
	}
	parsed := ir.Result{Functions: []ir.FuncDecl{
		{
			ID:        "graph_utils|mlx/graph_utils.h|mlx::core|export_to_dot|void(std::ostream&, NodeNamer, std::vector<array>)",
			Namespace: "mlx::core",
			Name:      "export_to_dot",
			Return:    "void",
			Params: []ir.Param{
				{Type: "std::ostream&"},
				{Type: "NodeNamer"},
				{Type: "std::vector<array>"},
			},
		},
		{
			ID:        "ops|mlx/ops.h|mlx::core|sum|array(array, int, bool, StreamOrDevice)",
			Namespace: "mlx::core",
			Name:      "sum",
			Return:    "array",
			Params: []ir.Param{
				{Type: "array"},
				{Type: "int"},
				{Type: "bool"},
				{Type: "StreamOrDevice"},
			},
		},
		{
			ID:        "ops|mlx/ops.h|mlx::core|sum|array(array, bool, StreamOrDevice)",
			Namespace: "mlx::core",
			Name:      "sum",
			Return:    "array",
			Params: []ir.Param{
				{Type: "array"},
				{Type: "bool"},
				{Type: "StreamOrDevice"},
			},
		},
		{
			ID:        "ops|mlx/ops.h|mlx::core|sum|array(array, StreamOrDevice)",
			Namespace: "mlx::core",
			Name:      "sum",
			Return:    "array",
			Params: []ir.Param{
				{Type: "array"},
				{Type: "StreamOrDevice"},
			},
		},
		{
			ID:        "fft|mlx/fft.h|mlx::core::fft|fftn|array(array, std::vector<int>, StreamOrDevice)",
			Namespace: "mlx::core::fft",
			Name:      "fftn",
			Return:    "array",
			Params: []ir.Param{
				{Type: "array"},
				{Type: "std::vector<int>"},
				{Type: "StreamOrDevice"},
			},
		},
	}}
	decisions, summary := parseVariantDecisions(manifest, parsed)
	want := []parseDecision{
		{
			Source:    "variant_mapping",
			DeclID:    "graph_utils|mlx/graph_utils.h|mlx::core|export_to_dot|void(std::ostream&, NodeNamer, std::vector<array>)",
			Namespace: "mlx_core",
			Function:  "export_to_dot",
			Signature: "void(std::ostream&, NodeNamer, std::vector<array>)",
			Action:    "hook",
			CName:     "mlx_export_to_dot",
			Reason:    "custom_hook",
		},
		{
			Source:    "variant_mapping",
			DeclID:    "ops|mlx/ops.h|mlx::core|sum|array(array, int, bool, StreamOrDevice)",
			Namespace: "mlx_core",
			Function:  "sum",
			Signature: "array(array, int, bool, StreamOrDevice)",
			Action:    "emit",
			CName:     "mlx_sum_axis",
			Suffix:    "axis",
		},
		{
			Source:    "variant_mapping",
			DeclID:    "ops|mlx/ops.h|mlx::core|sum|array(array, bool, StreamOrDevice)",
			Namespace: "mlx_core",
			Function:  "sum",
			Signature: "array(array, bool, StreamOrDevice)",
			Action:    "emit",
			CName:     "mlx_sum",
		},
		{
			Source:    "variant_mapping",
			DeclID:    "ops|mlx/ops.h|mlx::core|sum|array(array, StreamOrDevice)",
			Namespace: "mlx_core",
			Function:  "sum",
			Signature: "array(array, StreamOrDevice)",
			Action:    "skip",
			Reason:    "variant_mapping",
		},
		{
			Source:    "variant_mapping",
			DeclID:    "fft|mlx/fft.h|mlx::core::fft|fftn|array(array, std::vector<int>, StreamOrDevice)",
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

func TestParseVariantDecisionsUsesCustomHooks(t *testing.T) {
	decisions, summary := parseVariantDecisions(hookManifest(), ir.Result{})
	if summary.Hooks != 6 {
		t.Fatalf("summary = %#v, want 6 hooks", summary)
	}
	for _, decision := range decisions {
		if decision.Reason == "custom_hook_unmatched" {
			t.Fatalf("unexpected unmatched hook decision: %#v", decision)
		}
	}
	var found bool
	for _, decision := range decisions {
		if decision.Source == "custom_hook" && decision.CName == "mlx_load_gguf" {
			found = true
			if decision.Reason != "custom GGUF loading API" {
				t.Fatalf("custom hook decision = %#v", decision)
			}
		}
	}
	if !found {
		t.Fatalf("decisions = %#v, missing mlx_load_gguf custom hook", decisions)
	}
}

func TestParseInventoryDecisions(t *testing.T) {
	root := t.TempDir()
	inventoryPath := filepath.Join(root, "codegen", "generated-files.txt")
	if err := os.MkdirAll(filepath.Dir(inventoryPath), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inventoryPath, []byte(`
generated_header_api mlxc mlx/c/ops.h
handwritten_runtime jacclc mlx/c/jaccl.cpp
custom_spec_generated mlxc mlx/c/array.h
not_owned_by_codegen mlxc mlx/c/mlx.h
generated_support mlxc mlx/c/vector.h
`), 0o666); err != nil {
		t.Fatal(err)
	}
	decisions, summary, err := parseInventoryDecisions(parseOptions{
		RepoRoot:      root,
		InventoryPath: filepath.Join("codegen", "generated-files.txt"),
	})
	if err != nil {
		t.Fatalf("parseInventoryDecisions: %v", err)
	}
	want := []parseFileDecision{
		{Source: "inventory", Target: "mlxc", Path: "mlx/c/array.h", Action: "custom_spec", Reason: "custom_spec_generated"},
		{Source: "inventory", Target: "jacclc", Path: "mlx/c/jaccl.cpp", Action: "handwritten", Reason: "handwritten_runtime"},
		{Source: "inventory", Target: "mlxc", Path: "mlx/c/mlx.h", Action: "not_owned", Reason: "not_owned_by_codegen"},
		{Source: "inventory", Target: "mlxc", Path: "mlx/c/ops.h", Action: "emit", Reason: "generated_header_api"},
		{Source: "inventory", Target: "mlxc", Path: "mlx/c/vector.h", Action: "emit", Reason: "generated_support"},
	}
	if !reflect.DeepEqual(decisions, want) {
		t.Fatalf("decisions = %#v, want %#v", decisions, want)
	}
	if summary.Handwritten != 1 || summary.CustomSpecs != 1 || summary.NotOwned != 1 {
		t.Fatalf("summary = %#v, want one handwritten, custom spec, and not owned", summary)
	}
}

func TestParseCheckOptionsDefaults(t *testing.T) {
	t.Setenv("MLX_C_AST_CACHE", "/tmp/mlx-c-default-cache")
	t.Setenv("MLX_C_FORMAT_CACHE", "/tmp/mlx-c-format-cache")
	opts, err := parseCheckOptions(nil)
	if err != nil {
		t.Fatalf("parseCheckOptions defaults: %v", err)
	}
	got := opts.Options
	if got.RepoRoot != "." ||
		got.InventoryPath != "codegen/generated-files.txt" ||
		got.MLXSrc != "" ||
		got.ASTCacheDir != "/tmp/mlx-c-default-cache" ||
		got.FormatCacheDir != "/tmp/mlx-c-format-cache" ||
		got.NoASTCache ||
		got.NoFormatCache ||
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

func TestParseCheckOptionsNoFormatCache(t *testing.T) {
	t.Setenv("MLX_C_FORMAT_CACHE", "/tmp/mlx-c-format-cache")
	opts, err := parseCheckOptions([]string{"--no-format-cache"})
	if err != nil {
		t.Fatalf("parseCheckOptions no format cache: %v", err)
	}
	if opts.Options.FormatCacheDir != "" || !opts.Options.NoFormatCache {
		t.Fatalf("cache options = %#v, want disabled", opts.Options)
	}
}

func TestCheckDocCoverageUsesManifestPolicy(t *testing.T) {
	report := &regenreport.Report{
		DocCoverage: doccoverage.Coverage{Missing: 1},
	}
	if err := checkDocCoverage(report, false); err != nil {
		t.Fatalf("checkDocCoverage without policy = %v, want nil", err)
	}
	report.Manifest.Report.RequireDocCoverage = true
	err := checkDocCoverage(report, false)
	if err == nil || !strings.Contains(err.Error(), "missing doc source") {
		t.Fatalf("checkDocCoverage with manifest policy = %v, want missing docs error", err)
	}
	report.DocCoverage.Missing = 0
	if err := checkDocCoverage(report, false); err != nil {
		t.Fatalf("checkDocCoverage clean = %v, want nil", err)
	}
}

func TestCheckTypeCoverageUsesManifestPolicy(t *testing.T) {
	report := &regenreport.Report{
		Diagnostics: []regenreport.Diagnostic{{
			Code: "skip_unsupported_return_type",
		}},
	}
	if err := checkTypeCoverage(report); err != nil {
		t.Fatalf("checkTypeCoverage without policy = %v, want nil", err)
	}
	report.Manifest.Report.RequireTypeCoverage = true
	err := checkTypeCoverage(report)
	if err == nil || !strings.Contains(err.Error(), "unsupported types") {
		t.Fatalf("checkTypeCoverage with manifest policy = %v, want unsupported types error", err)
	}
	report.Diagnostics = nil
	report.MissingTypes = []types.MissingType{{Type: "Missing"}}
	err = checkTypeCoverage(report)
	if err == nil || !strings.Contains(err.Error(), "missing type policy entries") {
		t.Fatalf("checkTypeCoverage missing type = %v, want missing type error", err)
	}
	report.MissingTypes = nil
	if err := checkTypeCoverage(report); err != nil {
		t.Fatalf("checkTypeCoverage clean = %v, want nil", err)
	}
}

func TestCheckDiagnosticReasonsUsesManifestPolicy(t *testing.T) {
	report := &regenreport.Report{
		Diagnostics: []regenreport.Diagnostic{{
			Code: "skip_operator",
		}},
	}
	if err := checkDiagnosticReasons(report); err != nil {
		t.Fatalf("checkDiagnosticReasons without policy = %v, want nil", err)
	}
	report.Manifest.Report.RequireDiagnosticReasons = true
	err := checkDiagnosticReasons(report)
	if err == nil || !strings.Contains(err.Error(), "missing reason") {
		t.Fatalf("checkDiagnosticReasons with manifest policy = %v, want missing reason error", err)
	}
	report.Diagnostics[0].Reason = "not_c_api"
	if err := checkDiagnosticReasons(report); err != nil {
		t.Fatalf("checkDiagnosticReasons clean = %v, want nil", err)
	}
}

func TestCheckExplicitVariantsUsesManifestPolicy(t *testing.T) {
	report := &regenreport.Report{
		Diagnostics: []regenreport.Diagnostic{{
			Code: "emit_implicit_single_overload",
		}},
	}
	if err := checkExplicitVariants(report); err != nil {
		t.Fatalf("checkExplicitVariants without policy = %v, want nil", err)
	}
	report.Manifest.Report.RequireExplicitVariants = true
	err := checkExplicitVariants(report)
	if err == nil || !strings.Contains(err.Error(), "implicit variant selection") {
		t.Fatalf("checkExplicitVariants with manifest policy = %v, want implicit variant error", err)
	}
	report.Diagnostics = nil
	if err := checkExplicitVariants(report); err != nil {
		t.Fatalf("checkExplicitVariants clean = %v, want nil", err)
	}
}

func TestCheckGeneratedCleanUsesManifestPolicy(t *testing.T) {
	report := &regenreport.Report{
		Summary: regenreport.Summary{Different: 1},
	}
	if err := checkGeneratedClean(report, false); err != nil {
		t.Fatalf("checkGeneratedClean without policy = %v, want nil", err)
	}
	report.Manifest.Report.RequireCleanGenerated = true
	err := checkGeneratedClean(report, false)
	if err == nil || !strings.Contains(err.Error(), "regenerated files differ") {
		t.Fatalf("checkGeneratedClean with manifest policy = %v, want generated diff error", err)
	}
	report.Summary.Different = 0
	if err := checkGeneratedClean(report, false); err != nil {
		t.Fatalf("checkGeneratedClean clean = %v, want nil", err)
	}
}

func TestCheckGeneratedCleanUsesStrictFlag(t *testing.T) {
	report := &regenreport.Report{
		GeneratedOnly: []string{"mlx/c/extra.h"},
	}
	err := checkGeneratedClean(report, true)
	if err == nil || !strings.Contains(err.Error(), "regenerated files differ") {
		t.Fatalf("checkGeneratedClean strict = %v, want generated diff error", err)
	}
}

func TestCheckAPILockRequiresLockPathWhenManifestRequiresIt(t *testing.T) {
	report := &regenreport.Report{}
	if err := checkAPILock(checkOptions{}, report); err != nil {
		t.Fatalf("checkAPILock without policy = %v, want nil", err)
	}
	report.Manifest.Report.RequireAPILock = true
	err := checkAPILock(checkOptions{}, report)
	if err == nil || !strings.Contains(err.Error(), "requires API lock") {
		t.Fatalf("checkAPILock without lock path = %v, want required lock error", err)
	}
}

func TestCheckDocCoverageUsesStrictFlag(t *testing.T) {
	report := &regenreport.Report{
		DocCoverage: doccoverage.Coverage{Missing: 1},
	}
	err := checkDocCoverage(report, true)
	if err == nil || !strings.Contains(err.Error(), "missing doc source") {
		t.Fatalf("checkDocCoverage strict = %v, want missing docs error", err)
	}
}

func TestCheckGeneratedMarkersUsesManifestPolicy(t *testing.T) {
	report := &regenreport.Report{
		GeneratedMarkerViolations: []regenreport.GeneratedMarkerViolation{{
			Path:   "mlx/c/ops.h",
			Reason: "timestamp",
			Marker: "/* Generated at 2026-05-31T17:00:00Z */",
		}},
	}
	if err := checkGeneratedMarkers(report); err != nil {
		t.Fatalf("checkGeneratedMarkers without policy = %v, want nil", err)
	}
	report.Manifest.GeneratedMarkers.ForbidVolatileData = true
	err := checkGeneratedMarkers(report)
	if err == nil || !strings.Contains(err.Error(), "volatile data") {
		t.Fatalf("checkGeneratedMarkers with policy = %v, want volatile data error", err)
	}
	report.GeneratedMarkerViolations = nil
	if err := checkGeneratedMarkers(report); err != nil {
		t.Fatalf("checkGeneratedMarkers clean = %v, want nil", err)
	}
}

func TestCheckHookManifestPolicy(t *testing.T) {
	if err := checkHookManifestPolicy(hookManifest()); err != nil {
		t.Fatalf("checkHookManifestPolicy complete = %v, want nil", err)
	}
	manifest := hookManifest()
	manifest.CustomHooks = manifest.CustomHooks[:len(manifest.CustomHooks)-1]
	err := checkHookManifestPolicy(manifest)
	if err == nil || !strings.Contains(err.Error(), "not declared in manifest") {
		t.Fatalf("checkHookManifestPolicy missing hook = %v, want undeclared error", err)
	}
	manifest = hookManifest()
	manifest.CustomHooks = append(manifest.CustomHooks, plan.CustomHook{CName: "mlx_missing_hook"})
	err = checkHookManifestPolicy(manifest)
	if err == nil || !strings.Contains(err.Error(), "unknown custom hook") {
		t.Fatalf("checkHookManifestPolicy unknown hook = %v, want unknown hook error", err)
	}
}

func TestParseDetailDecisions(t *testing.T) {
	selected := ir.Result{Functions: []ir.FuncDecl{
		{
			ID:        "compile|mlx/compile_impl.h|mlx::core::detail|compile_clear_cache|void()",
			Namespace: "mlx::core::detail",
			Name:      "compile_clear_cache",
			Return:    "void",
		},
		{
			ID:        "ops|mlx/ops.h|mlx::core|sum|array(array)",
			Namespace: "mlx::core",
			Name:      "sum",
			Return:    "array",
			Params:    []ir.Param{{Type: "array"}},
		},
	}}
	decisions, summary := parseDetailDecisions(selected)
	want := []parseDecision{{
		Source:    "allowed_detail_function",
		DeclID:    "compile|mlx/compile_impl.h|mlx::core::detail|compile_clear_cache|void()",
		Namespace: "mlx_core_detail",
		Function:  "compile_clear_cache",
		Signature: "void()",
		Action:    "emit",
		CName:     "mlx_detail_compile_clear_cache",
	}}
	if !reflect.DeepEqual(decisions, want) {
		t.Fatalf("decisions = %#v, want %#v", decisions, want)
	}
	if summary.Emits != 1 || summary.Hooks != 0 || summary.Skips != 0 {
		t.Fatalf("summary = %#v, want one emit", summary)
	}
}

func TestCheckDecisionDeclIDsUsesManifestPolicy(t *testing.T) {
	decisions := []parseDecision{{
		Source:    "variant_mapping",
		Function:  "sum",
		Signature: "array(array, StreamOrDevice)",
		Action:    "emit",
	}}
	if err := checkDecisionDeclIDs(plan.Manifest{}, decisions); err != nil {
		t.Fatalf("checkDecisionDeclIDs without policy = %v, want nil", err)
	}
	manifest := plan.Manifest{
		Report: plan.ReportPolicy{RequireDecisionDeclIDs: true},
	}
	err := checkDecisionDeclIDs(manifest, decisions)
	if err == nil || !strings.Contains(err.Error(), "missing declaration id") {
		t.Fatalf("checkDecisionDeclIDs missing id = %v, want missing declaration id error", err)
	}
	decisions[0].DeclID = "ops|mlx/ops.h|mlx::core|sum|array(array, StreamOrDevice)"
	if err := checkDecisionDeclIDs(manifest, decisions); err != nil {
		t.Fatalf("checkDecisionDeclIDs with id = %v, want nil", err)
	}
}

func TestEnrichDiagnosticsWithDeclIDs(t *testing.T) {
	diagnostics := []parseDiagnostic{
		{
			Code: "skip_variant_mapping",
			File: "/repo/mlx/ops.h",
			Line: 12,
			Col:  4,
		},
		{
			Code: "skip_template_function",
			File: "/repo/mlx/ops.h",
			Line: 20,
			Col:  1,
		},
	}
	parsed := ir.Result{Functions: []ir.FuncDecl{{
		ID:     "ops|mlx/ops.h|mlx::core|sum|array(array)",
		Loc:    ir.SourceLoc{File: "mlx/ops.h", Line: 12, Col: 4},
		Return: "array",
	}}}

	enrichDiagnosticsWithDeclIDs(diagnostics, parsed)
	if diagnostics[0].DeclID != "ops|mlx/ops.h|mlx::core|sum|array(array)" {
		t.Fatalf("diagnostic decl id = %q, want parsed declaration id", diagnostics[0].DeclID)
	}
	if diagnostics[1].DeclID != "" {
		t.Fatalf("unmatched diagnostic decl id = %q, want empty", diagnostics[1].DeclID)
	}
}

func hookManifest() plan.Manifest {
	base := ""
	return plan.Manifest{
		VariantMappings: map[string]map[string][]plan.Variant{
			"mlx_core": {
				"export_to_dot": {
					{Signature: "void(std::ostream&, NodeNamer, std::vector<array>)", Suffix: &base},
				},
				"print_graph": {
					{Signature: "void(std::ostream&, NodeNamer, std::vector<array>)", Suffix: &base},
				},
			},
		},
		CustomHooks: []plan.CustomHook{
			{CName: "mlx_fast_cuda_kernel", Reason: "custom CUDA kernel API"},
			{CName: "mlx_fast_metal_kernel", Reason: "custom Metal kernel API"},
			{CName: "mlx_load_gguf", Reason: "custom GGUF loading API"},
			{CName: "mlx_save_gguf", Reason: "custom GGUF saving API"},
		},
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

func TestResolveFormatCacheDir(t *testing.T) {
	t.Setenv("MLX_C_FORMAT_CACHE", "/tmp/mlx-c-env-format-cache")
	if got := resolveFormatCacheDir("", false); got != "/tmp/mlx-c-env-format-cache" {
		t.Fatalf("default cache dir = %q, want env", got)
	}
	if got := resolveFormatCacheDir("/tmp/mlx-c-explicit-format-cache", false); got != "/tmp/mlx-c-explicit-format-cache" {
		t.Fatalf("explicit cache dir = %q, want explicit", got)
	}
	if got := resolveFormatCacheDir("/tmp/mlx-c-explicit-format-cache", true); got != "" {
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
	got, ok := firstDiagnosticWithCode(diagnostics, "skip_unsupported_return_type")
	if !ok {
		t.Fatalf("diagnostics = %#v, missing skip_unsupported_return_type", diagnostics)
	}
	if got.Code != "skip_unsupported_return_type" ||
		got.File != "mlx/ops.h" ||
		got.Line != 9 ||
		got.Col != 4 {
		t.Fatalf("diagnostic = %#v", got)
	}
}

func firstDiagnosticWithCode(diagnostics []parseDiagnostic, code string) (parseDiagnostic, bool) {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return diagnostic, true
		}
	}
	return parseDiagnostic{}, false
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
