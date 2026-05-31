package main

import (
	"os"
	"path/filepath"
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
