package parser

import (
	"path/filepath"
	"testing"
)

func TestWalkASTRecordsSkippedFunctionDiagnostics(t *testing.T) {
	target := filepath.Join(t.TempDir(), "ops.h")
	tests := []struct {
		name string
		node clangNode
		code string
	}{
		{
			name: "operator",
			node: clangNode{
				Kind: "FunctionDecl",
				Name: "operator+",
				Type: &clangType{QualType: "array (array, array)"},
				Loc:  &clangLoc{File: target, Line: 7, Col: 2},
			},
			code: "skip_operator",
		},
		{
			name: "missing type",
			node: clangNode{
				Kind: "FunctionDecl",
				Name: "missing",
				Loc:  &clangLoc{File: target, Line: 8, Col: 2},
			},
			code: "skip_missing_type",
		},
		{
			name: "stream return",
			node: clangNode{
				Kind: "FunctionDecl",
				Name: "default_stream",
				Type: &clangType{QualType: "Stream ()"},
				Loc:  &clangLoc{File: target, Line: 9, Col: 2},
			},
			code: "skip_stream_return",
		},
		{
			name: "template function",
			node: clangNode{
				Kind: "FunctionDecl",
				Name: "identity",
				Type: &clangType{QualType: "T (T)"},
				Loc:  &clangLoc{File: target, Line: 10, Col: 2},
				Inner: []clangNode{{
					Kind: "ParmVarDecl",
					Name: "x",
					Type: &clangType{QualType: "T"},
				}},
			},
			code: "skip_template_function",
		},
		{
			name: "unparsed function",
			node: clangNode{
				Kind: "FunctionDecl",
				Name: "bad",
				Type: &clangType{QualType: "not a function type"},
				Loc:  &clangLoc{File: target, Line: 11, Col: 2},
			},
			code: "skip_unparsed_function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ParseResult{
				Functions: make(map[string][]*Function),
				Enums:     make(map[string]*Enum),
			}
			root := namespace("mlx", namespace("core", tt.node))
			currentFile := ""
			walkAST(&root, "", result, []string{target}, map[string]bool{
				filepath.Dir(target): true,
			}, filepath.Join(t.TempDir(), "wrapper.cpp"), &currentFile)
			if len(result.Functions) != 0 {
				t.Fatalf("functions = %#v, want none", result.Functions)
			}
			if len(result.Diagnostics) != 1 {
				t.Fatalf("diagnostics = %#v, want one", result.Diagnostics)
			}
			diag := result.Diagnostics[0]
			if diag.Code != tt.code {
				t.Fatalf("diagnostic code = %q, want %q", diag.Code, tt.code)
			}
			if diag.File != target || diag.Line == 0 || diag.Col == 0 {
				t.Fatalf("diagnostic location = %#v, want target file and line/column", diag)
			}
		})
	}
}

func TestWalkASTKeepsGeneratedFunctionWithoutDiagnostics(t *testing.T) {
	target := filepath.Join(t.TempDir(), "ops.h")
	result := &ParseResult{
		Functions: make(map[string][]*Function),
		Enums:     make(map[string]*Enum),
	}
	root := namespace("mlx", namespace("core", clangNode{
		Kind: "FunctionDecl",
		Name: "add",
		Type: &clangType{QualType: "array (array, array)"},
		Loc:  &clangLoc{File: target, Line: 12, Col: 2},
		Inner: []clangNode{
			{Kind: "ParmVarDecl", Name: "a", Type: &clangType{QualType: "array"}},
			{Kind: "ParmVarDecl", Name: "b", Type: &clangType{QualType: "array"}},
		},
	}))
	currentFile := ""
	walkAST(&root, "", result, []string{target}, map[string]bool{
		filepath.Dir(target): true,
	}, filepath.Join(t.TempDir(), "wrapper.cpp"), &currentFile)

	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
	funcs := result.Functions["mlx::core::add"]
	if len(funcs) != 1 {
		t.Fatalf("functions[mlx::core::add] = %#v, want one function", funcs)
	}
	if funcs[0].File != target || funcs[0].Line != 12 || funcs[0].Col != 2 {
		t.Fatalf("function location = %s:%d:%d, want %s:12:2", funcs[0].File, funcs[0].Line, funcs[0].Col, target)
	}
	if got := funcs[0].ParamTypes; len(got) != 2 || got[0] != "array" || got[1] != "array" {
		t.Fatalf("param types = %#v, want two array params", got)
	}
}

func namespace(name string, inner ...clangNode) clangNode {
	return clangNode{
		Kind:  "NamespaceDecl",
		Name:  name,
		Inner: inner,
	}
}
