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

	raw := make([]byte, 3)
	if err := AllGatherBytes(context.Background(), g, raw, []byte{1, 2, 3}); err != nil {
		t.Fatalf("AllGatherBytes: %v", err)
	}
	if string(raw) != string([]byte{1, 2, 3}) {
		t.Fatalf("AllGatherBytes = %v, want [1 2 3]", raw)
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

func TestMeshConnectivity(t *testing.T) {
	cfg := Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, nil},
			{nil, nil},
		},
	}
	if err := checkMeshConnectivity(cfg); err == nil {
		t.Fatal("checkMeshConnectivity succeeded")
	}
	cfg.Devices[0][1] = []string{"rdma_en1"}
	if err := checkMeshConnectivity(cfg); err != nil {
		t.Fatalf("checkMeshConnectivity connected rank: %v", err)
	}
}

func TestGraphConnectivity(t *testing.T) {
	cfg := Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1"}, nil},
			{{"rdma_en1"}, nil, {"rdma_en2"}},
			{nil, {"rdma_en2"}, nil},
		},
	}
	if err := checkGraphConnectivity(cfg); err != nil {
		t.Fatalf("line graph connectivity: %v", err)
	}
	if isMesh(cfg) {
		t.Fatal("isMesh succeeded for line graph")
	}
	if err := checkMeshConnectivity(cfg); err == nil {
		t.Fatal("mesh connectivity succeeded for line graph")
	}
	cfg.Devices[0][2] = []string{"rdma_en3"}
	cfg.Devices[2][0] = []string{"rdma_en3"}
	if !isMesh(cfg) {
		t.Fatal("isMesh failed for full mesh")
	}
	cfg.Devices[1][2] = nil
	cfg.Devices[0][2] = nil
	cfg.Devices[2][0] = nil
	if err := checkGraphConnectivity(cfg); err == nil {
		t.Fatal("graph connectivity succeeded for disconnected graph")
	}
}

func TestConfigTopologyQueries(t *testing.T) {
	line := Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1"}, nil},
			{{"rdma_en1"}, nil, {"rdma_en2"}},
			{nil, {"rdma_en2"}, nil},
		},
	}
	if got, err := line.GroupSize(); err != nil || got != 3 {
		t.Fatalf("line GroupSize = %d, %v, want 3, nil", got, err)
	}
	if line.IsValidMesh() {
		t.Fatal("line IsValidMesh succeeded")
	}
	if line.IsValidRing() {
		t.Fatal("line IsValidRing succeeded")
	}

	ring := Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1"}, nil, {"rdma_en4"}},
			{{"rdma_en1"}, nil, {"rdma_en2"}, nil},
			{nil, {"rdma_en2"}, nil, {"rdma_en3"}},
			{{"rdma_en4"}, nil, {"rdma_en3"}, nil},
		},
	}
	if !ring.IsValidRing() {
		t.Fatal("ring IsValidRing failed")
	}
	if ring.IsValidMesh() {
		t.Fatal("four-rank ring IsValidMesh succeeded")
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

func TestGatheredBytesLength(t *testing.T) {
	if _, err := gatheredBytes("all sum", 1, []byte{1, 2}, 4); err == nil {
		t.Fatal("gatheredBytes succeeded with short value")
	}
	got, err := gatheredBytes("all sum", 1, []byte{1, 2}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
}

func TestMergeGraphGatherPayloadLength(t *testing.T) {
	known := []bool{true, false}
	values := [][]byte{[]byte{1}, nil}
	if err := mergeGraphGatherPayload(known, values, []byte{1, 0, 1}, 1); err == nil {
		t.Fatal("mergeGraphGatherPayload succeeded with short payload")
	}
	payload := []byte{1, 1, 1, 2}
	if err := mergeGraphGatherPayload(known, values, payload, 1); err != nil {
		t.Fatal(err)
	}
	if !known[1] || values[1][0] != 2 {
		t.Fatalf("known/value = %v/%v, want rank 1 value 2", known, values)
	}
}
