package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
		"--compile-commands", "/build/compile_commands.json",
		"--inventory", "/repo/codegen/generated-files.txt",
		"--lock", "/repo/codegen/mlxc-capi.lock.json",
		"--lock-tu", "/repo/codegen/lock.c",
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
		got.CompileCommandsPath != "/build/compile_commands.json" ||
		got.InventoryPath != "/repo/codegen/generated-files.txt" ||
		opts.LockPath != "/repo/codegen/mlxc-capi.lock.json" ||
		opts.LockTUPath != "/repo/codegen/lock.c" ||
		opts.NM != "llvm-nm" ||
		got.WorkDir != "/tmp/work" ||
		got.ASTCacheDir != "/tmp/cache" ||
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

func TestParseCheckOptionsDefaults(t *testing.T) {
	opts, err := parseCheckOptions(nil)
	if err != nil {
		t.Fatalf("parseCheckOptions defaults: %v", err)
	}
	got := opts.Options
	if got.RepoRoot != "." ||
		got.InventoryPath != "codegen/generated-files.txt" ||
		got.MLXSrc != "" ||
		got.NoFormat ||
		got.KeepWork ||
		opts.LockPath != "codegen/mlxc-capi.lock.json" ||
		opts.LockTUPath != "codegen/lock.c" ||
		opts.NM != "nm" ||
		opts.StrictGenerated {
		t.Fatalf("defaults = %#v check = %#v", got, opts)
	}
	wantGenerator := []string{"go", "run", "./tools/mlx-c-gen"}
	if !reflect.DeepEqual(got.Generator, wantGenerator) {
		t.Fatalf("generator = %#v, want %#v", got.Generator, wantGenerator)
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
