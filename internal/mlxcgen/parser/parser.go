// Package parser provides C++ header parsing for MLX binding generation using clang AST JSON.
package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IncludePaths holds include paths for parsing.
var IncludePaths []string

// PreIncludes holds headers to include before the target headers.
// This is needed for headers that don't include their dependencies.
var PreIncludes []string

// SetIncludePaths sets the include paths to use when parsing headers.
func SetIncludePaths(paths []string) {
	IncludePaths = paths
}

// SetPreIncludes sets headers to include before target headers.
func SetPreIncludes(includes []string) {
	PreIncludes = includes
}

// Function represents a parsed C++ function declaration.
type Function struct {
	Name         string
	Namespace    string
	ReturnType   string
	ParamTypes   []string
	ParamNames   []string
	ParamDefault []string
	Doc          string // Doxygen comment
	Variant      string // Set later by variant selection
}

// Enum represents a parsed C++ enum declaration.
type Enum struct {
	Name      string
	Namespace string
	Values    []string
}

// ParseResult holds the results of parsing a header file.
type ParseResult struct {
	Functions map[string][]*Function // namespace::name -> list of overloads
	Enums     map[string]*Enum       // namespace::name -> enum
}

// clangNode represents a node in the clang AST JSON.
type clangNode struct {
	ID                  string      `json:"id"`
	Kind                string      `json:"kind"`
	Name                string      `json:"name"`
	MangledName         string      `json:"mangledName"`
	Type                *clangType  `json:"type"`
	Inner               []clangNode `json:"inner"`
	Loc                 *clangLoc   `json:"loc"`
	Range               *clangRange `json:"range"`
	IsUsed              bool        `json:"isUsed"`
	ReferencedDecl      *clangRef   `json:"referencedDecl"`
	Value               interface{} `json:"value"` // Can be string, number, or bool
	ValueCategory       string      `json:"valueCategory"`
	StorageClass        string      `json:"storageClass"`
	Inline              bool        `json:"inline"`
	Constexpr           bool        `json:"constexpr"`
	QualType            string      `json:"qualType"`
	DesugaredQualType   string      `json:"desugaredQualType"`
	TagUsed             string      `json:"tagUsed"`
	CompleteDefinition  bool        `json:"completeDefinition"`
	FixedUnderlyingType *clangType  `json:"fixedUnderlyingType"`
	ScopedEnumTag       string      `json:"scopedEnumTag"`
	Text                string      `json:"text"` // For TextComment

}

type clangType struct {
	QualType          string `json:"qualType"`
	DesugaredQualType string `json:"desugaredQualType"`
}

type clangLoc struct {
	File         string       `json:"file"`
	Line         int          `json:"line"`
	Col          int          `json:"col"`
	Offset       int          `json:"offset"`
	IncludedFrom *clangIncLoc `json:"includedFrom"`
}

type clangIncLoc struct {
	File string `json:"file"`
}

type clangRange struct {
	Begin clangLoc `json:"begin"`
	End   clangLoc `json:"end"`
}

type clangRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// ParseFile parses a C++ header file using clang AST JSON.
func ParseFile(path string) (*ParseResult, error) {
	return ParseFiles([]string{path})
}

// ParseFiles parses multiple header files using clang AST JSON.
// Each file is parsed individually to preserve location information.
func ParseFiles(paths []string) (*ParseResult, error) {
	if len(paths) == 0 {
		return &ParseResult{
			Functions: make(map[string][]*Function),
			Enums:     make(map[string]*Enum),
		}, nil
	}

	result := &ParseResult{
		Functions: make(map[string][]*Function),
		Enums:     make(map[string]*Enum),
	}

	// Parse each file individually to preserve location information
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("getting absolute path for %s: %w", p, err)
		}

		// Get the AST JSON using clang for this single file
		astJSON, wrapperPath, err := runClangAST([]string{p})
		if err != nil {
			return nil, fmt.Errorf("running clang for %s: %w", p, err)
		}

		// Parse JSON
		var root clangNode
		if err := json.Unmarshal(astJSON, &root); err != nil {
			return nil, fmt.Errorf("parsing clang AST JSON for %s: %w", p, err)
		}

		// Extract the directory containing the header for filtering
		headerDirs := make(map[string]bool)
		headerDirs[filepath.Dir(absPath)] = true

		// Walk the AST for this file
		// Initialize currentFile to empty - it will be set by loc.File as we walk
		currentFile := ""
		walkAST(&root, "", result, []string{absPath}, headerDirs, wrapperPath, &currentFile)
	}

	// Deduplicate functions that differ only in namespace qualification of types
	// (e.g., "array" vs "mlx::core::array")
	for key, funcs := range result.Functions {
		result.Functions[key] = deduplicateFunctions(funcs)
	}

	return result, nil
}

// deduplicateFunctions removes duplicate functions that differ only in type namespace qualification.
func deduplicateFunctions(funcs []*Function) []*Function {
	seen := make(map[string]bool)
	var unique []*Function

	for _, f := range funcs {
		// Create a signature key from normalized parameter types
		sig := f.Name + "(" + strings.Join(f.ParamTypes, ",") + ")->" + f.ReturnType
		if !seen[sig] {
			seen[sig] = true
			unique = append(unique, f)
		}
	}

	return unique
}

// runClangAST runs clang to produce AST JSON for header files.
// Returns the AST JSON and the main target file path (for location matching).
func runClangAST(paths []string) ([]byte, string, error) {
	if len(paths) == 0 {
		return nil, "", fmt.Errorf("no paths provided")
	}

	// Convert to absolute paths
	var absPaths []string
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			return nil, "", fmt.Errorf("getting absolute path for %s: %w", p, err)
		}
		absPaths = append(absPaths, absPath)
	}

	// Always create a wrapper to ensure proper type resolution
	// Include pre-includes first (e.g., mlx/array.h for type definitions)
	// then include the target headers
	var wrapper strings.Builder
	for _, inc := range PreIncludes {
		wrapper.WriteString(fmt.Sprintf("#include \"%s\"\n", inc))
	}
	for _, p := range absPaths {
		wrapper.WriteString(fmt.Sprintf("#include \"%s\"\n", p))
	}

	tmpFile, err := os.CreateTemp("", "mlxcgen-*.cpp")
	if err != nil {
		return nil, "", err
	}
	mainFile := tmpFile.Name()

	if _, err := tmpFile.WriteString(wrapper.String()); err != nil {
		tmpFile.Close()
		os.Remove(mainFile)
		return nil, "", err
	}
	tmpFile.Close()
	defer os.Remove(mainFile)

	// Run clang with AST dump
	args := []string{
		"-Xclang", "-ast-dump=json",
		"-fsyntax-only",
		"-std=c++20",
		"-x", "c++", // Always c++ since we use a .cpp wrapper
	}

	// Add configured include paths (e.g., MLX source directories)
	for _, inc := range IncludePaths {
		args = append(args, "-I"+inc)
	}

	// Add include paths based on input file locations
	includeDirs := make(map[string]bool)
	for _, p := range absPaths {
		// Add the directory containing the header and its parents
		dir := filepath.Dir(p)
		for dir != "/" && dir != "." {
			includeDirs[dir] = true
			dir = filepath.Dir(dir)
		}
	}
	for dir := range includeDirs {
		args = append(args, "-I"+dir)
	}

	args = append(args, mainFile)

	cmd := exec.Command("clang++", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	// Clang may return non-zero even on success with AST dump
	// Check if we got JSON output
	if stdout.Len() == 0 {
		if runErr != nil {
			return nil, "", fmt.Errorf("clang failed: %v\nstderr: %s", runErr, stderr.String())
		}
		return nil, "", fmt.Errorf("clang produced no output")
	}

	return stdout.Bytes(), mainFile, nil
}

// walkAST recursively walks the clang AST and extracts declarations.
// currentFile tracks the most recently seen file for location filtering.
func walkAST(node *clangNode, namespace string, result *ParseResult, targetPaths []string, headerDirs map[string]bool, wrapperPath string, currentFile *string) {
	// Track the current file context - update when we see a loc.File
	if node.Loc != nil && node.Loc.File != "" {
		*currentFile = node.Loc.File
	}

	switch node.Kind {
	case "NamespaceDecl":
		newNs := namespace
		if node.Name != "" {
			if newNs != "" {
				newNs += "::" + node.Name
			} else {
				newNs = node.Name
			}
		}
		for i := range node.Inner {
			walkAST(&node.Inner[i], newNs, result, targetPaths, headerDirs, wrapperPath, currentFile)
		}

	case "FunctionDecl":
		// Skip if not in a target header
		if !isInTargetHeaders(node.Loc, targetPaths, headerDirs, wrapperPath, *currentFile) {
			return
		}
		// Skip operator overloads
		if strings.HasPrefix(node.Name, "operator") {
			return
		}
		// Skip compiler builtins
		if strings.HasPrefix(node.Name, "__") {
			return
		}
		// Skip if no type info
		if node.Type == nil {
			return
		}

		f := extractFunction(node, namespace)
		if f != nil {
			// Skip Stream return type
			if f.ReturnType == "Stream" {
				return
			}
			// Skip template functions (have template type parameters like T, U)
			if isTemplateFunction(f) {
				return
			}

			key := namespace + "::" + f.Name
			result.Functions[key] = append(result.Functions[key], f)
		}

	case "FullComment":
		// This case might not be reached directly if comments are attached to Decls
		// but we handle inner comments in extractFunction

	case "EnumDecl":
		if !isInTargetHeaders(node.Loc, targetPaths, headerDirs, wrapperPath, *currentFile) {
			return
		}
		if node.Name == "" {
			return
		}
		// Skip system/standard library enums
		if strings.HasPrefix(namespace, "std") {
			return
		}

		e := extractEnum(node, namespace)
		if e != nil {
			key := namespace + "::" + e.Name
			result.Enums[key] = e
		}

	case "CXXRecordDecl", "ClassTemplateDecl", "TypedefDecl", "TypeAliasDecl":
		// Skip these but process their children for nested namespaces
		for i := range node.Inner {
			walkAST(&node.Inner[i], namespace, result, targetPaths, headerDirs, wrapperPath, currentFile)
		}

	default:
		// Recurse into children for TranslationUnitDecl and other containers
		for i := range node.Inner {
			walkAST(&node.Inner[i], namespace, result, targetPaths, headerDirs, wrapperPath, currentFile)
		}
	}
}

// isInTargetHeaders checks if a location is in one of the target headers.
// mainFilePath is the file passed to clang (either a single header or a wrapper).
// currentFile is the most recently seen file from loc.File, used as context when loc.File is empty.
func isInTargetHeaders(loc *clangLoc, targetPaths []string, headerDirs map[string]bool, mainFilePath string, currentFile string) bool {
	// Determine the effective file - either from loc.File or from tracked context
	effectiveFile := currentFile
	if loc != nil && loc.File != "" {
		effectiveFile = loc.File
	}

	// Check if the effective file matches a target path
	for _, p := range targetPaths {
		if effectiveFile == p || strings.HasSuffix(effectiveFile, "/"+filepath.Base(p)) {
			return true
		}
	}

	return false
}

// extractFunction extracts a Function from a FunctionDecl node.
func extractFunction(node *clangNode, namespace string) *Function {
	if node.Type == nil {
		return nil
	}

	// Parse the function type string
	// Format is typically: "returnType (paramType1, paramType2, ...)"
	typeStr := node.Type.QualType
	if node.Type.DesugaredQualType != "" {
		typeStr = node.Type.DesugaredQualType
	}

	// Find the parameter list start - must be outside angle brackets
	// e.g., for "std::function<array(array)> (int)" we want the last "(" not inside <>
	parenIdx := findFunctionParenthesis(typeStr)
	if parenIdx == -1 {
		return nil
	}

	returnType := strings.TrimSpace(typeStr[:parenIdx])

	// Extract parameter information from inner nodes
	var paramTypes, paramNames, paramDefaults []string
	for _, inner := range node.Inner {
		if inner.Kind == "ParmVarDecl" {
			pt := ""
			if inner.Type != nil {
				pt = inner.Type.QualType
			}
			paramTypes = append(paramTypes, normalizeType(pt))
			paramNames = append(paramNames, inner.Name)

			// Check for default value
			hasDefault := false
			for _, innerInner := range inner.Inner {
				if isExprNode(innerInner.Kind) {
					hasDefault = true
					break
				}
			}
			if hasDefault {
				paramDefaults = append(paramDefaults, "default") // Mark as having default
			} else {
				paramDefaults = append(paramDefaults, "")
			}
		}
	}

	return &Function{
		Name:         node.Name,
		Namespace:    namespace,
		ReturnType:   normalizeType(returnType),
		ParamTypes:   paramTypes,
		ParamNames:   paramNames,
		ParamDefault: paramDefaults,
		Doc:          extractDoc(node),
	}
}

// extractDoc extracts documentation from a node's children.
func extractDoc(node *clangNode) string {
	var docs []string
	for _, inner := range node.Inner {
		if inner.Kind == "FullComment" {
			for _, commentPart := range inner.Inner {
				if commentPart.Kind == "ParagraphComment" {
					for _, textPart := range commentPart.Inner {
						if textPart.Kind == "TextComment" {
							docs = append(docs, strings.TrimSpace(textPart.Text))
						}
					}
				}
			}
		}
	}
	return strings.Join(docs, "\n")

}

// isTemplateFunction returns true if the function has template type parameters.
// Template functions have single-letter type names or contain template syntax.
func isTemplateFunction(f *Function) bool {
	templateTypes := map[string]bool{
		"T": true, "U": true, "V": true, "W": true,
		"T1": true, "T2": true, "T3": true, "T4": true,
		"...": true,
	}

	// Check return type
	if templateTypes[f.ReturnType] {
		return true
	}

	// Check parameter types
	for _, pt := range f.ParamTypes {
		if templateTypes[pt] {
			return true
		}
		// Also check for template syntax like std::enable_if, etc.
		if strings.Contains(pt, "enable_if") || strings.Contains(pt, "type-parameter") {
			return true
		}
	}
	return false
}

// isExprNode returns true if the kind represents an expression (potential default value).
func isExprNode(kind string) bool {
	exprKinds := map[string]bool{
		"IntegerLiteral":           true,
		"FloatingLiteral":          true,
		"StringLiteral":            true,
		"CXXBoolLiteralExpr":       true,
		"CXXNullPtrLiteralExpr":    true,
		"CXXDefaultArgExpr":        true,
		"CXXConstructExpr":         true,
		"CXXTemporaryObjectExpr":   true,
		"CallExpr":                 true,
		"DeclRefExpr":              true,
		"ImplicitCastExpr":         true,
		"MaterializeTemporaryExpr": true,
		"ExprWithCleanups":         true,
		"InitListExpr":             true,
		"UnaryOperator":            true,
		"BinaryOperator":           true,
	}
	return exprKinds[kind]
}

// extractEnum extracts an Enum from an EnumDecl node.
func extractEnum(node *clangNode, namespace string) *Enum {
	var values []string
	for _, inner := range node.Inner {
		if inner.Kind == "EnumConstantDecl" && inner.Name != "" {
			values = append(values, inner.Name)
		}
	}

	return &Enum{
		Name:      node.Name,
		Namespace: namespace,
		Values:    values,
	}
}

// findFunctionParenthesis finds the index of the '(' that starts the function parameter list.
// This handles cases like "std::function<array(array)> (int)" where the first '(' is inside a template.
func findFunctionParenthesis(typeStr string) int {
	depth := 0
	for i, c := range typeStr {
		switch c {
		case '<':
			depth++
		case '>':
			depth--
		case '(':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// normalizeType normalizes C++ type names for consistent lookup.
func normalizeType(t string) string {
	t = strings.TrimSpace(t)

	// Remove const at the beginning
	if strings.HasPrefix(t, "const ") {
		t = strings.TrimPrefix(t, "const ")
	}

	// Remove reference at the end
	t = strings.TrimSuffix(t, " &")
	t = strings.TrimSuffix(t, "&")

	// Normalize spacing around punctuation for consistent matching
	// Remove space before >
	t = strings.ReplaceAll(t, " >", ">")
	// Remove space before &
	t = strings.ReplaceAll(t, " &", "&")
	// Remove space after <
	t = strings.ReplaceAll(t, "< ", "<")
	// Remove space before (
	t = strings.ReplaceAll(t, " (", "(")
	// Remove space after )
	t = strings.ReplaceAll(t, ") ", ")")

	// Normalize multiple spaces to single
	t = strings.Join(strings.Fields(t), " ")

	// Common type normalizations
	t = strings.ReplaceAll(t, "class ", "")
	t = strings.ReplaceAll(t, "struct ", "")

	// Remove mlx::core:: namespace prefix for MLX types
	t = strings.ReplaceAll(t, "mlx::core::", "")

	return strings.TrimSpace(t)
}

// ParseString parses C++ header content directly (without clang).
// This is a fallback for when clang isn't available.
func ParseString(content string) (*ParseResult, error) {
	// Create a temp file and use clang
	tmpFile, err := os.CreateTemp("", "mlxcgen-*.h")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return nil, err
	}
	tmpFile.Close()

	return ParseFile(tmpFile.Name())
}
