package generators

import (
	"testing"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/parser"
)

func TestDiagnosticsReportsUnsupportedTypes(t *testing.T) {
	result := &parser.ParseResult{
		Functions: map[string][]*parser.Function{
			"mlx::core::bad_return": {{
				Name:       "bad_return",
				Namespace:  "mlx::core",
				ReturnType: "UnsupportedReturn",
				File:       "ops.h",
				Line:       12,
				Col:        3,
			}},
			"mlx::core::bad_param": {{
				Name:       "bad_param",
				Namespace:  "mlx::core",
				ReturnType: "void",
				ParamTypes: []string{"array", "UnsupportedParam"},
				ParamNames: []string{"x", "bad"},
				File:       "ops.h",
				Line:       13,
				Col:        3,
			}},
			"mlx::core::add": {{
				Name:       "add",
				Namespace:  "mlx::core",
				ReturnType: "array",
				ParamTypes: []string{"array", "array"},
				ParamNames: []string{"a", "b"},
				File:       "ops.h",
				Line:       14,
				Col:        3,
			}},
		},
		Enums: map[string]*parser.Enum{},
	}

	diagnostics := New().Diagnostics(result)
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want 2", diagnostics)
	}
	want := map[string]int{
		"skip_unsupported_return_type": 12,
		"skip_unsupported_param_type":  13,
	}
	for _, diagnostic := range diagnostics {
		line, ok := want[diagnostic.Code]
		if !ok {
			t.Fatalf("unexpected diagnostic: %#v", diagnostic)
		}
		if diagnostic.File != "ops.h" || diagnostic.Line != line || diagnostic.Col != 3 {
			t.Fatalf("diagnostic location = %#v, want ops.h:%d:3", diagnostic, line)
		}
		delete(want, diagnostic.Code)
	}
	if len(want) != 0 {
		t.Fatalf("missing diagnostics: %#v", want)
	}
}

func TestDiagnosticsSkipHookedFunctions(t *testing.T) {
	result := &parser.ParseResult{
		Functions: map[string][]*parser.Function{
			"mlx::core::fast::metal_kernel": {{
				Name:       "metal_kernel",
				Namespace:  "mlx::core::fast",
				ReturnType: "void",
				ParamTypes: []string{"Unsupported"},
				ParamNames: []string{"x"},
				File:       "fast.h",
				Line:       4,
				Col:        1,
			}},
		},
		Enums: map[string]*parser.Enum{},
	}

	if diagnostics := New().Diagnostics(result); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for hooked function", diagnostics)
	}
}
