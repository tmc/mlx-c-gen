package jacclnative

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unsafe"
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

	rawSrc := []int32{1, 2, 3}
	rawDst := make([]int32, len(rawSrc))
	if err := AllSumBytes(context.Background(), g, bytesOf(rawDst), bytesOf(rawSrc), DTypeInt32); err != nil {
		t.Fatalf("AllSumBytes: %v", err)
	}
	if !reflect.DeepEqual(rawDst, rawSrc) {
		t.Fatalf("AllSumBytes = %v, want %v", rawDst, rawSrc)
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

func TestConfigFromEnvRequiresJACCLInputs(t *testing.T) {
	clearJACCLEnv(t)
	t.Setenv("JACCL_RANK", "0")
	t.Setenv("JACCL_SIZE", "1")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("ConfigFromEnv succeeded without coordinator and devices")
	}

	t.Setenv("JACCL_COORDINATOR", "127.0.0.1:9000")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("ConfigFromEnv succeeded without devices")
	}
}

func TestConfigFromEnvReadsJACCLInputs(t *testing.T) {
	clearJACCLEnv(t)
	path := writeDeviceMatrix(t, [][][]string{{nil}})
	t.Setenv("JACCL_RANK", "0")
	t.Setenv("JACCL_COORDINATOR", "127.0.0.1:9000")
	t.Setenv("JACCL_IBV_DEVICES", path)
	t.Setenv("JACCL_RING", "1")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rank != 0 || cfg.Coordinator != "127.0.0.1:9000" || !cfg.PreferRing {
		t.Fatalf("ConfigFromEnv = %+v", cfg)
	}
	if got, err := cfg.GroupSize(); err != nil || got != 1 {
		t.Fatalf("GroupSize = %d, %v, want 1, nil", got, err)
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
	if err := readyToReceiveRDMA(context.Background(), nil, rdmaDestination{}, rdmaDestination{}, rdmaRTRPolicy{}); !errors.Is(err, errRDMAUnavailable) {
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

func TestReciprocalConnections(t *testing.T) {
	cfg := Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1"}},
			{{"rdma_en1"}, nil},
		},
	}
	if err := checkReciprocalConnections(cfg); err != nil {
		t.Fatalf("checkReciprocalConnections symmetric: %v", err)
	}

	cfg.Devices[1][0] = nil
	if err := checkReciprocalConnections(cfg); err == nil {
		t.Fatal("checkReciprocalConnections succeeded with missing reverse edge")
	}
	cfg.Devices[1][0] = []string{"rdma_en1", "rdma_en2"}
	if err := checkReciprocalConnections(cfg); err == nil {
		t.Fatal("checkReciprocalConnections succeeded with uneven wire counts")
	}
}

func TestNoSelfConnections(t *testing.T) {
	cfg := Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1"}},
			{{"rdma_en1"}, nil},
		},
	}
	if err := checkNoSelfConnections(cfg); err != nil {
		t.Fatalf("checkNoSelfConnections clean matrix: %v", err)
	}

	cfg.Devices[1][1] = []string{" ", "rdma_en2"}
	if err := checkNoSelfConnections(cfg); err == nil {
		t.Fatal("checkNoSelfConnections succeeded with self connection")
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

	mesh := Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1"}},
			{{"rdma_en1"}, nil},
		},
	}
	if !mesh.IsValidMesh() {
		t.Fatal("mesh IsValidMesh failed")
	}
	mesh.Devices[0][0] = []string{"rdma_en1"}
	if mesh.IsValidMesh() {
		t.Fatal("mesh IsValidMesh succeeded with self connection")
	}

	unevenRing := ring
	unevenRing.Devices[0][1] = append(unevenRing.Devices[0][1], "rdma_en5")
	if unevenRing.IsValidRing() {
		t.Fatal("ring IsValidRing succeeded with uneven wire counts")
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
	for _, n := range []int{0, 1, 8192, rdmaStagingBytes, rdmaStagingBytes + 1} {
		if got := recvPostLen(n); got != n {
			t.Fatalf("recvPostLen(%d) = %d, want %d", n, got, n)
		}
	}
}

func TestPollRDMACompletionsRejectsNegativeCount(t *testing.T) {
	if _, err := pollRDMACompletions(context.Background(), nil, -1); err == nil {
		t.Fatal("pollRDMACompletions succeeded with negative count")
	}
}

// TestWarmupOrderingCompletesBothSides exercises the invariant warmupConnections
// relies on: when both peers post their recv before either posts its send (the
// barrier fence), the warm-up exchange completes on both sides. This is the
// ordering that keeps a fresh queue pair off the RNR-NAK path that causes the
// setup-time phase-lock wedge.
func TestWarmupOrderingCompletesBothSides(t *testing.T) {
	a, b := newMemPair()
	aRecv := slotWorkID(workKindRecv, 0, 0)
	aSend := slotWorkID(workKindSend, 0, 0)
	bRecv := slotWorkID(workKindRecv, 0, 0)
	bSend := slotWorkID(workKindSend, 0, 0)

	// Barrier-synced order: every recv is posted before any send goes out.
	if err := a.postRecv(0, recvPostLen(1), aRecv); err != nil {
		t.Fatal(err)
	}
	if err := b.postRecv(0, recvPostLen(1), bRecv); err != nil {
		t.Fatal(err)
	}
	// (barrier here) — now the sends cannot land on an empty receive queue.
	if err := a.postSend(0, 1, aSend); err != nil {
		t.Fatal(err)
	}
	if err := b.postSend(0, 1, bSend); err != nil {
		t.Fatal(err)
	}
	if err := confirmCompletions(context.Background(), a,
		expect{id: aRecv, bytes: 1}, expect{id: aSend, bytes: 1}); err != nil {
		t.Fatalf("side a warm-up confirm: %v", err)
	}
	if err := confirmCompletions(context.Background(), b,
		expect{id: bRecv, bytes: 1}, expect{id: bSend, bytes: 1}); err != nil {
		t.Fatalf("side b warm-up confirm: %v", err)
	}
}

// TestWarmupSendWithoutPeerRecvStalls shows the failure the barrier avoids: a
// send posted before the peer has a matching recv produces no completion (the
// RNR stall the wedge is built on). confirmCompletions then times out rather
// than returning a bogus success.
func TestWarmupSendWithoutPeerRecvStalls(t *testing.T) {
	a, _ := newMemPair()
	sendID := slotWorkID(workKindSend, 0, 0)
	if err := a.postSend(0, 1, sendID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := confirmCompletions(ctx, a, expect{id: sendID, bytes: 1}); err == nil {
		t.Fatal("send with no matching peer recv reported completion; expected a stall/timeout")
	}
}

func TestConfirmCompletionsMatchesPostedWork(t *testing.T) {
	a, b := newMemPair()
	recvID := slotWorkID(workKindRecv, 0, 0)
	sendID := slotWorkID(workKindSend, 0, 0)
	if err := b.postRecv(0, 8, recvID); err != nil {
		t.Fatal(err)
	}
	if err := a.postSend(0, 8, sendID); err != nil {
		t.Fatal(err)
	}
	if err := confirmCompletions(context.Background(), b, expect{id: recvID, bytes: 8}); err != nil {
		t.Fatalf("confirm recv completion: %v", err)
	}
	if err := confirmCompletions(context.Background(), a, expect{id: sendID, bytes: 8}); err != nil {
		t.Fatalf("confirm send completion: %v", err)
	}
}

func TestConfirmCompletionsDrainsOnTimeout(t *testing.T) {
	a, _ := newMemPair()
	// Post a send whose matching recv never arrives, then confirm with an
	// already-expired context so poll returns ctx.Err() with the work request
	// still outstanding — the wedge trigger.
	sendID := slotWorkID(workKindSend, 0, 0)
	if err := a.postSend(0, 8, sendID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := confirmCompletions(ctx, a, expect{id: sendID, bytes: 8})
	if err == nil {
		t.Fatal("confirmCompletions succeeded despite a cancelled context")
	}
	if !a.drained {
		t.Fatal("timed-out confirmCompletions did not drain the transport")
	}
	// A poisoned transport must fail fast rather than silently reuse the dead
	// queue pair (the durable wedge).
	if _, perr := a.poll(context.Background(), 1); !errors.Is(perr, errTransportPoisoned) {
		t.Fatalf("poll after drain = %v, want errTransportPoisoned", perr)
	}
}

func TestConfirmCompletionsRejectsUnexpectedID(t *testing.T) {
	a, b := newMemPair()
	recvID := slotWorkID(workKindRecv, 0, 0)
	if err := b.postRecv(0, 8, recvID); err != nil {
		t.Fatal(err)
	}
	if err := a.postSend(0, 8, slotWorkID(workKindSend, 0, 0)); err != nil {
		t.Fatal(err)
	}
	// The recv completion carries recvID, but the caller waits on a
	// different slot's work id: the stale completion must be rejected
	// rather than silently consumed.
	wrongID := slotWorkID(workKindRecv, 0, 1)
	if err := confirmCompletions(context.Background(), b, expect{id: wrongID, bytes: 8}); err == nil {
		t.Fatal("confirmCompletions accepted a completion for an unexpected work id")
	}
}

func TestConfirmCompletionsRejectsShortReceive(t *testing.T) {
	a, b := newMemPair()
	recvID := slotWorkID(workKindRecv, 0, 0)
	if err := b.postRecv(0, 16, recvID); err != nil {
		t.Fatal(err)
	}
	// Peer delivers fewer bytes than the caller expects for this chunk.
	if err := a.postSend(0, 8, slotWorkID(workKindSend, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := confirmCompletions(context.Background(), b, expect{id: recvID, bytes: 16}); err == nil {
		t.Fatal("confirmCompletions accepted a receive shorter than the expected chunk")
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

func TestAllReduceBytesValidation(t *testing.T) {
	g, err := NewGroup(context.Background(), Config{Rank: 0, Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	if err := AllSumBytes(context.Background(), g, make([]byte, 3), make([]byte, 3), DTypeInt16); err == nil {
		t.Fatal("AllSumBytes succeeded with unaligned int16 byte length")
	}
	if err := AllMaxBytes(context.Background(), g, make([]byte, 4), make([]byte, 8), DTypeInt32); err == nil {
		t.Fatal("AllMaxBytes succeeded with mismatched lengths")
	}
	if err := AllMinBytes(context.Background(), g, make([]byte, 4), make([]byte, 4), DType(-1)); err == nil {
		t.Fatal("AllMinBytes succeeded with invalid dtype")
	}
}

func TestAllReduceBytesSingleRankDTypes(t *testing.T) {
	g, err := NewGroup(context.Background(), Config{Rank: 0, Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	floatSrc := []float32{1.5, 2.5}
	floatDst := make([]float32, len(floatSrc))
	if err := AllMaxBytes(context.Background(), g, bytesOf(floatDst), bytesOf(floatSrc), DTypeFloat32); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(floatDst, floatSrc) {
		t.Fatalf("AllMaxBytes float32 = %v, want %v", floatDst, floatSrc)
	}

	complexSrc := []complex64{complex(1, 2), complex(3, 4)}
	complexDst := make([]complex64, len(complexSrc))
	if err := AllMinBytes(context.Background(), g, bytesOf(complexDst), bytesOf(complexSrc), DTypeComplex64); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(complexDst, complexSrc) {
		t.Fatalf("AllMinBytes complex64 = %v, want %v", complexDst, complexSrc)
	}

	halfSrc := []byte{0x00, 0x3e, 0x00, 0x41}
	halfDst := make([]byte, len(halfSrc))
	if err := AllSumBytes(context.Background(), g, halfDst, halfSrc, DTypeFloat16); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(halfDst, halfSrc) {
		t.Fatalf("AllSumBytes float16 = %v, want %v", halfDst, halfSrc)
	}

	bfloatSrc := []byte{0xc0, 0x3f, 0x20, 0x40}
	bfloatDst := make([]byte, len(bfloatSrc))
	if err := AllMaxBytes(context.Background(), g, bfloatDst, bfloatSrc, DTypeBFloat16); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bfloatDst, bfloatSrc) {
		t.Fatalf("AllMaxBytes bfloat16 = %v, want %v", bfloatDst, bfloatSrc)
	}

	if unsafe.Sizeof(true) != 1 {
		t.Skip("bool is not one byte")
	}
	boolSrc := []bool{true, false, true}
	boolDst := make([]bool, len(boolSrc))
	if err := AllSumBytes(context.Background(), g, bytesOf(boolDst), bytesOf(boolSrc), DTypeBool); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(boolDst, boolSrc) {
		t.Fatalf("AllSumBytes bool = %v, want %v", boolDst, boolSrc)
	}
}

func TestAddUint8MatchesByteAddition(t *testing.T) {
	dst := []uint8{0, 1, 127, 128, 129, 254, 255, 200, 10, 250, 99}
	src := []uint8{0, 1, 1, 1, 127, 1, 1, 100, 255, 10, 201}
	want := append([]uint8(nil), dst...)
	for i := range want {
		want[i] += src[i]
	}
	addUint8(dst, src)
	if !reflect.DeepEqual(dst, want) {
		t.Fatalf("addUint8 = %v, want %v", dst, want)
	}
}

func TestFloat16Conversions(t *testing.T) {
	tests := []struct {
		name  string
		dtype DType
		bits  uint16
		want  float32
	}{
		{"float16 one", DTypeFloat16, 0x3c00, 1},
		{"float16 two", DTypeFloat16, 0x4000, 2},
		{"float16 negative zero", DTypeFloat16, 0x8000, float32(math.Copysign(0, -1))},
		{"bfloat16 one", DTypeBFloat16, 0x3f80, 1},
		{"bfloat16 two", DTypeBFloat16, 0x4000, 2},
	}
	for _, tt := range tests {
		got := float16ToFloat32(tt.bits, tt.dtype)
		if math.Float32bits(got) != math.Float32bits(tt.want) {
			t.Fatalf("%s = %08x, want %08x", tt.name, math.Float32bits(got), math.Float32bits(tt.want))
		}
		if got := float32ToFloat16(tt.want, tt.dtype); got != tt.bits {
			t.Fatalf("%s roundtrip = %#04x, want %#04x", tt.name, got, tt.bits)
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

func TestMergeGraphGatherPayloadRejectsInvalidFlag(t *testing.T) {
	known := []bool{true, false}
	values := [][]byte{[]byte{1}, nil}
	payload := []byte{1, 2, 1, 2}
	if err := mergeGraphGatherPayload(known, values, payload, 1); err == nil {
		t.Fatal("mergeGraphGatherPayload succeeded with invalid flag")
	}
}

func clearJACCLEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"JACCL_RANK",
		"MLX_RANK",
		"JACCL_SIZE",
		"MLX_WORLD_SIZE",
		"MLX_SIZE",
		"JACCL_COORDINATOR",
		"MLX_JACCL_COORDINATOR",
		"JACCL_IBV_DEVICES",
		"MLX_IBV_DEVICES",
		"JACCL_RING",
		"MLX_JACCL_RING",
	} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
	}
}

func writeDeviceMatrix(t *testing.T, matrix [][][]string) string {
	t.Helper()
	data, err := json.Marshal(matrix)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "devices.json")
	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatal(err)
	}
	return path
}
