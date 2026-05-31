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

func TestGeneratedHeaders(t *testing.T) {
	got := GeneratedHeaders([]Spec{
		{Header: "mlx/c/ops.h"},
		{Header: "mlx/c/jaccl.h", Generate: GenerateSpec{Header: true}},
		{Header: "mlx/c/fft.h", Generate: GenerateSpec{Header: true}},
	})
	want := "mlx/c/fft.h\nmlx/c/jaccl.h"
	if strings.Join(got, "\n") != want {
		t.Fatalf("headers = %#v, want %s", got, want)
	}
}

func TestRenderHeader(t *testing.T) {
	spec := Spec{
		SchemaVersion: 1,
		Name:          "jaccl",
		Target:        "jacclc",
		Header:        "mlx/c/jaccl.h",
		Ownership:     "handwritten_runtime",
		Generate:      GenerateSpec{Header: true},
		Copyright:     "Copyright \u00a9 2026 Apple Inc.",
		IncludeGuard:  "MLX_JACCL_H",
		Includes:      []string{"stdbool.h"},
		Group: Group{
			Name:  "mlx_jaccl",
			Title: "JACCL",
			Doc:   "Standalone C API for libjaccl.",
		},
		Items: []Item{
			{
				Kind:   "struct",
				Name:   "mlx_jaccl_group",
				Action: "custom_spec",
				Reason: "runtime_handle",
				Doc:    "A JACCL communication group.",
				Opaque: true,
			},
			{
				Kind:      "function",
				Name:      "mlx_jaccl_is_available",
				Action:    "handwritten",
				Reason:    "optional_runtime",
				Doc:       "Check if JACCL is available on this system.",
				Signature: "bool mlx_jaccl_is_available(void)",
			},
		},
	}
	got, err := RenderHeader(spec)
	if err != nil {
		t.Fatalf("RenderHeader: %v", err)
	}
	for _, want := range []string{
		"#ifndef MLX_JACCL_H",
		"#include <stdbool.h>",
		"\\defgroup mlx_jaccl JACCL",
		"typedef struct mlx_jaccl_group_ { void* ctx; } mlx_jaccl_group;",
		"bool mlx_jaccl_is_available(void);",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("header missing %q:\n%s", want, got)
		}
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
			{Kind: "struct", Name: "mlx_jaccl_group", Action: "handwritten", Reason: "runtime_handle"},
			{Kind: "function", Name: "mlx_jaccl_group_free", Action: "handwritten", Reason: "runtime_lifetime", Signature: "int mlx_jaccl_group_free(mlx_jaccl_group group)"},
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
			{Kind: "function", Name: "mlx_jaccl_group_free", Action: "handwritten", Reason: "runtime_lifetime", Signature: "void mlx_jaccl_group_free(mlx_jaccl_group group)"},
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

func TestLoadRejectsUnknownTarget(t *testing.T) {
	_, err := Load(strings.NewReader(`
schema_version: 1
name: bad
target: other
header: mlx/c/jaccl.h
ownership: handwritten_runtime
items:
  - kind: function
    name: mlx_jaccl_group_free
    action: handwritten
    reason: runtime_lifetime
    signature: int mlx_jaccl_group_free(mlx_jaccl_group group)
`))
	if err == nil {
		t.Fatal("Load succeeded, want error")
	}
	if !strings.Contains(err.Error(), "unknown target other") {
		t.Fatalf("error = %v, want unknown target", err)
	}
}

func TestLoadRejectsMissingReason(t *testing.T) {
	_, err := Load(strings.NewReader(`
schema_version: 1
name: bad
target: jacclc
header: mlx/c/jaccl.h
ownership: handwritten_runtime
items:
  - kind: function
    name: mlx_jaccl_group_free
    action: handwritten
    signature: int mlx_jaccl_group_free(mlx_jaccl_group group)
`))
	if err == nil {
		t.Fatal("Load succeeded, want error")
	}
	if !strings.Contains(err.Error(), "missing reason") {
		t.Fatalf("error = %v, want missing reason", err)
	}
}

func TestLoadRejectsHeaderOutsideMLXC(t *testing.T) {
	for _, header := range []string{"include/jaccl.h", "mlx/c/../jaccl.h"} {
		_, err := Load(strings.NewReader(`
schema_version: 1
name: bad
target: jacclc
header: ` + header + `
ownership: handwritten_runtime
items:
  - kind: function
    name: mlx_jaccl_group_free
    action: handwritten
    reason: runtime_lifetime
    signature: int mlx_jaccl_group_free(mlx_jaccl_group group)
`))
		if err == nil {
			t.Fatalf("Load accepted header %q", header)
		}
		if !strings.Contains(err.Error(), "must be under mlx/c") {
			t.Fatalf("error = %v, want header error", err)
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
			{Kind: "function", Name: "mlx_jaccl_group_new", Action: "handwritten", Reason: "runtime_lifetime", Signature: "mlx_jaccl_group mlx_jaccl_group_new(void)"},
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
