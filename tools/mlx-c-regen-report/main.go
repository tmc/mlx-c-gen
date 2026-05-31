// Command mlx-c-regen-report reports scratch-tree regeneration drift.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/regenreport"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mlx-c-regen-report: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	repoRoot := flag.String("root", ".", "repository root")
	mlxSrc := flag.String("mlx-src", "", "MLX source directory")
	inventoryPath := flag.String("inventory", "codegen/generated-files.txt", "generated-file inventory path")
	workDir := flag.String("work-dir", "", "scratch work directory")
	generator := flag.String("generator", "go run ./tools/mlx-c-gen", "generator command")
	noFormat := flag.Bool("no-format", false, "pass --no-format to mlx-c-gen")
	keepWork := flag.Bool("keep-work", false, "keep an auto-created scratch directory")
	check := flag.Bool("check", false, "exit non-zero when drift is detected")
	flag.Parse()

	report, err := regenreport.Run(regenreport.Options{
		RepoRoot:      *repoRoot,
		MLXSrc:        *mlxSrc,
		InventoryPath: *inventoryPath,
		WorkDir:       *workDir,
		Generator:     strings.Fields(*generator),
		NoFormat:      *noFormat,
		KeepWork:      *keepWork,
	})
	if err != nil {
		return err
	}
	data, err := report.JSON()
	if err != nil {
		return err
	}
	if _, err := os.Stdout.Write(data); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	if *check && !report.Clean() {
		return fmt.Errorf("regenerated files differ")
	}
	return nil
}
