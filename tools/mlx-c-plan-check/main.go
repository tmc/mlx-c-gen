// Command mlx-c-plan-check verifies generator outputs against the inventory.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/inventory"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/plan"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mlx-c-plan-check: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	inventoryPath := flag.String("inventory", "codegen/generated-files.txt", "generated-file inventory path")
	flag.Parse()

	f, err := os.Open(*inventoryPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", *inventoryPath, err)
	}
	defer f.Close()
	entries, err := inventory.Read(f)
	if err != nil {
		return err
	}
	return plan.CheckInventory(entries)
}
