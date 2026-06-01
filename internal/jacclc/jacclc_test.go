package jacclc

import (
	"errors"
	"os"
	"strings"
	"testing"
	"unsafe"
)

var (
	_ func(DType) (uint, error)                                      = DType.Size
	_ func(Group) error                                              = Group.Barrier
	_ func(Group, unsafe.Pointer, unsafe.Pointer, uint, DType) error = Group.AllSum
	_ func(Group, unsafe.Pointer, unsafe.Pointer, uint, DType) error = Group.AllMax
	_ func(Group, unsafe.Pointer, unsafe.Pointer, uint, DType) error = Group.AllMin
	_ func(Group, unsafe.Pointer, unsafe.Pointer, uint) error        = Group.AllGather
	_ func(Group, unsafe.Pointer, uint, int) error                   = Group.Send
	_ func(Group, unsafe.Pointer, uint, int) error                   = Group.Recv
	_ func(Group, []byte, []byte, DType) error                       = Group.AllSumBytes
	_ func(Group, []byte, []byte, DType) error                       = Group.AllMaxBytes
	_ func(Group, []byte, []byte, DType) error                       = Group.AllMinBytes
	_ func(Group, []byte, []byte) error                              = Group.AllGatherBytes
	_ func(Group, []byte, int) error                                 = Group.SendBytes
	_ func(Group, []byte, int) error                                 = Group.RecvBytes
)

func TestDylibSmoke(t *testing.T) {
	lib := testLibraryPath(t)
	if lib == "" {
		t.Skip("set MLX_C_JACCLC_TEST_LIB to a libjacclc dylib or library directory")
	}
	if err := LoadPath(lib); err != nil {
		t.Fatalf("load libjacclc: %v", err)
	}
	if LibraryPath() == "" {
		t.Fatal("library path is empty after LoadPath")
	}

	size, err := DTypeSize(DTypeFloat32)
	if err != nil {
		t.Fatalf("DTypeSize(float32): %v", err)
	}
	if size != 4 {
		t.Fatalf("DTypeSize(float32) = %d, want 4", size)
	}
	if _, err := DTypeSize(DType(-1)); err == nil {
		t.Fatal("DTypeSize(invalid) succeeded")
	} else {
		var cerr CError
		if !errors.As(err, &cerr) {
			t.Fatalf("DTypeSize(invalid) error = %T, want CError", err)
		}
		if cerr.Call != "mlx_jaccl_dtype_size" {
			t.Fatalf("DTypeSize(invalid) call = %q", cerr.Call)
		}
	}
	if err := ClearError(); err != nil {
		t.Fatalf("ClearError: %v", err)
	}

	if _, err := ConfigRank(Config{}); err == nil {
		t.Fatal("ConfigRank(empty) succeeded")
	}

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	if config.IsNil() {
		t.Fatal("NewConfig returned nil")
	}
	if err := config.SetRank(0); err != nil {
		t.Fatalf("Config.SetRank: %v", err)
	}
	if err := config.SetCoordinator("127.0.0.1:9000"); err != nil {
		t.Fatalf("Config.SetCoordinator: %v", err)
	}
	rank, err := config.Rank()
	if err != nil {
		t.Fatalf("Config.Rank: %v", err)
	}
	if rank != 0 {
		t.Fatalf("ConfigRank = %d, want 0", rank)
	}
	if err := config.SetRank(1); err != nil {
		t.Fatalf("Config.SetRank(update): %v", err)
	}
	rank, err = config.Rank()
	if err != nil {
		t.Fatalf("Config.Rank(update): %v", err)
	}
	if rank != 1 {
		t.Fatalf("ConfigRank after update = %d, want 1", rank)
	}
	if err := config.SetRank(0); err != nil {
		t.Fatalf("Config.SetRank(restore): %v", err)
	}
	coordinator, err := config.Coordinator()
	if err != nil {
		t.Fatalf("Config.Coordinator: %v", err)
	}
	if coordinator != "127.0.0.1:9000" {
		t.Fatalf("ConfigCoordinator = %q", coordinator)
	}
	if err := config.SetCoordinator("127.0.0.1:9001"); err != nil {
		t.Fatalf("Config.SetCoordinator(update): %v", err)
	}
	coordinator, err = config.Coordinator()
	if err != nil {
		t.Fatalf("Config.Coordinator(update): %v", err)
	}
	if coordinator != "127.0.0.1:9001" {
		t.Fatalf("ConfigCoordinator after update = %q", coordinator)
	}
	if err := config.Close(); err != nil {
		t.Fatalf("Config.Close: %v", err)
	}
	if _, ok := cachedCoordinator(config); ok {
		t.Fatal("Config.Close left coordinator cached")
	}
	if _, ok := cachedRank(config); ok {
		t.Fatal("Config.Close left rank cached")
	}
	if err := (Group{}).Close(); err != nil {
		t.Fatalf("Group.Close(zero): %v", err)
	}
}

func TestByteHelpersValidateLengthsBeforeLoad(t *testing.T) {
	oldGetenv := getenv
	getenv = func(string) string { return "" }
	defer func() { getenv = oldGetenv }()

	group := Group{}
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "all sum",
			run:  func() error { return group.AllSumBytes([]byte{1, 2}, []byte{0}, DTypeUint8) },
			want: "all sum: output length 1, want 2",
		},
		{
			name: "all max",
			run:  func() error { return group.AllMaxBytes([]byte{1, 2}, []byte{0}, DTypeUint8) },
			want: "all max: output length 1, want 2",
		},
		{
			name: "all min",
			run:  func() error { return group.AllMinBytes([]byte{1, 2}, []byte{0}, DTypeUint8) },
			want: "all min: output length 1, want 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil {
				t.Fatal("operation succeeded")
			}
			if got := err.Error(); got != tt.want {
				t.Fatalf("error = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAllGatherBytesLen(t *testing.T) {
	got, err := allGatherBytesLen(3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got != 12 {
		t.Fatalf("allGatherBytesLen = %d, want 12", got)
	}

	max := int(^uint(0) >> 1)
	if _, err := allGatherBytesLen(max, 2); err == nil {
		t.Fatal("allGatherBytesLen overflow succeeded")
	}
	if _, err := allGatherBytesLen(-1, 2); err == nil {
		t.Fatal("allGatherBytesLen negative size succeeded")
	}
}

func testLibraryPath(t testing.TB) string {
	t.Helper()
	for _, name := range []string{"MLX_C_JACCLC_TEST_LIB", "MLX_C_JACCLC_LIB_PATH", "MLX_JACCLC_LIB_PATH"} {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

var getenv = os.Getenv
