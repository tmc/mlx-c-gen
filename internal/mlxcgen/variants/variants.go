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
		"arange":           {suffix(""), skip(), skip(), skip(), skip(), skip(), skip(), skip(), skip()},
		"eye":              {suffix(""), skip(), skip(), skip(), skip()},
		"tri":              {suffix(""), skip()},
		"flatten":          {suffix(""), skip()},
		"squeeze":          {suffix("axes"), suffix("axis"), suffix("")},
		"expand_dims":      {suffix("axes"), suffix("")},
		"slice":            {suffix(""), skip(), suffix("dynamic"), skip()},
		"slice_update":     {suffix(""), skip(), suffix("dynamic")},
		"split":            {suffix(""), suffix("sections"), skip(), skip()},
		"concatenate":      {suffix("axis"), suffix("")},
		"stack":            {suffix("axis"), suffix("")},
		"repeat":           {suffix("axis"), suffix("")},
		"transpose":        {suffix("axes"), skip(), suffix("")},
		"all":              {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"any":              {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"sum":              {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"mean":             {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"var":              {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"std":              {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"prod":             {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"max":              {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"min":              {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"argmax":           {suffix("axis"), suffix(""), skip()},
		"argmin":           {suffix("axis"), suffix(""), skip()},
		"load":             {suffix("reader"), suffix("")},
		"load_safetensors": {suffix("reader"), suffix("")},
		"pad":              {suffix(""), skip(), skip(), suffix("symmetric")},
		"save":             {suffix("writer"), suffix("")},
		"save_safetensors": {suffix("writer"), suffix("")},
		"gather":           {suffix(""), suffix("single")},
		"scatter":          {suffix(""), suffix("single")},
		"scatter_add":      {suffix(""), suffix("single")},
		"scatter_min":      {suffix(""), suffix("single")},
		"scatter_prod":     {suffix(""), suffix("single")},
		"scatter_max":      {suffix(""), suffix("single")},
		"argpartition":     {suffix("axis"), suffix("")},
		"partition":        {suffix("axis"), suffix("")},
		"argsort":          {suffix("axis"), suffix("")},
		"sort":             {suffix("axis"), suffix("")},
		"topk":             {suffix("axis"), suffix("")},
		"take":             {suffix("axis"), skip(), suffix(""), skip()},
		"roll":             {skip(), skip(), suffix("axis"), suffix("axes"), skip(), suffix("")},
		"logsumexp":        {suffix("axes"), suffix("axis"), suffix(""), skip()},
		"softmax":          {suffix("axes"), suffix("axis"), suffix("")},
		"tensordot":        {suffix(""), suffix("axis")},
		"array_equal":      {suffix(""), skip()},
		"round":            {suffix(""), skip()},
		"trace":            {suffix(""), skip(), skip()},
		"export_function":  {skip(), suffix(""), suffix("kwargs")},
	},
	"mlx_core_linalg": {
		"norm": {suffix(""), skip(), suffix("matrix"), skip(), suffix("l2"), skip()},
	},
	"mlx_core_random": {
		"categorical": {suffix("shape"), suffix("num_samples"), suffix("")},
		"permutation": {suffix(""), suffix("arange")},
		"split":       {suffix("num"), suffix("")},
		"uniform":     {suffix(""), skip(), skip()},
		"normal":      {suffix("broadcast"), suffix(""), skip(), skip(), skip()},
	},
	"mlx_core_fast": {
		"rope": {suffix(""), suffix("dynamic")},
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
func SelectVariants(namespace, name string, defs []*Func) []*Func {
	nsKey := strings.ReplaceAll(namespace, "::", "_")

	// Special handling for detail namespace
	if nsKey == "mlx_core_detail" {
		if !allowedDetailFuncs[name] {
			return nil
		}
		if len(defs) > 0 {
			return []*Func{defs[0]}
		}
		return nil
	}

	variants, hasVariants := variantMappings[nsKey]
	if !hasVariants {
		// No variant mapping for this namespace - take first definition only
		if len(defs) > 1 {
			fmt.Printf("OVL\n")
			for i, d := range defs {
				v := ""
				if i > 0 {
					v = "None"
				}
				fmt.Printf("OVL %d %s  ->  %s\n", i, d.PrettyString(), v)
			}
		}
		if len(defs) > 0 {
			return []*Func{defs[0]}
		}
		return nil
	}

	funcVariants, hasFunc := variants[name]
	if !hasFunc {
		// No variant mapping for this function - take first definition only
		if len(defs) > 1 {
			fmt.Printf("OVL\n")
			for i, d := range defs {
				v := ""
				if i > 0 {
					v = "None"
				}
				fmt.Printf("OVL %d %s  ->  %s\n", i, d.PrettyString(), v)
			}
		}
		if len(defs) > 0 {
			return []*Func{defs[0]}
		}
		return nil
	}

	if len(funcVariants) != len(defs) {
		fmt.Printf("function overloads length: %d\n", len(defs))
		for i, d := range defs {
			fmt.Printf("%d %s\n", i, d.PrettyString())
		}
		fmt.Printf("namings length: %d\n", len(funcVariants))
		for i, v := range funcVariants {
			s := "None"
			if v != nil {
				s = *v
			}
			fmt.Printf("%d %s\n", i, s)
		}
		panic("function overloads and namings do not match")
	}

	if len(defs) > 1 {
		fmt.Printf("OVL\n")
	}

	var result []*Func
	variantIdx := 0
	for i, d := range defs {
		v := funcVariants[i]
		if v == nil {
			// Skip this variant
			if len(defs) > 1 {
				fmt.Printf("OVL %d %s  ->  None\n", i, d.PrettyString())
			}
			continue
		}
		if len(defs) > 1 {
			fmt.Printf("OVL %d %s  ->  %s\n", i, d.PrettyString(), *v)
		}
		if *v != "" {
			d.Variant = *v
		}
		d.VariantIndex = variantIdx
		variantIdx++
		result = append(result, d)
	}

	return result
}
