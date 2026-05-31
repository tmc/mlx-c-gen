package variants

import "testing"

func TestSelectVariantsSkipsMetalDeviceInfo(t *testing.T) {
	defs := []*Func{{
		Name:       "device_info",
		Namespace:  "mlx::core::metal",
		ReturnType: "std::unordered_map<std::string, std::variant<std::string, size_t>>",
	}}
	got, err := SelectVariants("mlx::core::metal", "device_info", defs)
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
	got, err := SelectVariants("mlx::core::metal", "is_available", defs)
	if err != nil {
		t.Fatalf("SelectVariants: %v", err)
	}
	if len(got) != 1 || got[0] != defs[0] {
		t.Fatalf("SelectVariants returned %#v, want original definition", got)
	}
}
