package parser

import (
	"os"
	"path/filepath"
	"strings"
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

func TestIsInTargetHeadersRequiresExactPath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "mlx", "ops.h")
	other := filepath.Join(root, "third_party", "ops.h")
	targetPaths := []string{target}
	headerDirs := map[string]bool{filepath.Dir(target): true}
	wrapper := filepath.Join(root, "wrapper.cpp")

	if !isInTargetHeaders(&clangLoc{File: target}, targetPaths, headerDirs, wrapper, "") {
		t.Fatalf("target path was not accepted")
	}
	if isInTargetHeaders(&clangLoc{File: other}, targetPaths, headerDirs, wrapper, "") {
		t.Fatalf("same basename in a different directory was accepted")
	}
	if !isInTargetHeaders(&clangLoc{}, targetPaths, headerDirs, wrapper, target) {
		t.Fatalf("current file fallback target path was not accepted")
	}
	if isInTargetHeaders(&clangLoc{}, targetPaths, headerDirs, wrapper, other) {
		t.Fatalf("current file fallback accepted same basename in a different directory")
	}
}

func TestCompileCommandArgsPrefersMatchingSourceAndFiltersBuildOutputs(t *testing.T) {
	root := t.TempDir()
	compileCommands := filepath.Join(root, "compile_commands.json")
	target := filepath.Join(root, "mlx", "ops.h")
	source := filepath.Join(root, "mlx", "ops.cpp")
	otherSource := filepath.Join(root, "mlx", "array.cpp")
	data := `[
  {
    "directory": "` + root + `",
    "command": "/usr/bin/c++ -DARRAY=1 -Iarray -std=gnu++20 -o array.o -c ` + otherSource + `",
    "file": "` + otherSource + `"
  },
  {
    "directory": "` + root + `",
    "command": "/usr/bin/c++ -DOPS=1 -Iops -std=gnu++20 -o ops.o -c ` + source + `",
    "file": "` + source + `"
  }
]`
	if err := os.WriteFile(compileCommands, []byte(data), 0o666); err != nil {
		t.Fatal(err)
	}

	args, err := compileCommandArgs(compileCommands, []string{target})
	if err != nil {
		t.Fatalf("compileCommandArgs: %v", err)
	}
	for _, want := range []string{"-DOPS=1", "-Iops", "-std=gnu++20"} {
		if !hasArg(args, want) {
			t.Fatalf("compile args = %#v, missing %q", args, want)
		}
	}
	for _, unwanted := range []string{"-DARRAY=1", "-o", "ops.o", "-c", source} {
		if hasArg(args, unwanted) {
			t.Fatalf("compile args = %#v, unexpectedly contains %q", args, unwanted)
		}
	}
}

func TestClangASTArgsUsesCompileCommands(t *testing.T) {
	oldCompileCommandsPath := CompileCommandsPath
	oldIncludePaths := append([]string(nil), IncludePaths...)
	t.Cleanup(func() {
		SetCompileCommandsPath(oldCompileCommandsPath)
		SetIncludePaths(oldIncludePaths)
	})

	root := t.TempDir()
	compileCommands := filepath.Join(root, "compile_commands.json")
	target := filepath.Join(root, "mlx", "ops.h")
	source := filepath.Join(root, "mlx", "ops.cpp")
	data := `[{
  "directory": "` + root + `",
  "arguments": ["/usr/bin/c++", "-DOPS=1", "-Iops", "-std=gnu++20", "-o", "ops.o", "-c", "` + source + `"],
  "file": "` + source + `"
}]`
	if err := os.WriteFile(compileCommands, []byte(data), 0o666); err != nil {
		t.Fatal(err)
	}
	SetCompileCommandsPath(compileCommands)
	SetIncludePaths([]string{filepath.Join(root, "include")})

	args, err := clangASTArgs([]string{target})
	if err != nil {
		t.Fatalf("clangASTArgs: %v", err)
	}
	for _, want := range []string{"-Xclang", "-ast-dump=json", "-fsyntax-only", "-DOPS=1", "-Iops", "-std=gnu++20", "-x", "c++"} {
		if !hasArg(args, want) {
			t.Fatalf("clang args = %#v, missing %q", args, want)
		}
	}
	if strings.Count(strings.Join(args, "\x00"), "-std=") != 1 {
		t.Fatalf("clang args = %#v, want one -std flag", args)
	}
}

func TestClangASTArgsSortsHeaderIncludeDirs(t *testing.T) {
	oldCompileCommandsPath := CompileCommandsPath
	oldIncludePaths := append([]string(nil), IncludePaths...)
	t.Cleanup(func() {
		SetCompileCommandsPath(oldCompileCommandsPath)
		SetIncludePaths(oldIncludePaths)
	})
	SetCompileCommandsPath("")
	SetIncludePaths(nil)

	root := t.TempDir()
	target := filepath.Join(root, "a", "b", "c", "ops.h")
	args, err := clangASTArgs([]string{target})
	if err != nil {
		t.Fatalf("clangASTArgs: %v", err)
	}
	var dirs []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-I"+root) {
			dirs = append(dirs, strings.TrimPrefix(arg, "-I"))
		}
	}
	want := []string{
		root,
		filepath.Join(root, "a"),
		filepath.Join(root, "a", "b"),
		filepath.Join(root, "a", "b", "c"),
	}
	if strings.Join(dirs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("include dirs = %#v, want %#v", dirs, want)
	}
}

func TestClangDependencyArgsRemoveASTDumpFlags(t *testing.T) {
	args := []string{
		"-Xclang", "-ast-dump=json",
		"-fsyntax-only",
		"-DOPS=1",
		"-x", "c++",
	}
	got := clangDependencyArgs(args)
	for _, unwanted := range []string{"-Xclang", "-ast-dump=json", "-fsyntax-only"} {
		if hasArg(got, unwanted) {
			t.Fatalf("dependency args = %#v, unexpectedly contains %q", got, unwanted)
		}
	}
	for _, want := range []string{"-DOPS=1", "-x", "c++"} {
		if !hasArg(got, want) {
			t.Fatalf("dependency args = %#v, missing %q", got, want)
		}
	}
}

func TestParseMakeDeps(t *testing.T) {
	got := parseMakeDeps("mlxcgen.o: /tmp/wrapper.cpp \\\n /tmp/mlx/ops.h /tmp/mlx/array.h\n")
	want := []string{"/tmp/wrapper.cpp", "/tmp/mlx/ops.h", "/tmp/mlx/array.h"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("parseMakeDeps = %#v, want %#v", got, want)
	}
}

func TestASTCacheDepsFresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ops.h")
	if err := os.WriteFile(path, []byte("one"), 0o666); err != nil {
		t.Fatal(err)
	}
	deps, err := statASTCacheDeps([]string{path}, "")
	if err != nil {
		t.Fatalf("statASTCacheDeps: %v", err)
	}
	if !astCacheDepsFresh(deps) {
		t.Fatal("fresh dependency reported stale")
	}
	if err := os.WriteFile(path, []byte("two"), 0o666); err != nil {
		t.Fatal(err)
	}
	if astCacheDepsFresh(deps) {
		t.Fatal("modified dependency reported fresh")
	}
}

func TestStatASTCacheDepsSkipsWrapper(t *testing.T) {
	dir := t.TempDir()
	dep := filepath.Join(dir, "ops.h")
	wrapper := filepath.Join(dir, "wrapper.cpp")
	if err := os.WriteFile(dep, []byte("ops"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte("#include \"ops.h\"\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	deps, err := statASTCacheDeps([]string{wrapper, dep}, wrapper)
	if err != nil {
		t.Fatalf("statASTCacheDeps: %v", err)
	}
	if len(deps) != 1 || deps[0].Path != dep {
		t.Fatalf("deps = %#v, want only %s", deps, dep)
	}
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func namespace(name string, inner ...clangNode) clangNode {
	return clangNode{
		Kind:  "NamespaceDecl",
		Name:  name,
		Inner: inner,
	}
}
