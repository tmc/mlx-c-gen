package variants

import "testing"

func testPolicy() variantPolicy {
	return variantPolicy{
		mappings: map[string]map[string][]VariantSuffix{
			"mlx_core": {
				"squeeze": {testSuffix("axes"), testSuffix("axis"), testSuffix("")},
			},
			"mlx_core_metal": {
				"device_info": {nil},
			},
		},
		allowedDetailFuncs: map[string]bool{
			"compile": true,
		},
	}
}

func testSuffix(s string) VariantSuffix {
	return &s
}

func TestSelectVariantsUsesManifestSuffixes(t *testing.T) {
	defs := []*Func{
		{Name: "squeeze", Namespace: "mlx::core", ReturnType: "array"},
		{Name: "squeeze", Namespace: "mlx::core", ReturnType: "array"},
		{Name: "squeeze", Namespace: "mlx::core", ReturnType: "array"},
	}
	got, diagnostics, err := selectVariantsWithPolicy(testPolicy(), "mlx::core", "squeeze", defs)
	if err != nil {
		t.Fatalf("SelectVariantsWithDiagnostics: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	if len(got) != 3 {
		t.Fatalf("SelectVariants returned %d definitions, want 3", len(got))
	}
	for i, want := range []string{"axes", "axis", ""} {
		if got[i] != defs[i] {
			t.Fatalf("selected[%d] = %#v, want original definition", i, got[i])
		}
		if got[i].Variant != want {
			t.Fatalf("selected[%d].Variant = %q, want %q", i, got[i].Variant, want)
		}
		if got[i].VariantIndex != i {
			t.Fatalf("selected[%d].VariantIndex = %d, want %d", i, got[i].VariantIndex, i)
		}
	}
}

func TestSelectVariantsSkipsMetalDeviceInfo(t *testing.T) {
	defs := []*Func{{
		Name:       "device_info",
		Namespace:  "mlx::core::metal",
		ReturnType: "std::unordered_map<std::string, std::variant<std::string, size_t>>",
	}}
	got, diagnostics, err := selectVariantsWithPolicy(testPolicy(), "mlx::core::metal", "device_info", defs)
	if err != nil {
		t.Fatalf("SelectVariantsWithDiagnostics: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("SelectVariants returned %d definitions, want 0", len(got))
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != "skip_variant_mapping" || diagnostics[0].Func != defs[0] {
		t.Fatalf("diagnostics = %#v, want one skip_variant_mapping for original definition", diagnostics)
	}
}

func TestSelectVariantsKeepsCompatibilityWrapper(t *testing.T) {
	defs := []*Func{{
		Name:       "device_info",
		Namespace:  "mlx::core::metal",
		ReturnType: "std::unordered_map<std::string, std::variant<std::string, size_t>>",
	}}
	got, _, err := selectVariantsWithPolicy(testPolicy(), "mlx::core::metal", "device_info", defs)
	if err != nil {
		t.Fatalf("SelectVariants: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("SelectVariants returned %d definitions, want 0", len(got))
	}
}

func TestSelectVariantsKeepsMetalSingles(t *testing.T) {
	defs := []*Func{{
		Name:       "is_available",
		Namespace:  "mlx::core::metal",
		ReturnType: "bool",
	}}
	got, _, err := selectVariantsWithPolicy(testPolicy(), "mlx::core::metal", "is_available", defs)
	if err != nil {
		t.Fatalf("SelectVariants: %v", err)
	}
	if len(got) != 1 || got[0] != defs[0] {
		t.Fatalf("SelectVariants returned %#v, want original definition", got)
	}
}

func TestSelectVariantsAllowsManifestDetailFunction(t *testing.T) {
	defs := []*Func{{
		Name:       "compile",
		Namespace:  "mlx::core::detail",
		ReturnType: "void",
	}}
	got, diagnostics, err := selectVariantsWithPolicy(testPolicy(), "mlx::core::detail", "compile", defs)
	if err != nil {
		t.Fatalf("SelectVariantsWithDiagnostics: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	if len(got) != 1 || got[0] != defs[0] {
		t.Fatalf("SelectVariants returned %#v, want original definition", got)
	}
}

func TestSelectVariantsRejectsUnmappedOverloads(t *testing.T) {
	defs := []*Func{
		{Name: "new_overload", Namespace: "mlx::core", ReturnType: "array", ParamTypes: []string{"array"}},
		{Name: "new_overload", Namespace: "mlx::core", ReturnType: "array", ParamTypes: []string{"array", "array"}},
	}
	_, _, err := selectVariantsWithPolicy(testPolicy(), "mlx::core", "new_overload", defs)
	if err == nil {
		t.Fatal("SelectVariantsWithDiagnostics succeeded for unmapped overloads")
	}
}
