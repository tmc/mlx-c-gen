package jacclnative

import (
	"context"
	"errors"
	"testing"
	"time"
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
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := NewGroup(ctx, Config{
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

func TestRDMAResourcesUnavailable(t *testing.T) {
	if RDMAAvailable() {
		t.Skip("RDMA provider is available")
	}
	if _, err := openRDMADevice(""); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("openRDMADevice error = %v, want rdma unavailable", err)
	}
	if _, err := newRDMAProtectionDomain(nil); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("newRDMAProtectionDomain error = %v, want rdma unavailable", err)
	}
	if _, err := newRDMACompletionQueue(nil, 1); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("newRDMACompletionQueue error = %v, want rdma unavailable", err)
	}
	if _, err := newRDMAQueuePair(nil, nil); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("newRDMAQueuePair error = %v, want rdma unavailable", err)
	}
	if _, err := newRDMAMemoryRegion(nil, 1); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("newRDMAMemoryRegion error = %v, want rdma unavailable", err)
	}
	if _, err := localRDMADestination(nil); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("localRDMADestination error = %v, want rdma unavailable", err)
	}
	if err := initRDMAQueuePair(nil); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("initRDMAQueuePair error = %v, want rdma unavailable", err)
	}
	if err := readyToReceiveRDMA(context.Background(), nil, rdmaDestination{}, rdmaDestination{}); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("readyToReceiveRDMA error = %v, want rdma unavailable", err)
	}
	if err := readyToSendRDMA(context.Background(), nil, 7); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("readyToSendRDMA error = %v, want rdma unavailable", err)
	}
	if err := postRDMASend(nil, nil, 0, 1, 1); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("postRDMASend error = %v, want rdma unavailable", err)
	}
	if err := postRDMARecv(nil, nil, 0, 1, 1); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("postRDMARecv error = %v, want rdma unavailable", err)
	}
	if _, err := pollRDMACompletion(context.Background(), nil); !errors.Is(err, errRDMAUnavailable) {
		t.Fatalf("pollRDMACompletion error = %v, want rdma unavailable", err)
	}
}

func TestRDMAPostNilValidation(t *testing.T) {
	if !RDMAAvailable() {
		t.Skip("RDMA provider is unavailable")
	}
	if err := postRDMASend(nil, nil, 0, 1, 1); err == nil {
		t.Fatal("postRDMASend nil args succeeded")
	}
	if err := postRDMARecv(nil, nil, 0, 1, 1); err == nil {
		t.Fatal("postRDMARecv nil args succeeded")
	}
	if _, err := pollRDMACompletion(context.Background(), nil); err == nil {
		t.Fatal("pollRDMACompletion nil args succeeded")
	}
}

func TestRequiredMemoryRegions(t *testing.T) {
	cfg := Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1", " "}, {"rdma_en2"}},
			{{"rdma_en1"}, nil, {"rdma_en3"}},
			{{"rdma_en2"}, {"rdma_en3"}, nil},
		},
	}
	if got, want := requiredMemoryRegions(cfg), 4; got != want {
		t.Fatalf("requiredMemoryRegions = %d, want %d", got, want)
	}
}

func TestMemoryRegionBudget(t *testing.T) {
	row := make([][]string, 2)
	wires := make([]string, maxMemoryRegions/2+1)
	for i := range wires {
		wires[i] = "rdma_en1"
	}
	row[1] = wires
	cfg := Config{
		Rank:    0,
		Devices: [][][]string{row, {nil, nil}},
	}
	err := checkMemoryRegionBudget(cfg)
	if err == nil {
		t.Fatal("checkMemoryRegionBudget succeeded")
	}
	if _, ok := err.(*memoryRegionBudgetError); !ok {
		t.Fatalf("checkMemoryRegionBudget error = %T, want *memoryRegionBudgetError", err)
	}
}

func TestWireRangePartitions(t *testing.T) {
	total := 17
	nWires := 4
	seen := 0
	for wire := 0; wire < nWires; wire++ {
		off, n := wireRange(total, nWires, wire)
		if off != seen {
			t.Fatalf("wire %d offset = %d, want %d", wire, off, seen)
		}
		seen += n
	}
	if seen != total {
		t.Fatalf("partition covered %d bytes, want %d", seen, total)
	}
}

func TestRecvPostLen(t *testing.T) {
	if got := recvPostLen(1); got != rdmaStagingBytes {
		t.Fatalf("recvPostLen(1) = %d, want %d", got, rdmaStagingBytes)
	}
	if got := recvPostLen(rdmaStagingBytes + 1); got != rdmaStagingBytes+1 {
		t.Fatalf("recvPostLen(large) = %d, want %d", got, rdmaStagingBytes+1)
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
