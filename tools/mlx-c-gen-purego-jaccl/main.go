package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tmc/mlx-c-gen/internal/mlxcgen/apilock"
	"github.com/tmc/mlx-c-gen/internal/mlxcgen/puregojaccl"
)

func main() {
	lockPath := flag.String("lock", "codegen/mlxc-capi.lock.json", "API lock JSON path")
	outDir := flag.String("out", "internal/jacclc", "output package directory")
	packageName := flag.String("package", "jacclc", "output package name")
	targetName := flag.String("target", "jacclc", "API lock target name")
	check := flag.Bool("check", false, "check generated files without writing")
	flag.Parse()

	if err := run(*lockPath, *outDir, *packageName, *targetName, *check); err != nil {
		fmt.Fprintf(os.Stderr, "mlx-c-gen-purego-jaccl: %v\n", err)
		os.Exit(1)
	}
}

func run(lockPath, outDir, packageName, targetName string, check bool) error {
	lock, err := readLock(lockPath)
	if err != nil {
		return err
	}
	files, err := puregojaccl.Generate(lock, puregojaccl.Options{
		PackageName: packageName,
		TargetName:  targetName,
		ToolName:    "mlx-c-gen-purego-jaccl",
	})
	if err != nil {
		return err
	}
	if check {
		return checkDir(outDir, files)
	}
	return writeDir(outDir, files)
}

func readLock(path string) (*apilock.Lock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read api lock: %w", err)
	}
	var lock apilock.Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse api lock: %w", err)
	}
	return &lock, nil
}

func writeDir(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	names := sortedNames(files)
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, files[name], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func checkDir(dir string, files map[string][]byte) error {
	var errs []error
	for _, name := range sortedNames(files) {
		path := filepath.Join(dir, name)
		got, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("read %s: %w", path, err))
			continue
		}
		if !bytes.Equal(got, files[name]) {
			errs = append(errs, fmt.Errorf("%s is stale", path))
		}
	}
	return errors.Join(errs...)
}

func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
