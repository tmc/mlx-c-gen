package symbols

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/apilock"
)

func TestParseNM(t *testing.T) {
	got := ParseNM([]byte(`
00000001 T mlx_array_free
         U mlx_array_free
00000002 T _mlx_error
lib.a(obj.o):
`))
	for _, want := range []string{"mlx_array_free", "_mlx_error"} {
		if !got[want] {
			t.Fatalf("missing %s in %#v", want, got)
		}
	}
	if got["lib.a(obj.o):"] {
		t.Fatalf("archive header parsed as symbol")
	}
}

func TestParseNMIgnoresUndefinedOnlySymbols(t *testing.T) {
	got := ParseNM([]byte(`
         U mlx_missing
                 U _mlx_missing
`))
	if got["mlx_missing"] || got["_mlx_missing"] {
		t.Fatalf("undefined symbols parsed as defined: %#v", got)
	}
}

func TestParseSymbolList(t *testing.T) {
	got := ParseSymbolList([]byte("\nmlx_array_free\r\n_mlx_error  mlx_eval\n"))
	for _, want := range []string{"mlx_array_free", "_mlx_error", "mlx_eval"} {
		if !got[want] {
			t.Fatalf("missing %s in %#v", want, got)
		}
	}
}

func TestCheckReadsActualSymbolFile(t *testing.T) {
	dir := t.TempDir()
	lock := &apilock.Lock{
		SchemaVersion: apilock.SchemaVersion,
		Targets: map[string]apilock.Target{
			"jacclc": {
				Functions: []apilock.Function{{Name: "mlx_jaccl_group_free"}},
			},
		},
	}
	data, err := lock.JSON()
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "lock.json")
	if err := os.WriteFile(lockPath, data, 0o666); err != nil {
		t.Fatal(err)
	}
	actualPath := filepath.Join(dir, "actual.txt")
	if err := os.WriteFile(actualPath, []byte("_mlx_jaccl_group_free\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := Check(Options{
		LockPath: lockPath,
		Actuals:  []TargetSymbols{{Target: "jacclc", Path: actualPath}},
	}); err != nil {
		t.Fatalf("Check actual symbols: %v", err)
	}
}

func TestCheckTargetFindsMissingAndForbidden(t *testing.T) {
	target := apilock.Target{
		Functions: []apilock.Function{
			{Name: "mlx_array_free"},
			{Name: "_mlx_error"},
		},
	}
	problems := checkTarget("mlxc", target, map[string]bool{
		"_mlx_array_free":        true,
		"__mlx_error":            true,
		"mlx_jaccl_group_free":   true,
		"unrelated_cxx_symbol":   true,
		"_Z15mlx_array_free_foo": true,
	})
	text := strings.Join(problems, "\n")
	if strings.Contains(text, "missing") {
		t.Fatalf("unexpected missing problem: %s", text)
	}
	if !strings.Contains(text, "forbidden JACCL symbol") {
		t.Fatalf("missing forbidden problem: %s", text)
	}
}

func TestCheckTargetRejectsNonJACCLCAPISymbols(t *testing.T) {
	target := apilock.Target{
		Functions: []apilock.Function{{Name: "mlx_jaccl_group_free"}},
	}
	problems := checkTarget("jacclc", target, map[string]bool{
		"_mlx_jaccl_group_free": true,
		"mlx_array_free":        true,
		"_Z3foov":               true,
	})
	text := strings.Join(problems, "\n")
	if !strings.Contains(text, "forbidden non-JACCL C API symbol") {
		t.Fatalf("missing forbidden problem: %s", text)
	}
	if strings.Contains(text, "_Z3foov") {
		t.Fatalf("unexpected C++ symbol problem: %s", text)
	}
}
