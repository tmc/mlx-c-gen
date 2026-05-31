package generators

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/parser"
)

func TestGenerateYamlIncludesParserDiagnostics(t *testing.T) {
	result := &parser.ParseResult{
		Functions: make(map[string][]*parser.Function),
		Enums:     make(map[string]*parser.Enum),
		Diagnostics: []parser.Diagnostic{{
			Code:    "skip_template_function",
			Message: "mlx::core::identity uses template parameters",
			File:    "/tmp/ops.h",
			Line:    12,
			Col:     3,
		}},
	}

	var buf bytes.Buffer
	if err := NewYaml().GenerateYaml(&buf, result); err != nil {
		t.Fatalf("GenerateYaml: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"diagnostics:",
		"code: skip_template_function",
		"message: mlx::core::identity uses template parameters",
		"file: /tmp/ops.h",
		"line: 12",
		"col: 3",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("metadata missing %q:\n%s", want, out)
		}
	}
}
