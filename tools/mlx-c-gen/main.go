// Command mlxcgen generates C bindings for MLX from C++ headers.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/generators"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/parser"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/plan"
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
	mlxSrc := flag.String("mlx-src", "", "Path to MLX source directory")
	outputDir := flag.String("output-dir", "", "Output directory for generated files")
	metadataPath := flag.String("metadata", "", "Path to output YAML metadata file")
	compileCommandsPath := flag.String("compile-commands", "", "Path to compile_commands.json for parser flags")
	astCacheDir := flag.String("ast-cache", "", "Cache clang AST JSON under directory")
	dryRun := flag.Bool("dry-run", false, "Print what would be done without doing it")
	noFormat := flag.Bool("no-format", false, "Skip running clang-format on generated files")
	flag.Parse()

	// Find MLX source
	var mlxSrcPath string
	if *mlxSrc != "" {
		mlxSrcPath = *mlxSrc
	} else {
		// Try common locations relative to current directory
		candidates := []string{
			"build/_deps/mlx-src",
			"build-Debug/_deps/mlx-src",
			"build-Release/_deps/mlx-src",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				mlxSrcPath = c
				break
			}
		}
		if mlxSrcPath == "" {
			fmt.Fprintln(os.Stderr, "ERROR: Could not find MLX source directory.")
			fmt.Fprintln(os.Stderr, "Please build the project first or specify --mlx-src")
			os.Exit(1)
		}
	}

	fmt.Printf("Using MLX source: %s\n", mlxSrcPath)

	// Set up include paths for the parser
	absMlxSrc, _ := filepath.Abs(mlxSrcPath)
	parser.SetIncludePaths([]string{absMlxSrc})
	parser.SetCompileCommandsPath(*compileCommandsPath)
	parser.SetASTCacheDir(*astCacheDir)

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

	headerMappings, err := plan.HeaderMappings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	standaloneNames, err := plan.StandaloneNames()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Output directory: %s\n\n", outDir)

	if *dryRun {
		fmt.Println("DRY RUN - no files will be written")
		fmt.Println()
	}

	success := true

	// Generate header-based bindings
	fmt.Println("Generating header-based bindings...")

	// Initialize the generator
	gen := generators.New()

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
			if err := yg.GenerateYaml(f, combinedResult); err != nil {
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

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Generate C bindings for MLX from C++ headers.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Example:")
		fmt.Fprintln(os.Stderr, "  mlxcgen --mlx-src=build/_deps/mlx-src")
	}
}
