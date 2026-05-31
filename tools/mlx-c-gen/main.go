// Command mlxcgen generates C bindings for MLX from C++ headers.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/apilock"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/generators"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/ir"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/parser"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/plan"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/regenreport"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/symbols"
)

var standaloneGenerators = map[string]func(mode string) string{
	"vector": func(mode string) string {
		var buf bytes.Buffer
		generators.GenerateVector(&buf, mode)
		return buf.String()
	},
	"closure": func(mode string) string {
		var buf bytes.Buffer
		generators.GenerateClosure(&buf, mode)
		return buf.String()
	},
	"map": func(mode string) string {
		var buf bytes.Buffer
		generators.GenerateMap(&buf, mode)
		return buf.String()
	},
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "check":
			if err := runCheck(os.Args[2:]); err != nil {
				if errors.Is(err, flag.ErrHelp) {
					return
				}
				fmt.Fprintf(os.Stderr, "mlx-c-gen check: %v\n", err)
				os.Exit(1)
			}
			return
		case "parse":
			if err := runParse(os.Args[2:]); err != nil {
				if errors.Is(err, flag.ErrHelp) {
					return
				}
				fmt.Fprintf(os.Stderr, "mlx-c-gen parse: %v\n", err)
				os.Exit(1)
			}
			return
		case "generate":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		}
	}

	mlxSrc := flag.String("mlx-src", "", "Path to MLX source directory")
	outputDir := flag.String("output-dir", "", "Output directory for generated files")
	metadataPath := flag.String("metadata", "", "Path to output YAML metadata file")
	manifestPath := flag.String("manifest", "", "Path to generator manifest")
	customDir := flag.String("custom-dir", "", "Path to custom generator specs (reserved)")
	compileCommandsPath := flag.String("compile-commands", "", "Path to compile_commands.json for parser flags")
	astCacheDir := flag.String("ast-cache", "", "Cache parsed clang AST results under directory")
	noASTCache := flag.Bool("no-ast-cache", false, "Disable parsed clang AST cache")
	dryRun := flag.Bool("dry-run", false, "Print what would be done without doing it")
	noFormat := flag.Bool("no-format", false, "Skip running clang-format on generated files")
	flag.Parse()
	_ = *customDir

	mlxSrcPath, err := discoverMLXSource(*mlxSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Using MLX source: %s\n", mlxSrcPath)

	// Set up include paths for the parser
	absMlxSrc, _ := filepath.Abs(mlxSrcPath)
	parser.SetIncludePaths([]string{absMlxSrc})
	parser.SetCompileCommandsPath(*compileCommandsPath)
	parser.SetASTCacheDir(resolveASTCacheDir(*astCacheDir, *noASTCache))

	// Output directory
	var outDir string
	if *outputDir != "" {
		outDir = *outputDir
	} else {
		outDir = "mlx/c"
	}
	privateDir := filepath.Join(outDir, "private")

	if err := prepareOutputDir(outDir, *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	manifest, err := plan.LoadPath(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	headerMappings := manifest.Headers
	standaloneNames := manifest.Standalone

	fmt.Printf("Output directory: %s\n\n", outDir)

	if *dryRun {
		fmt.Println("DRY RUN - no files will be written")
		fmt.Println()
	}

	success := true

	// Generate header-based bindings
	fmt.Println("Generating header-based bindings...")

	// Initialize the generator
	gen := generators.NewWithManifest(manifest)

	// Parse all headers first to build a complete result for metadata
	// We need to collect all headers to parse them in one go if we want a single metadata file
	// But current structure parses per mapping.
	// For now, we'll collect results after parsing loop, or simpler:
	// We just used existing loop. To support metadata better, we might want to aggregate.
	// However, distinct mappings might have distinct needs.
	// Let's rely on the parser accumulating if we passed a single context, but we don't.
	//
	// Better approach for metadata:
	// Create a combined result holder if metadata is requested.
	combinedResult := &parser.ParseResult{
		Functions: make(map[string][]*parser.Function),
		Enums:     make(map[string]*parser.Enum),
	}
	combinedIR := ir.Result{}

	for _, hm := range headerMappings {
		if *dryRun {
			fmt.Printf("  Would generate %s.h and %s.cpp from %v\n", hm.Name, hm.Name, hm.Headers)
			continue
		}

		// Build full paths
		var fullPaths []string
		allExist := true
		for _, h := range hm.Headers {
			fullPath := filepath.Join(mlxSrcPath, h)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				fmt.Printf("  ERROR: Header not found: %s\n", fullPath)
				allExist = false
				break
			}
			fullPaths = append(fullPaths, fullPath)
		}
		if !allExist {
			success = false
			continue
		}

		// Set pre-includes for this header (may be nil)
		parser.SetPreIncludes(hm.PreIncludes)

		// Parse headers
		result, err := parser.ParseFiles(fullPaths)
		if err != nil {
			fmt.Printf("  ERROR parsing %s: %v\n", hm.Name, err)
			success = false
			continue
		}

		// Aggregate for metadata
		if *metadataPath != "" {
			result.Diagnostics = append(result.Diagnostics, gen.Diagnostics(result)...)
			combinedIR = ir.Merge(combinedIR, ir.FromParseResult(hm.Name, result))
			for k, v := range result.Functions {
				combinedResult.Functions[k] = v
			}
			for k, v := range result.Enums {
				combinedResult.Enums[k] = v
			}
			combinedResult.Diagnostics = append(combinedResult.Diagnostics, result.Diagnostics...)
		}

		// Generate .h file
		fmt.Printf("  Generating %s.h...\n", hm.Name)
		var hBuf bytes.Buffer
		if err := gen.Generate(&hBuf, result, hm.Name, fullPaths, false, hm.Docstring); err != nil {
			fmt.Printf("    ERROR: %v\n", err)
			success = false
			continue
		}
		hPath := filepath.Join(outDir, hm.Name+".h")
		if err := os.WriteFile(hPath, hBuf.Bytes(), 0644); err != nil {
			fmt.Printf("    ERROR writing %s: %v\n", hPath, err)
			success = false
			continue
		}

		// Generate .cpp file
		fmt.Printf("  Generating %s.cpp...\n", hm.Name)
		var cppBuf bytes.Buffer
		if err := gen.Generate(&cppBuf, result, hm.Name, fullPaths, true, hm.Docstring); err != nil {
			fmt.Printf("    ERROR: %v\n", err)
			success = false
			continue
		}
		cppPath := filepath.Join(outDir, hm.Name+".cpp")
		if err := os.WriteFile(cppPath, cppBuf.Bytes(), 0644); err != nil {
			fmt.Printf("    ERROR writing %s: %v\n", cppPath, err)
			success = false
			continue
		}
	}

	fmt.Println()

	// Generate metadata if requested
	if *metadataPath != "" && success {
		fmt.Printf("Generating metadata to %s...\n", *metadataPath)
		f, err := os.Create(*metadataPath)
		if err != nil {
			fmt.Printf("  ERROR creating metadata file: %v\n", err)
			success = false
		} else {
			defer f.Close()
			yg := generators.NewYaml()
			if err := yg.GenerateYamlWithIR(f, combinedResult, combinedIR); err != nil {
				fmt.Printf("  ERROR generating metadata: %v\n", err)
				success = false
			}
		}
	}

	// Generate standalone bindings
	fmt.Println("Generating standalone bindings...")
	for _, name := range standaloneNames {
		generate := standaloneGenerators[name]
		if generate == nil {
			fmt.Printf("  ERROR: no standalone generator for %s\n", name)
			success = false
			continue
		}
		if *dryRun {
			fmt.Printf("  Would generate %s.h, %s.cpp, private/%s.h\n", name, name, name)
			continue
		}

		// Generate .h file
		fmt.Printf("  Generating %s.h...\n", name)
		hContent := generate("header")
		hPath := filepath.Join(outDir, name+".h")
		if err := os.WriteFile(hPath, []byte(hContent), 0644); err != nil {
			fmt.Printf("    ERROR writing %s: %v\n", hPath, err)
			success = false
			continue
		}

		// Generate .cpp file
		fmt.Printf("  Generating %s.cpp...\n", name)
		cppContent := generate("impl")
		cppPath := filepath.Join(outDir, name+".cpp")
		if err := os.WriteFile(cppPath, []byte(cppContent), 0644); err != nil {
			fmt.Printf("    ERROR writing %s: %v\n", cppPath, err)
			success = false
			continue
		}

		// Generate private .h file
		fmt.Printf("  Generating private/%s.h...\n", name)
		privContent := generate("private")
		privPath := filepath.Join(privateDir, name+".h")
		if err := os.WriteFile(privPath, []byte(privContent), 0644); err != nil {
			fmt.Printf("    ERROR writing %s: %v\n", privPath, err)
			success = false
			continue
		}
	}

	fmt.Println()

	// Run clang-format
	if success && !*dryRun && !*noFormat {
		fmt.Println("Running clang-format on generated files...")
		var files []string
		for _, hm := range headerMappings {
			files = append(files, filepath.Join(outDir, hm.Name+".h"))
			files = append(files, filepath.Join(outDir, hm.Name+".cpp"))
		}
		for _, name := range standaloneNames {
			files = append(files, filepath.Join(outDir, name+".h"))
			files = append(files, filepath.Join(outDir, name+".cpp"))
			files = append(files, filepath.Join(privateDir, name+".h"))
		}

		for _, f := range files {
			// Read, format via stdin with --assume-filename, and write back
			// The assume-filename makes clang-format look for .clang-format
			// based on that path, so we use mlx/c/<basename> to find our config
			content, err := os.ReadFile(f)
			if err != nil {
				fmt.Printf("  WARNING: could not read %s: %v\n", f, err)
				continue
			}

			// Build assumed filename relative to repo root
			base := filepath.Base(f)
			assumedPath := filepath.Join("mlx", "c", base)
			if strings.Contains(f, "private") {
				assumedPath = filepath.Join("mlx", "c", "private", base)
			}

			cmd := exec.Command("clang-format", "--assume-filename="+assumedPath)
			cmd.Stdin = bytes.NewReader(content)
			var formatted bytes.Buffer
			cmd.Stdout = &formatted
			if err := cmd.Run(); err != nil {
				fmt.Printf("  WARNING: clang-format failed for %s: %v\n", f, err)
				continue
			}

			if err := os.WriteFile(f, formatted.Bytes(), 0644); err != nil {
				fmt.Printf("  WARNING: could not write %s: %v\n", f, err)
			}
		}
		fmt.Println()
	}

	if success {
		fmt.Println("Done!")
	} else {
		fmt.Println("Completed with errors.")
		os.Exit(1)
	}
}

type parseOptions struct {
	RepoRoot            string
	MLXSrc              string
	ManifestPath        string
	CustomDir           string
	CompileCommandsPath string
	ASTCacheDir         string
	NoASTCache          bool
	OutPath             string
	ReportPath          string
}

type parseReport struct {
	SchemaVersion int               `json:"schema_version"`
	RepoRoot      string            `json:"repo_root"`
	MLXSrc        string            `json:"mlx_src"`
	MLXRevision   string            `json:"mlx_revision,omitempty"`
	ClangVersion  string            `json:"clang_version,omitempty"`
	ASTCacheDir   string            `json:"ast_cache_dir,omitempty"`
	ManifestPath  string            `json:"manifest_path,omitempty"`
	CustomDir     string            `json:"custom_dir,omitempty"`
	Modules       []parseModule     `json:"modules,omitempty"`
	Summary       parseSummary      `json:"summary"`
	Diagnostics   []parseDiagnostic `json:"diagnostics,omitempty"`
	Command       []string          `json:"command"`
	IR            ir.Result         `json:"ir"`
}

type parseModule struct {
	Name      string   `json:"name"`
	Headers   []string `json:"headers"`
	Functions int      `json:"functions"`
	Enums     int      `json:"enums"`
}

type parseSummary struct {
	Functions   int `json:"functions"`
	Enums       int `json:"enums"`
	Diagnostics int `json:"diagnostics"`
}

type parseDiagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Col     int    `json:"col,omitempty"`
}

func runParse(args []string) error {
	opts, err := parseOptionsFromArgs(args)
	if err != nil {
		return err
	}
	if opts.MLXSrc == "" {
		mlxSrc, err := discoverMLXSource("")
		if err != nil {
			return err
		}
		opts.MLXSrc = mlxSrc
	}
	parsed, modules, diagnostics, err := parseIR(opts)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal parse IR: %w", err)
	}
	if err := writeCheckReport(opts.OutPath, append(data, '\n')); err != nil {
		return err
	}
	if opts.ReportPath == "" {
		return nil
	}
	report := parseReport{
		SchemaVersion: regenreport.SchemaVersion,
		RepoRoot:      opts.RepoRoot,
		MLXSrc:        opts.MLXSrc,
		ASTCacheDir:   opts.ASTCacheDir,
		ManifestPath:  opts.ManifestPath,
		CustomDir:     opts.CustomDir,
		Modules:       modules,
		Summary: parseSummary{
			Functions:   len(parsed.Functions),
			Enums:       len(parsed.Enums),
			Diagnostics: len(diagnostics),
		},
		Diagnostics: diagnostics,
		Command:     append([]string{"mlx-c-gen", "parse"}, args...),
		IR:          parsed,
	}
	if clangVersion, err := commandOutputLine("clang++", "--version"); err == nil {
		report.ClangVersion = clangVersion
	}
	if mlxRevision, err := commandOutput("git", "-C", opts.MLXSrc, "rev-parse", "HEAD"); err == nil {
		report.MLXRevision = mlxRevision
	}
	reportData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal parse report: %w", err)
	}
	return writeCheckReport(opts.ReportPath, append(reportData, '\n'))
}

func parseOptionsFromArgs(args []string) (parseOptions, error) {
	var opts parseOptions
	fs := flag.NewFlagSet("mlx-c-gen parse", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mlx-c-gen parse [options]")
		fmt.Fprintln(os.Stderr)
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Environment:")
		fmt.Fprintln(os.Stderr, "  MLX_C_AST_CACHE sets the default parsed clang AST cache directory.")
	}
	fs.StringVar(&opts.RepoRoot, "root", ".", "repository root")
	fs.StringVar(&opts.RepoRoot, "output-root", ".", "repository root (alias for --root)")
	fs.StringVar(&opts.MLXSrc, "mlx-src", "", "MLX source directory")
	fs.StringVar(&opts.ManifestPath, "manifest", "", "generator manifest path")
	fs.StringVar(&opts.CustomDir, "custom-dir", "", "custom generator spec directory (reserved)")
	fs.StringVar(&opts.CompileCommandsPath, "compile-commands", "", "compile_commands.json path for parser flags")
	fs.StringVar(&opts.ASTCacheDir, "ast-cache", "", "cache parsed clang AST results under directory")
	fs.BoolVar(&opts.NoASTCache, "no-ast-cache", false, "disable parsed clang AST cache")
	fs.StringVar(&opts.OutPath, "out", "-", "write normalized IR JSON to path or - for stdout")
	fs.StringVar(&opts.ReportPath, "report", "", "write parse report JSON to path")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	opts.ASTCacheDir = resolveASTCacheDir(opts.ASTCacheDir, opts.NoASTCache)
	return opts, nil
}

func parseIR(opts parseOptions) (ir.Result, []parseModule, []parseDiagnostic, error) {
	manifest, err := plan.LoadPath(opts.ManifestPath)
	if err != nil {
		return ir.Result{}, nil, nil, err
	}
	if err := manifest.CheckCMakeMLXRef(opts.RepoRoot); err != nil {
		return ir.Result{}, nil, nil, err
	}
	absMlxSrc, err := filepath.Abs(opts.MLXSrc)
	if err != nil {
		return ir.Result{}, nil, nil, fmt.Errorf("resolve MLX source: %w", err)
	}
	parser.SetIncludePaths([]string{absMlxSrc})
	parser.SetCompileCommandsPath(opts.CompileCommandsPath)
	parser.SetASTCacheDir(opts.ASTCacheDir)

	var parsed ir.Result
	var modules []parseModule
	var diagnostics []parseDiagnostic
	for _, hm := range manifest.Headers {
		fullPaths, err := headerPaths(opts.MLXSrc, hm.Headers)
		if err != nil {
			return ir.Result{}, nil, nil, err
		}
		parser.SetPreIncludes(hm.PreIncludes)
		result, err := parser.ParseFiles(fullPaths)
		if err != nil {
			return ir.Result{}, nil, nil, fmt.Errorf("parse %s: %w", hm.Name, err)
		}
		moduleIR := ir.FromParseResult(hm.Name, result)
		parsed = ir.Merge(parsed, moduleIR)
		modules = append(modules, parseModule{
			Name:      hm.Name,
			Headers:   append([]string(nil), hm.Headers...),
			Functions: len(moduleIR.Functions),
			Enums:     len(moduleIR.Enums),
		})
		for _, d := range result.Diagnostics {
			diagnostics = append(diagnostics, parseDiagnostic{
				Code:    d.Code,
				Message: d.Message,
				File:    d.File,
				Line:    d.Line,
				Col:     d.Col,
			})
		}
	}
	parsed.Sort()
	sortParseDiagnostics(diagnostics)
	return parsed, modules, diagnostics, nil
}

func headerPaths(mlxSrc string, headers []string) ([]string, error) {
	paths := make([]string, 0, len(headers))
	for _, header := range headers {
		path := filepath.Join(mlxSrc, header)
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("header %s: %w", path, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func sortParseDiagnostics(diagnostics []parseDiagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		a, b := diagnostics[i], diagnostics[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
}

func runCheck(args []string) error {
	opts, err := parseCheckOptions(args)
	if err != nil {
		return err
	}
	if opts.Options.MLXSrc == "" {
		mlxSrc, err := discoverMLXSource("")
		if err != nil {
			return err
		}
		opts.Options.MLXSrc = mlxSrc
	}
	report, err := regenreport.Run(opts.Options)
	if err != nil {
		return err
	}
	data, err := report.JSON()
	if err != nil {
		return err
	}
	if err := writeCheckReport(opts.ReportPath, data); err != nil {
		return err
	}
	if err := checkAPILock(opts); err != nil {
		return err
	}
	if len(opts.Symbols) > 0 {
		if err := symbols.Check(symbols.Options{
			LockPath: opts.LockPath,
			NM:       opts.NM,
			Targets:  opts.Symbols,
		}); err != nil {
			return err
		}
	}
	if opts.StrictGenerated && !report.Clean() {
		return fmt.Errorf("regenerated files differ")
	}
	return nil
}

type checkOptions struct {
	Options         regenreport.Options
	LockPath        string
	LockTUPath      string
	ReportPath      string
	NM              string
	Symbols         []symbols.TargetLibrary
	StrictGenerated bool
}

func parseCheckOptions(args []string) (checkOptions, error) {
	var opts checkOptions
	fs := flag.NewFlagSet("mlx-c-gen check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mlx-c-gen check [options]")
		fmt.Fprintln(os.Stderr)
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Environment:")
		fmt.Fprintln(os.Stderr, "  MLX_C_AST_CACHE sets the default parsed clang AST cache directory.")
	}
	var symbolTargets targetFlags
	var repoRoot string
	var inventoryPath string
	fs.StringVar(&repoRoot, "root", ".", "repository root")
	fs.StringVar(&repoRoot, "output-root", ".", "repository root (alias for --root)")
	mlxSrc := fs.String("mlx-src", "", "MLX source directory")
	manifestPath := fs.String("manifest", "", "generator manifest path")
	customDir := fs.String("custom-dir", "", "custom generator spec directory (reserved)")
	compileCommandsPath := fs.String("compile-commands", "", "compile_commands.json path for parser flags")
	fs.StringVar(&inventoryPath, "inventory", "codegen/generated-files.txt", "generated-file inventory path")
	fs.StringVar(&inventoryPath, "generated-files", "codegen/generated-files.txt", "generated-file inventory path (alias for --inventory)")
	typePolicyPath := fs.String("types", "", "type policy path")
	lockPath := fs.String("lock", "codegen/mlxc-capi.lock.json", "API lock JSON path")
	lockTUPath := fs.String("lock-tu", "codegen/lock.c", "generated API lock translation unit path")
	reportPath := fs.String("report", "", "write regeneration report JSON to path instead of stdout")
	workDir := fs.String("work-dir", "", "scratch work directory")
	astCacheDir := fs.String("ast-cache", "", "cache parsed clang AST results under directory")
	noASTCache := fs.Bool("no-ast-cache", false, "disable parsed clang AST cache")
	generator := fs.String("generator", defaultGeneratorCommand(), "generator command")
	nm := fs.String("nm", "nm", "nm command for optional symbol checks")
	noFormat := fs.Bool("no-format", false, "pass --no-format to mlx-c-gen")
	keepWork := fs.Bool("keep-work", false, "keep an auto-created scratch directory")
	strictGenerated := fs.Bool("strict-generated", false, "fail when generated files differ from checked-in artifacts")
	fs.Var(&symbolTargets, "symbol", "optional symbol check target=library; may be repeated")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	opts.Options = regenreport.Options{
		RepoRoot:            repoRoot,
		MLXSrc:              *mlxSrc,
		ManifestPath:        *manifestPath,
		CustomDir:           *customDir,
		TypePolicyPath:      *typePolicyPath,
		CompileCommandsPath: *compileCommandsPath,
		InventoryPath:       inventoryPath,
		WorkDir:             *workDir,
		ASTCacheDir:         resolveASTCacheDir(*astCacheDir, *noASTCache),
		NoASTCache:          *noASTCache,
		Generator:           strings.Fields(*generator),
		NoFormat:            *noFormat,
		KeepWork:            *keepWork,
	}
	opts.LockPath = *lockPath
	opts.LockTUPath = *lockTUPath
	opts.ReportPath = *reportPath
	opts.NM = *nm
	opts.Symbols = symbolTargets
	opts.StrictGenerated = *strictGenerated
	return opts, nil
}

func writeCheckReport(path string, data []byte) error {
	if path == "" || path == "-" {
		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create report temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write report temp file: %w", err)
	}
	if err := tmp.Chmod(0o666); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod report temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close report temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("write report %s: %w", path, err)
	}
	return nil
}

func commandOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func commandOutputLine(name string, args ...string) (string, error) {
	out, err := commandOutput(name, args...)
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(out, "\n")
	return line, nil
}

func defaultGeneratorCommand() string {
	exe, err := os.Executable()
	if err == nil && exe != "" {
		return exe
	}
	return "go run ./tools/mlx-c-gen"
}

func resolveASTCacheDir(explicit string, disabled bool) string {
	return parser.ResolveASTCacheDir(explicit, disabled)
}

type targetFlags []symbols.TargetLibrary

func (f *targetFlags) String() string {
	var parts []string
	for _, tl := range *f {
		parts = append(parts, tl.Target+"="+tl.Path)
	}
	return strings.Join(parts, ",")
}

func (f *targetFlags) Set(s string) error {
	target, path, ok := strings.Cut(s, "=")
	if !ok || target == "" || path == "" {
		return fmt.Errorf("expected target=library")
	}
	*f = append(*f, symbols.TargetLibrary{Target: target, Path: path})
	return nil
}

func checkAPILock(opts checkOptions) error {
	if opts.LockPath == "" {
		return nil
	}
	headersDir := filepath.Join(opts.Options.RepoRoot, "mlx", "c")
	lock, err := apilock.Generate(headersDir)
	if err != nil {
		return err
	}
	jsonData, err := lock.JSON()
	if err != nil {
		return err
	}
	if err := checkFile(repoPath(opts.Options.RepoRoot, opts.LockPath), jsonData); err != nil {
		return err
	}
	if opts.LockTUPath != "" {
		tuData, err := lock.LockC()
		if err != nil {
			return err
		}
		if err := checkFile(repoPath(opts.Options.RepoRoot, opts.LockTUPath), tuData); err != nil {
			return err
		}
	}
	return nil
}

func repoPath(root, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func checkFile(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s is out of date", path)
	}
	return nil
}

func prepareOutputDir(outDir string, dryRun bool) error {
	if dryRun {
		return nil
	}
	if outDir == "" {
		return fmt.Errorf("missing output directory")
	}
	if err := os.MkdirAll(filepath.Join(outDir, "private"), 0o777); err != nil {
		return fmt.Errorf("create output directory %s: %w", outDir, err)
	}
	return nil
}

func discoverMLXSource(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	candidates := []string{
		"build/_deps/mlx-src",
		"build-Debug/_deps/mlx-src",
		"build-Release/_deps/mlx-src",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find MLX source directory; please build the project first or specify --mlx-src")
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Generate C bindings for MLX from C++ headers.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Subcommands:")
		fmt.Fprintln(os.Stderr, "  generate   Generate bindings. This is the default.")
		fmt.Fprintln(os.Stderr, "  parse      Parse upstream headers into normalized IR JSON.")
		fmt.Fprintln(os.Stderr, "  check      Regenerate into a scratch tree and verify API locks.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Environment:")
		fmt.Fprintln(os.Stderr, "  MLX_C_AST_CACHE sets the default parsed clang AST cache directory.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Example:")
		fmt.Fprintln(os.Stderr, "  mlxcgen --mlx-src=build/_deps/mlx-src")
		fmt.Fprintln(os.Stderr, "  mlxcgen check --mlx-src=build/_deps/mlx-src")
	}
}
