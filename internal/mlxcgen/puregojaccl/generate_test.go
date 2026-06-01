package puregojaccl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/apilock"
)

func TestGenerateRegistersEveryJACCLFunction(t *testing.T) {
	lock := readAPILock(t)
	files, err := Generate(lock, Options{})
	if err != nil {
		t.Fatal(err)
	}
	functions := string(files["functions.gen.go"])
	for _, fn := range lock.Targets["jacclc"].Functions {
		want := "registerLibFunc(&_" + fn.Name
		if !strings.Contains(functions, want) {
			t.Fatalf("generated functions missing registration for %s", fn.Name)
		}
	}
	for _, want := range []string{
		"func AllSum(group Group, input unsafe.Pointer, output unsafe.Pointer, nBytes uint, dtype DType) error",
		"func Init(strict bool) (Group, error)",
		"func ConfigSetCoordinator(config Config, coordinator string) error",
		"runtime.LockOSThread()",
		"func NewConfig() (Config, error)",
		"return Group{}, nil",
		"func (config Config) Close() error",
		"if config.IsNil()",
		"func (dtype DType) Size() (uint, error)",
		"func (group Group) AllSum(input unsafe.Pointer, output unsafe.Pointer, nBytes uint, dtype DType) error",
		"func (group Group) Recv(output unsafe.Pointer, nBytes uint, src int) error",
		"func bytesPointer(b []byte) unsafe.Pointer",
		"func allGatherBytesLen(size, elemLen int) (int, error)",
		"func (group Group) AllSumBytes(input, output []byte, dtype DType) error",
		"func (group Group) AllGatherBytes(input, output []byte) error",
		"func (group Group) SendBytes(input []byte, dst int) error",
		"setCachedPreferRing(config, prefer)",
	} {
		if !strings.Contains(functions, want) {
			t.Fatalf("generated functions missing %q", want)
		}
	}
}

func TestGenerateTypes(t *testing.T) {
	files, err := Generate(readAPILock(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	types := string(files["types.gen.go"])
	for _, want := range []string{"type Group struct", "type Config struct", "DTypeFloat32", "DType = 11"} {
		if !strings.Contains(types, want) {
			t.Fatalf("generated types missing %q", want)
		}
	}
}

func readAPILock(t *testing.T) *apilock.Lock {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "codegen", "mlxc-capi.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock apilock.Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	return &lock
}
