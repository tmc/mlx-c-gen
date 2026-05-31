// Package variants handles function overload disambiguation for MLX C bindings.
package variants

import (
	"fmt"
	"strings"
)

// VariantSuffix indicates what suffix (if any) to use for a function variant.
// nil means skip this variant, empty string means use base name, non-empty means use as suffix.
type VariantSuffix *string

func suffix(s string) VariantSuffix {
	return &s
}

func skip() VariantSuffix {
	return nil
}

// Func represents a parsed C++ function for variant selection.
type Func struct {
	Name         string
	Namespace    string
	ReturnType   string
	ParamTypes   []string
	ParamNames   []string
	ParamDefault []string
	Variant      string // Set by variant selection
	VariantIndex int    // Index in variant list for sorting
	File         string
	Line         int
	Col          int
}

// Diagnostic records a variant-selection decision that excludes a function.
type Diagnostic struct {
	Code    string
	Message string
	Func    *Func
}

// PrettyString returns a human-readable function signature.
func (f *Func) PrettyString() string {
	var parts []string
	parts = append(parts, f.ReturnType)
	parts = append(parts, f.Namespace+"::"+f.Name)
	parts = append(parts, "(")

	var args []string
	for i := range f.ParamTypes {
		name := ""
		if i < len(f.ParamNames) {
			name = f.ParamNames[i]
		}
		args = append(args, f.ParamTypes[i]+" "+name)
	}
	parts = append(parts, strings.Join(args, ", "))
	parts = append(parts, ")")
	return strings.Join(parts, " ")
}

// variantMappings maps namespace -> function name -> list of variant suffixes.
var variantMappings = map[string]map[string][]VariantSuffix{
	"mlx_core": {
		"arange":            {suffix(""), skip(), skip(), skip(), skip(), skip(), skip(), skip(), skip()},
		"eye":               {suffix(""), skip(), skip(), skip(), skip()},
		"ones":              {suffix(""), skip()},
		"zeros":             {suffix(""), skip()},
		"atleast_1d":        {suffix(""), skip()},
		"atleast_2d":        {suffix(""), skip()},
		"atleast_3d":        {suffix(""), skip()},
		"tri":               {suffix(""), skip()},
		"flatten":           {suffix(""), skip()},
		"squeeze":           {suffix("axes"), suffix("axis"), suffix("")},
		"expand_dims":       {suffix("axes"), suffix("")},
		"slice":             {suffix(""), skip(), suffix("dynamic"), skip()},
		"slice_update":      {suffix(""), skip(), suffix("dynamic")},
		"slice_update_add":  {suffix(""), skip()},
		"slice_update_max":  {suffix(""), skip()},
		"slice_update_min":  {suffix(""), skip()},
		"slice_update_prod": {suffix(""), skip()},
		"split":             {suffix(""), suffix("sections"), skip(), skip()},
		"concatenate":       {suffix("axis"), suffix("")},
		"conv_general":      {suffix(""), skip()},
		"stack":             {suffix("axis"), suffix("")},
		"full":              {suffix(""), skip()},
		"full_like":         {suffix(""), skip()},
		"repeat":            {suffix("axis"), suffix("")},
		"cummax":            {suffix(""), skip()},
		"cumsum":            {suffix(""), skip()},
		"cummin":            {suffix(""), skip()},
		"cumprod":           {suffix(""), skip()},
		"transpose":         {suffix("axes"), skip(), suffix("")},
		"all":               {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"any":               {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"sum":               {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"mean":              {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"median":            {suffix(""), skip(), skip(), skip()},
		"identity":          {suffix(""), skip()},
		"var":               {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"std":               {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"prod":              {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"max":               {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"min":               {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"argmax":            {suffix("axis"), suffix(""), skip()},
		"argmin":            {suffix("axis"), suffix(""), skip()},
		"load":              {suffix("reader"), suffix("")},
		"load_safetensors":  {suffix("reader"), suffix("")},
		"pad":               {suffix(""), skip(), skip(), suffix("symmetric")},
		"save":              {suffix("writer"), suffix("")},
		"save_safetensors":  {suffix("writer"), suffix("")},
		"gather":            {suffix(""), suffix("single")},
		"scatter":           {suffix(""), suffix("single")},
		"scatter_add":       {suffix(""), suffix("single")},
		"scatter_min":       {suffix(""), suffix("single")},
		"scatter_prod":      {suffix(""), suffix("single")},
		"scatter_max":       {suffix(""), suffix("single")},
		"argpartition":      {suffix("axis"), suffix("")},
		"partition":         {suffix("axis"), suffix("")},
		"argsort":           {suffix("axis"), suffix("")},
		"sort":              {suffix("axis"), suffix("")},
		"topk":              {suffix("axis"), suffix("")},
		"take":              {suffix("axis"), skip(), suffix(""), skip()},
		"roll":              {skip(), skip(), suffix("axis"), suffix("axes"), skip(), suffix("")},
		"logcumsumexp":      {suffix(""), skip()},
		"logsumexp":         {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"softmax":           {suffix("axes"), suffix("axis"), suffix("")},
		"tensordot":         {suffix(""), suffix("axis")},
		"array_equal":       {suffix(""), skip()},
		"round":             {suffix(""), skip()},
		"trace":             {suffix(""), skip(), skip()},
		"async_eval":        {suffix(""), skip()},
		"eval":              {suffix(""), skip()},
		"compile":           {suffix(""), skip(), skip()},
		"grad":              {skip(), skip(), skip()},
		"jvp":               {suffix(""), skip()},
		"value_and_grad":    {suffix(""), skip(), skip(), skip(), skip()},
		"vjp":               {suffix(""), skip()},
		"vmap":              {skip(), skip(), skip()},
		"export_function":   {skip(), suffix(""), suffix("kwargs")},
		"export_to_dot":     {suffix(""), skip(), skip(), skip()},
		"print_graph":       {suffix(""), skip(), skip(), skip()},
	},
	"mlx_core_linalg": {
		"norm": {suffix(""), skip(), suffix("matrix"), skip(), suffix("l2"), skip()},
		"svd":  {suffix(""), skip()},
	},
	"mlx_core_random": {
		"bernoulli":        {suffix(""), skip(), skip()},
		"bits":             {suffix(""), skip()},
		"categorical":      {suffix("shape"), suffix("num_samples"), suffix("")},
		"laplace":          {suffix(""), skip(), skip(), skip()},
		"permutation":      {suffix(""), suffix("arange")},
		"split":            {suffix("num"), suffix("")},
		"uniform":          {suffix(""), skip(), skip()},
		"normal":           {suffix("broadcast"), suffix(""), skip(), skip(), skip()},
		"truncated_normal": {suffix(""), skip()},
	},
	"mlx_core_fast": {
		"rope": {suffix(""), suffix("dynamic")},
	},
	"mlx_core_metal": {
		"device_info": {skip()},
	},
	"mlx_core_fft": {
		"fft":       {suffix(""), skip()},
		"fft2":      {suffix(""), skip()},
		"fftn":      {suffix(""), skip(), skip()},
		"fftshift":  {suffix(""), skip()},
		"ifft":      {suffix(""), skip()},
		"ifft2":     {suffix(""), skip()},
		"ifftn":     {suffix(""), skip(), skip()},
		"ifftshift": {suffix(""), skip()},
		"irfft":     {suffix(""), skip()},
		"irfft2":    {suffix(""), skip()},
		"irfftn":    {suffix(""), skip(), skip()},
		"rfft":      {suffix(""), skip()},
		"rfft2":     {suffix(""), skip()},
		"rfftn":     {suffix(""), skip(), skip()},
	},
}

// allowedDetailFuncs is the set of detail functions that should be exposed.
var allowedDetailFuncs = map[string]bool{
	"compile":             true,
	"compile_clear_cache": true,
	"compile_erase":       true,
	"vmap_replace":        true,
	"vmap_trace":          true,
}

// SelectVariants filters and assigns variant suffixes to function definitions.
// It returns the functions that should be included in the bindings.
func SelectVariants(namespace, name string, defs []*Func) ([]*Func, error) {
	selected, _, err := SelectVariantsWithDiagnostics(namespace, name, defs)
	return selected, err
}

// SelectVariantsWithDiagnostics filters and assigns variant suffixes to
// function definitions, and reports selected-out definitions.
func SelectVariantsWithDiagnostics(namespace, name string, defs []*Func) ([]*Func, []Diagnostic, error) {
	nsKey := strings.ReplaceAll(namespace, "::", "_")

	// Special handling for detail namespace
	if nsKey == "mlx_core_detail" {
		if !allowedDetailFuncs[name] {
			return nil, diagnosticsForSkipped("skip_unallowed_detail_function", defs), nil
		}
		if len(defs) > 0 {
			return []*Func{defs[0]}, nil, nil
		}
		return nil, nil, nil
	}

	variants, hasVariants := variantMappings[nsKey]
	if !hasVariants {
		if len(defs) > 1 {
			return nil, nil, unmappedOverloadError(namespace, name, defs)
		}
		if len(defs) > 0 {
			return []*Func{defs[0]}, nil, nil
		}
		return nil, nil, nil
	}

	funcVariants, hasFunc := variants[name]
	if !hasFunc {
		if len(defs) > 1 {
			return nil, nil, unmappedOverloadError(namespace, name, defs)
		}
		if len(defs) > 0 {
			return []*Func{defs[0]}, nil, nil
		}
		return nil, nil, nil
	}

	if len(funcVariants) != len(defs) {
		var b strings.Builder
		fmt.Fprintf(&b, "variant mapping length mismatch for %s::%s: got %d overloads, %d mappings", namespace, name, len(defs), len(funcVariants))
		fmt.Fprintf(&b, "\noverloads:")
		for i, d := range defs {
			fmt.Fprintf(&b, "\n  %d %s", i, d.PrettyString())
		}
		fmt.Fprintf(&b, "\nmappings:")
		for i, v := range funcVariants {
			s := "None"
			if v != nil {
				s = *v
			}
			fmt.Fprintf(&b, "\n  %d %s", i, s)
		}
		return nil, nil, fmt.Errorf("%s", b.String())
	}

	var result []*Func
	var diagnostics []Diagnostic
	variantIdx := 0
	for i, d := range defs {
		v := funcVariants[i]
		if v == nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:    "skip_variant_mapping",
				Message: fmt.Sprintf("%s skipped by variant mapping", d.PrettyString()),
				Func:    d,
			})
			continue
		}
		if *v != "" {
			d.Variant = *v
		}
		d.VariantIndex = variantIdx
		variantIdx++
		result = append(result, d)
	}

	return result, diagnostics, nil
}

func diagnosticsForSkipped(code string, defs []*Func) []Diagnostic {
	var diagnostics []Diagnostic
	for _, d := range defs {
		diagnostics = append(diagnostics, Diagnostic{
			Code:    code,
			Message: fmt.Sprintf("%s skipped by variant selection", d.PrettyString()),
			Func:    d,
		})
	}
	return diagnostics
}

func unmappedOverloadError(namespace, name string, defs []*Func) error {
	var b strings.Builder
	fmt.Fprintf(&b, "unmapped overloads for %s::%s", namespace, name)
	for i, d := range defs {
		fmt.Fprintf(&b, "\n  %d %s", i, d.PrettyString())
	}
	return fmt.Errorf("%s", b.String())
}
