package types

import (
	"strings"
	"testing"
)

func TestRegistryFindsFFTNorm(t *testing.T) {
	r := NewRegistry()
	tm := r.FindByCpp("mlx::core::fft::FFTNorm")
	if tm == nil {
		t.Fatal("FindByCpp(FFTNorm) = nil")
	}
	if got, want := tm.CType, "mlx_fft_norm"; got != want {
		t.Fatalf("CType = %q, want %q", got, want)
	}
	if got, want := r.FindByCpp("FFTNorm"), tm; got != want {
		t.Fatalf("FindByCpp alt = %p, want %p", got, want)
	}
	if got, want := tm.CArg("norm"), "mlx_fft_norm norm"; got != want {
		t.Fatalf("CArg = %q, want %q", got, want)
	}
	if got, want := tm.CToCpp("norm"), "mlx_fft_norm_to_cpp(norm)"; got != want {
		t.Fatalf("CToCpp = %q, want %q", got, want)
	}
}

func TestRegistryFindsGraphUtilsTypes(t *testing.T) {
	r := NewRegistry()

	namer := r.FindByCpp("mlx::core::NodeNamer")
	if namer == nil {
		t.Fatal("FindByCpp(NodeNamer) = nil")
	}
	if got, want := namer.CType, "mlx_node_namer"; got != want {
		t.Fatalf("NodeNamer CType = %q, want %q", got, want)
	}
	if got, want := r.FindByCpp("NodeNamer"), namer; got != want {
		t.Fatalf("FindByCpp NodeNamer alt = %p, want %p", got, want)
	}

	stream := r.FindByCpp("std::ostream")
	if stream == nil {
		t.Fatal("FindByCpp(std::ostream) = nil")
	}
	if got, want := stream.CArg("os"), "FILE* os"; got != want {
		t.Fatalf("std::ostream CArg = %q, want %q", got, want)
	}
	if got, want := stream.CToCpp("os"), "CFileOutputStream::as_lvalue(CFileOutputStream(os))"; got != want {
		t.Fatalf("std::ostream CToCpp = %q, want %q", got, want)
	}
}

func TestDefaultPolicyCoversRegistry(t *testing.T) {
	policy, err := LoadPolicyPath("")
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.CheckRegistry(NewRegistry()); err != nil {
		t.Fatal(err)
	}
	if len(policy.Types) != len(NewRegistry().Mappings()) {
		t.Fatalf("policy types = %d, registry mappings = %d", len(policy.Types), len(NewRegistry().Mappings()))
	}
}

func TestLoadPolicyRejectsUnknownFields(t *testing.T) {
	_, err := LoadPolicy(strings.NewReader(`
schema_version: 1
types:
  - cpp: int
    c: int
    class: primitive
    ownership: value
    nullability: nonnull
    conversion: primitive
    surprise: true
`))
	if err == nil || !strings.Contains(err.Error(), "field surprise not found") {
		t.Fatalf("LoadPolicy error = %v", err)
	}
}

func TestPolicyCheckRegistryRejectsDrift(t *testing.T) {
	policy, err := LoadPolicy(strings.NewReader(`
schema_version: 1
types:
  - cpp: int
    c: long
    class: primitive
    ownership: value
    nullability: nonnull
    conversion: primitive
`))
	if err != nil {
		t.Fatal(err)
	}
	err = policy.CheckRegistry(NewRegistry())
	if err == nil || !strings.Contains(err.Error(), `type policy "int" c = "long"`) {
		t.Fatalf("CheckRegistry error = %v", err)
	}
}
