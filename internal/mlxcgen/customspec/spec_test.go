package customspec

import (
	"strings"
	"testing"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/apilock"
)

func TestLoad(t *testing.T) {
	spec, err := Load(strings.NewReader(`
schema_version: 1
name: jaccl
target: jacclc
header: mlx/c/jaccl.h
ownership: handwritten_runtime
items:
  - kind: struct
    name: mlx_jaccl_group
    action: handwritten
    reason: runtime_handle
  - kind: function
    name: mlx_jaccl_group_free
    action: handwritten
    reason: runtime_lifetime
    signature: int mlx_jaccl_group_free(mlx_jaccl_group group)
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if spec.Name != "jaccl" || spec.Target != "jacclc" || len(spec.Items) != 2 {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestCheckLock(t *testing.T) {
	lock := &apilock.Lock{
		Targets: map[string]apilock.Target{
			"jacclc": {
				Structs: []apilock.Struct{{
					Name:   "mlx_jaccl_group",
					Header: "mlx/c/jaccl.h",
				}},
				Functions: []apilock.Function{{
					Name:      "mlx_jaccl_group_free",
					Header:    "mlx/c/jaccl.h",
					Signature: "int mlx_jaccl_group_free(mlx_jaccl_group group)",
				}},
			},
		},
	}
	specs := []Spec{{
		SchemaVersion: 1,
		Name:          "jaccl",
		Target:        "jacclc",
		Header:        "mlx/c/jaccl.h",
		Ownership:     "handwritten_runtime",
		Items: []Item{
			{Kind: "struct", Name: "mlx_jaccl_group", Action: "handwritten"},
			{Kind: "function", Name: "mlx_jaccl_group_free", Action: "handwritten", Signature: "int mlx_jaccl_group_free(mlx_jaccl_group group)"},
		},
	}}
	if err := CheckLock(lock, specs); err != nil {
		t.Fatalf("CheckLock: %v", err)
	}
}

func TestCheckLockReportsSignatureMismatch(t *testing.T) {
	lock := &apilock.Lock{
		Targets: map[string]apilock.Target{
			"jacclc": {
				Functions: []apilock.Function{{
					Name:      "mlx_jaccl_group_free",
					Header:    "mlx/c/jaccl.h",
					Signature: "int mlx_jaccl_group_free(mlx_jaccl_group group)",
				}},
			},
		},
	}
	specs := []Spec{{
		SchemaVersion: 1,
		Name:          "jaccl",
		Target:        "jacclc",
		Header:        "mlx/c/jaccl.h",
		Ownership:     "handwritten_runtime",
		Items: []Item{
			{Kind: "function", Name: "mlx_jaccl_group_free", Action: "handwritten", Signature: "void mlx_jaccl_group_free(mlx_jaccl_group group)"},
		},
	}}
	err := CheckLock(lock, specs)
	if err == nil {
		t.Fatal("CheckLock succeeded, want error")
	}
	if !strings.Contains(err.Error(), "signature =") {
		t.Fatalf("error = %v, want signature mismatch", err)
	}
}

func TestLoadRejectsUnknownKindAndAction(t *testing.T) {
	_, err := Load(strings.NewReader(`
schema_version: 1
name: bad
target: jacclc
header: mlx/c/jaccl.h
ownership: handwritten_runtime
items:
  - kind: thing
    name: mlx_jaccl_group
    action: magic
`))
	if err == nil {
		t.Fatal("Load succeeded, want error")
	}
	for _, want := range []string{"unknown kind thing", "unknown action magic"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestCheckLockReportsMissingAndExtra(t *testing.T) {
	lock := &apilock.Lock{
		Targets: map[string]apilock.Target{
			"jacclc": {
				Functions: []apilock.Function{{
					Name:   "mlx_jaccl_group_free",
					Header: "mlx/c/jaccl.h",
				}},
			},
		},
	}
	specs := []Spec{{
		SchemaVersion: 1,
		Name:          "jaccl",
		Target:        "jacclc",
		Header:        "mlx/c/jaccl.h",
		Ownership:     "handwritten_runtime",
		Items: []Item{
			{Kind: "function", Name: "mlx_jaccl_group_new", Action: "handwritten", Signature: "mlx_jaccl_group mlx_jaccl_group_new(void)"},
		},
	}}
	err := CheckLock(lock, specs)
	if err == nil {
		t.Fatal("CheckLock succeeded, want error")
	}
	for _, want := range []string{
		"missing custom spec item function:mlx_jaccl_group_free",
		"custom spec item function:mlx_jaccl_group_new is not in API lock",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}
