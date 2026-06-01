package jacclc

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	native "github.com/ml-explore/mlx-c/jaccl"
)

var dtypeParityCases = []struct {
	name   string
	c      DType
	native native.DType
}{
	{"Bool", DTypeBool, native.DTypeBool},
	{"Int8", DTypeInt8, native.DTypeInt8},
	{"Int16", DTypeInt16, native.DTypeInt16},
	{"Int32", DTypeInt32, native.DTypeInt32},
	{"Int64", DTypeInt64, native.DTypeInt64},
	{"Uint8", DTypeUint8, native.DTypeUint8},
	{"Uint16", DTypeUint16, native.DTypeUint16},
	{"Uint32", DTypeUint32, native.DTypeUint32},
	{"Uint64", DTypeUint64, native.DTypeUint64},
	{"Float16", DTypeFloat16, native.DTypeFloat16},
	{"BFloat16", DTypeBFloat16, native.DTypeBFloat16},
	{"Float32", DTypeFloat32, native.DTypeFloat32},
	{"Float64", DTypeFloat64, native.DTypeFloat64},
	{"Complex64", DTypeComplex64, native.DTypeComplex64},
}

func TestParityDTypeSize(t *testing.T) {
	requireJACCLCLibrary(t)

	for _, tt := range dtypeParityCases {
		t.Run(tt.name, func(t *testing.T) {
			cSize, err := tt.c.Size()
			if err != nil {
				t.Fatalf("jacclc dtype size: %v", err)
			}
			nativeSize, err := tt.native.Size()
			if err != nil {
				t.Fatalf("native dtype size: %v", err)
			}
			if cSize != uint(nativeSize) {
				t.Fatalf("dtype size = %d, want %d", cSize, nativeSize)
			}
		})
	}
}

func TestParityConfigSize(t *testing.T) {
	cConfig := newJACCLCParityConfig(t)
	defer cConfig.Close()
	nativeSize, err := (native.Config{Rank: 0, Size: 1}).GroupSize()
	if err != nil {
		t.Fatalf("native group size: %v", err)
	}
	cSize, err := cConfig.Size()
	if err != nil {
		t.Fatalf("jacclc config size: %v", err)
	}
	if cSize != nativeSize {
		t.Fatalf("config size = %d, want %d", cSize, nativeSize)
	}
}

func TestParitySingleRankByteCollectives(t *testing.T) {
	cGroup := newJACCLCParityGroup(t)
	defer cGroup.Close()
	nativeGroup := newNativeParityGroup(t)
	defer nativeGroup.Close()

	input := testPattern(1024)
	tests := []struct {
		name string
		runC func([]byte) error
		runN func([]byte) error
	}{
		{
			name: "AllSumBytes",
			runC: func(out []byte) error {
				return cGroup.AllSumBytes(input, out, DTypeUint8)
			},
			runN: func(out []byte) error {
				return native.AllSumBytes(context.Background(), nativeGroup, out, input, native.DTypeUint8)
			},
		},
		{
			name: "AllMaxBytes",
			runC: func(out []byte) error {
				return cGroup.AllMaxBytes(input, out, DTypeUint8)
			},
			runN: func(out []byte) error {
				return native.AllMaxBytes(context.Background(), nativeGroup, out, input, native.DTypeUint8)
			},
		},
		{
			name: "AllMinBytes",
			runC: func(out []byte) error {
				return cGroup.AllMinBytes(input, out, DTypeUint8)
			},
			runN: func(out []byte) error {
				return native.AllMinBytes(context.Background(), nativeGroup, out, input, native.DTypeUint8)
			},
		},
		{
			name: "AllGatherBytes",
			runC: func(out []byte) error {
				return cGroup.AllGatherBytes(input, out)
			},
			runN: func(out []byte) error {
				return native.AllGatherBytes(context.Background(), nativeGroup, out, input)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cOut := make([]byte, len(input))
			nativeOut := make([]byte, len(input))
			if err := tt.runC(cOut); err != nil {
				t.Fatalf("jacclc: %v", err)
			}
			if err := tt.runN(nativeOut); err != nil {
				t.Fatalf("native: %v", err)
			}
			if !bytes.Equal(cOut, nativeOut) {
				t.Fatalf("jacclc output differs from native")
			}
			if !bytes.Equal(cOut, input) {
				t.Fatalf("single-rank output differs from input")
			}
		})
	}
}

func BenchmarkParityDTypeSize(b *testing.B) {
	b.Run("Native", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := native.DTypeFloat32.Size(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("JACCLC", func(b *testing.B) {
		requireJACCLCLibrary(b)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := DTypeFloat32.Size(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkParityAllSumBytes(b *testing.B) {
	benchmarkParityCollective(b,
		func(ctx context.Context, group *native.Group, out, input []byte) error {
			return native.AllSumBytes(ctx, group, out, input, native.DTypeUint8)
		},
		func(group Group, out, input []byte) error {
			return group.AllSumBytes(input, out, DTypeUint8)
		})
}

func BenchmarkParityAllGatherBytes(b *testing.B) {
	benchmarkParityCollective(b,
		func(ctx context.Context, group *native.Group, out, input []byte) error {
			return native.AllGatherBytes(ctx, group, out, input)
		},
		func(group Group, out, input []byte) error {
			return group.AllGatherBytes(input, out)
		})
}

func BenchmarkCompareDTypeSize(b *testing.B) {
	run := native.DTypeFloat32.Size
	if getenv("MLX_C_JACCL_BENCH_IMPL") == "jacclc" {
		requireJACCLCLibrary(b)
		run = func() (int, error) {
			size, err := DTypeFloat32.Size()
			return int(size), err
		}
	} else if value := getenv("MLX_C_JACCL_BENCH_IMPL"); value != "" && value != "native" {
		b.Fatalf("MLX_C_JACCL_BENCH_IMPL must be native or jacclc")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := run(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompareConfigSize(b *testing.B) {
	if getenv("MLX_C_JACCL_BENCH_IMPL") == "jacclc" {
		config := newJACCLCParityConfig(b)
		defer config.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := config.Size(); err != nil {
				b.Fatal(err)
			}
		}
		return
	} else if value := getenv("MLX_C_JACCL_BENCH_IMPL"); value != "" && value != "native" {
		b.Fatalf("MLX_C_JACCL_BENCH_IMPL must be native or jacclc")
	}
	config := native.Config{Rank: 0, Size: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := config.GroupSize(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompareConfigSetRank(b *testing.B) {
	if getenv("MLX_C_JACCL_BENCH_IMPL") == "jacclc" {
		config := newJACCLCParityConfig(b)
		defer config.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := config.SetRank(i & 1); err != nil {
				b.Fatal(err)
			}
		}
		return
	} else if value := getenv("MLX_C_JACCL_BENCH_IMPL"); value != "" && value != "native" {
		b.Fatalf("MLX_C_JACCL_BENCH_IMPL must be native or jacclc")
	}
	config := native.Config{Rank: 0, Size: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.Rank = i & 1
	}
}

func BenchmarkCompareConfigPreferRing(b *testing.B) {
	if getenv("MLX_C_JACCL_BENCH_IMPL") == "jacclc" {
		config := newJACCLCParityConfig(b)
		defer config.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := config.PreferRing(i&1 == 0); err != nil {
				b.Fatal(err)
			}
		}
		return
	} else if value := getenv("MLX_C_JACCL_BENCH_IMPL"); value != "" && value != "native" {
		b.Fatalf("MLX_C_JACCL_BENCH_IMPL must be native or jacclc")
	}
	config := native.Config{Rank: 0, Size: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.PreferRing = i&1 == 0
	}
}

func BenchmarkCompareConfigPrefersRing(b *testing.B) {
	if getenv("MLX_C_JACCL_BENCH_IMPL") == "jacclc" {
		config := newJACCLCParityConfig(b)
		defer config.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := config.PrefersRing(); err != nil {
				b.Fatal(err)
			}
		}
		return
	} else if value := getenv("MLX_C_JACCL_BENCH_IMPL"); value != "" && value != "native" {
		b.Fatalf("MLX_C_JACCL_BENCH_IMPL must be native or jacclc")
	}
	config := native.Config{Rank: 0, Size: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.PreferRing
	}
}

func BenchmarkCompareConfigIsValidRing(b *testing.B) {
	if getenv("MLX_C_JACCL_BENCH_IMPL") == "jacclc" {
		config := newJACCLCParityConfig(b)
		defer config.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := config.IsValidRing(); err != nil {
				b.Fatal(err)
			}
		}
		return
	} else if value := getenv("MLX_C_JACCL_BENCH_IMPL"); value != "" && value != "native" {
		b.Fatalf("MLX_C_JACCL_BENCH_IMPL must be native or jacclc")
	}
	config := native.Config{Rank: 0, Size: 1, Devices: [][][]string{{nil}}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.IsValidRing()
	}
}

func BenchmarkCompareAllSumBytes(b *testing.B) {
	benchmarkCompareCollective(b, func(impl parityImpl, out, input []byte) error {
		return impl.allSumBytes(out, input)
	})
}

func BenchmarkCompareAllGatherBytes(b *testing.B) {
	benchmarkCompareCollective(b, func(impl parityImpl, out, input []byte) error {
		return impl.allGatherBytes(out, input)
	})
}

func benchmarkParityCollective(
	b *testing.B,
	runNative func(context.Context, *native.Group, []byte, []byte) error,
	runC func(Group, []byte, []byte) error,
) {
	for _, size := range []struct {
		name string
		n    int
	}{
		{"64B", 64},
		{"4KiB", 4 << 10},
		{"1MiB", 1 << 20},
	} {
		b.Run("Native/"+size.name, func(b *testing.B) {
			group := newNativeParityGroup(b)
			defer group.Close()
			benchmarkBytes(b, size.n, func(out, input []byte) error {
				return runNative(context.Background(), group, out, input)
			})
		})
		b.Run("JACCLC/"+size.name, func(b *testing.B) {
			group := newJACCLCParityGroup(b)
			defer group.Close()
			benchmarkBytes(b, size.n, func(out, input []byte) error {
				return runC(group, out, input)
			})
		})
	}
}

func benchmarkCompareCollective(b *testing.B, run func(parityImpl, []byte, []byte) error) {
	for _, size := range []struct {
		name string
		n    int
	}{
		{"64B", 64},
		{"4KiB", 4 << 10},
		{"1MiB", 1 << 20},
	} {
		b.Run(size.name, func(b *testing.B) {
			impl := benchmarkImpl(b)
			defer impl.close()
			benchmarkBytes(b, size.n, func(out, input []byte) error {
				return run(impl, out, input)
			})
		})
	}
}

func benchmarkBytes(b *testing.B, n int, run func([]byte, []byte) error) {
	input := testPattern(n)
	output := make([]byte, n)
	b.SetBytes(int64(n))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := run(output, input); err != nil {
			b.Fatal(err)
		}
	}
}

type parityImpl interface {
	close() error
	allSumBytes([]byte, []byte) error
	allGatherBytes([]byte, []byte) error
}

func benchmarkImpl(tb testing.TB) parityImpl {
	tb.Helper()
	switch getenv("MLX_C_JACCL_BENCH_IMPL") {
	case "", "native":
		return nativeParityImpl{group: newNativeParityGroup(tb)}
	case "jacclc":
		return jacclcParityImpl{group: newJACCLCParityGroup(tb)}
	default:
		tb.Fatalf("MLX_C_JACCL_BENCH_IMPL must be native or jacclc")
		panic("unreachable")
	}
}

type nativeParityImpl struct {
	group *native.Group
}

func (p nativeParityImpl) close() error {
	return p.group.Close()
}

func (p nativeParityImpl) allSumBytes(out, input []byte) error {
	return native.AllSumBytes(context.Background(), p.group, out, input, native.DTypeUint8)
}

func (p nativeParityImpl) allGatherBytes(out, input []byte) error {
	return native.AllGatherBytes(context.Background(), p.group, out, input)
}

type jacclcParityImpl struct {
	group Group
}

func (p jacclcParityImpl) close() error {
	return p.group.Close()
}

func (p jacclcParityImpl) allSumBytes(out, input []byte) error {
	return p.group.AllSumBytes(input, out, DTypeUint8)
}

func (p jacclcParityImpl) allGatherBytes(out, input []byte) error {
	return p.group.AllGatherBytes(input, out)
}

func requireJACCLCLibrary(tb testing.TB) {
	tb.Helper()
	lib := testLibraryPath(tb)
	if lib == "" {
		tb.Skip("set MLX_C_JACCLC_TEST_LIB to a libjacclc dylib or library directory")
	}
	if err := LoadPath(lib); err != nil {
		tb.Fatalf("load libjacclc: %v", err)
	}
}

func newNativeParityGroup(tb testing.TB) *native.Group {
	tb.Helper()
	group, err := native.NewGroup(context.Background(), native.Config{Rank: 0, Size: 1})
	if err != nil {
		tb.Fatalf("new native group: %v", err)
	}
	return group
}

func newJACCLCParityGroup(tb testing.TB) Group {
	tb.Helper()
	config := newJACCLCParityConfig(tb)
	defer config.Close()
	group, err := NewGroupWithConfig(config, false)
	if err != nil {
		tb.Skipf("new jacclc group: %v", err)
	}
	if group.IsNil() {
		tb.Fatal("new jacclc group returned nil")
	}
	size, err := group.Size()
	if err != nil {
		group.Close()
		tb.Fatalf("jacclc group size: %v", err)
	}
	if size != 1 {
		group.Close()
		tb.Fatalf("jacclc group size = %d, want 1", size)
	}
	return group
}

func newJACCLCParityConfig(tb testing.TB) Config {
	tb.Helper()
	requireJACCLCLibrary(tb)
	config, err := NewConfig()
	if err != nil {
		tb.Fatalf("new jacclc config: %v", err)
	}
	if err := config.SetRank(0); err != nil {
		config.Close()
		tb.Fatalf("set jacclc rank: %v", err)
	}
	if err := config.SetCoordinator("127.0.0.1:0"); err != nil {
		config.Close()
		tb.Fatalf("set jacclc coordinator: %v", err)
	}
	if err := config.SetDevicesJSON("[[null]]"); err != nil {
		config.Close()
		tb.Fatalf("set jacclc devices: %v", err)
	}
	return config
}

func testPattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 + 7)
	}
	return b
}

func Example_benchstat() {
	fmt.Println("MLX_C_JACCL_BENCH_IMPL=native go test ./internal/jacclc -run '^$' -bench '^BenchmarkCompare' -benchmem -count=10 > /tmp/jaccl-native.txt")
	fmt.Println("MLX_C_JACCL_BENCH_IMPL=jacclc MLX_C_JACCLC_TEST_LIB=/path/to/libjacclc.dylib go test ./internal/jacclc -run '^$' -bench '^BenchmarkCompare' -benchmem -count=10 > /tmp/jaccl-jacclc.txt")
	fmt.Println("benchstat /tmp/jaccl-native.txt /tmp/jaccl-jacclc.txt")
	// Output:
	// MLX_C_JACCL_BENCH_IMPL=native go test ./internal/jacclc -run '^$' -bench '^BenchmarkCompare' -benchmem -count=10 > /tmp/jaccl-native.txt
	// MLX_C_JACCL_BENCH_IMPL=jacclc MLX_C_JACCLC_TEST_LIB=/path/to/libjacclc.dylib go test ./internal/jacclc -run '^$' -bench '^BenchmarkCompare' -benchmem -count=10 > /tmp/jaccl-jacclc.txt
	// benchstat /tmp/jaccl-native.txt /tmp/jaccl-jacclc.txt
}
