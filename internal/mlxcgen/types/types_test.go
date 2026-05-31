package types

import "testing"

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
