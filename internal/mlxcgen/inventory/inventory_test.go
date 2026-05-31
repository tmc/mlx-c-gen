package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRejectsDuplicate(t *testing.T) {
	_, err := Read(strings.NewReader(`
generated_header_api mlxc mlx/c/ops.h
generated_header_api mlxc mlx/c/ops.h
`))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Read duplicate error = %v", err)
	}
}

func TestCheckCoversFilesAndUmbrella(t *testing.T) {
	root := t.TempDir()
	write(t, root, "CMakeLists.txt", `
set(mlxc-src
  ${CMAKE_CURRENT_LIST_DIR}/mlx/c/ops.cpp)
add_library(jacclc ${CMAKE_CURRENT_LIST_DIR}/mlx/c/jaccl.cpp)
`)
	write(t, root, "mlx/c/mlx.h", `#include "mlx/c/ops.h"`)
	write(t, root, "mlx/c/ops.h", "")
	write(t, root, "mlx/c/ops.cpp", "")
	write(t, root, "mlx/c/jaccl.h", "")
	write(t, root, "mlx/c/jaccl.cpp", "")
	write(t, root, "codegen/generated-files.txt", `
not_owned_by_codegen mlxc mlx/c/mlx.h
generated_header_api mlxc mlx/c/ops.h
generated_header_api mlxc mlx/c/ops.cpp
handwritten_runtime jacclc mlx/c/jaccl.h
handwritten_runtime jacclc mlx/c/jaccl.cpp
`)
	if err := Check(root, filepath.Join(root, "codegen/generated-files.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestCheckFailsForUnclassifiedFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "CMakeLists.txt", "")
	write(t, root, "mlx/c/mlx.h", "")
	write(t, root, "mlx/c/extra.h", "")
	write(t, root, "codegen/generated-files.txt", `not_owned_by_codegen mlxc mlx/c/mlx.h`)
	err := Check(root, filepath.Join(root, "codegen/generated-files.txt"))
	if err == nil || !strings.Contains(err.Error(), "mlx/c/extra.h is not classified") {
		t.Fatalf("Check error = %v", err)
	}
}

func write(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o666); err != nil {
		t.Fatal(err)
	}
}
