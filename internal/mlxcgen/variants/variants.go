// Package variants handles function overload disambiguation for MLX C bindings.
package variants

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/plan"
)

// VariantSuffix indicates what suffix (if any) to use for a function variant.
// nil means skip this variant, empty string means use base name, non-empty means use as suffix.
type VariantSuffix *string

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

type variantPolicy struct {
	mappings           map[string]map[string][]VariantSuffix
	allowedDetailFuncs map[string]bool
}

var (
	defaultPolicyOnce sync.Once
	defaultPolicy     variantPolicy
	defaultPolicyErr  error
)

func loadDefaultPolicy() (variantPolicy, error) {
	defaultPolicyOnce.Do(func() {
		manifest, err := plan.Default()
		if err != nil {
			defaultPolicyErr = err
			return
		}
		defaultPolicy = policyFromManifest(manifest)
	})
	return defaultPolicy, defaultPolicyErr
}

func policyFromManifest(manifest plan.Manifest) variantPolicy {
	return variantPolicy{
		mappings:           variantMappingsFromManifest(manifest.VariantMappings),
		allowedDetailFuncs: manifest.AllowedDetailFunctionsSet(),
	}
}

func variantMappingsFromManifest(in map[string]map[string][]*string) map[string]map[string][]VariantSuffix {
	out := make(map[string]map[string][]VariantSuffix, len(in))
	for namespace, funcs := range in {
		out[namespace] = make(map[string][]VariantSuffix, len(funcs))
		for name, variants := range funcs {
			out[namespace][name] = variantSuffixesFromManifest(variants)
		}
	}
	return out
}

func variantSuffixesFromManifest(in []*string) []VariantSuffix {
	out := make([]VariantSuffix, len(in))
	for i, ptr := range in {
		if ptr == nil {
			continue
		}
		s := *ptr
		out[i] = &s
	}
	return out
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
	policy, err := loadDefaultPolicy()
	if err != nil {
		return nil, nil, err
	}
	return selectVariantsWithPolicy(policy, namespace, name, defs)
}

func selectVariantsWithPolicy(policy variantPolicy, namespace, name string, defs []*Func) ([]*Func, []Diagnostic, error) {
	nsKey := strings.ReplaceAll(namespace, "::", "_")

	// Special handling for detail namespace
	if nsKey == "mlx_core_detail" {
		if !policy.allowedDetailFuncs[name] {
			return nil, diagnosticsForSkipped("skip_unallowed_detail_function", defs), nil
		}
		if len(defs) > 0 {
			return []*Func{defs[0]}, nil, nil
		}
		return nil, nil, nil
	}

	variants, hasVariants := policy.mappings[nsKey]
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
