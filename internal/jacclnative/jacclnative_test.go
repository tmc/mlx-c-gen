package jacclnative

import (
	"context"
	"errors"
	"testing"
)

func TestSingleRankGroupCollectives(t *testing.T) {
	g, err := NewGroup(context.Background(), Config{Rank: 0, Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	src := []int32{1, 2, 3}
	dst := make([]int32, len(src))
	if err := AllSum(context.Background(), g, dst, src); err != nil {
		t.Fatalf("AllSum: %v", err)
	}
	for i := range src {
		if dst[i] != src[i] {
			t.Fatalf("AllSum dst[%d] = %d, want %d", i, dst[i], src[i])
		}
	}

	gather := make([]int32, len(src))
	if err := AllGather(context.Background(), g, gather, src); err != nil {
		t.Fatalf("AllGather: %v", err)
	}
	for i := range src {
		if gather[i] != src[i] {
			t.Fatalf("AllGather dst[%d] = %d, want %d", i, gather[i], src[i])
		}
	}
}

func TestMultiRankFailsClosed(t *testing.T) {
	_, err := NewGroup(context.Background(), Config{
		Rank:        0,
		Size:        2,
		Coordinator: "127.0.0.1:9000",
		Devices: [][][]string{
			{nil, {"rdma_en1"}},
			{{"rdma_en1"}, nil},
		},
	})
	if err == nil {
		t.Fatal("NewGroup multi-rank succeeded before transport is implemented")
	}
	if !RDMAAvailable() && !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("NewGroup error = %v, want rdma unavailable", err)
	}
}

func TestRDMADeviceNamesUnavailable(t *testing.T) {
	if RDMAAvailable() {
		t.Skip("RDMA provider is available")
	}
	_, err := RDMADeviceNames()
	if !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("RDMADeviceNames error = %v, want rdma unavailable", err)
	}
}

func TestDTypeSizeMatchesCAPI(t *testing.T) {
	tests := []struct {
		dt   DType
		want int
	}{
		{DTypeBool, 1},
		{DTypeInt8, 1},
		{DTypeFloat16, 2},
		{DTypeFloat32, 4},
		{DTypeComplex64, 8},
	}
	for _, tt := range tests {
		got, err := tt.dt.Size()
		if err != nil {
			t.Fatalf("%v Size: %v", tt.dt, err)
		}
		if got != tt.want {
			t.Fatalf("%v Size = %d, want %d", tt.dt, got, tt.want)
		}
	}
}
