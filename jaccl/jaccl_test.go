package jaccl

import (
	"context"
	"reflect"
	"testing"
	"unsafe"
)

func TestSingleRankPublicAPI(t *testing.T) {
	g, err := NewGroup(context.Background(), Config{Rank: 0, Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	if g.Rank() != 0 || g.Size() != 1 {
		t.Fatalf("rank/size = %d/%d, want 0/1", g.Rank(), g.Size())
	}
	if err := Barrier(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	dst := make([]int32, 2)
	if err := AllMax(context.Background(), g, dst, []int32{3, 4}); err != nil {
		t.Fatal(err)
	}
	if dst[0] != 3 || dst[1] != 4 {
		t.Fatalf("AllMax = %v, want [3 4]", dst)
	}
	raw := make([]byte, 4)
	if err := AllGatherBytes(context.Background(), g, raw, []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	if string(raw) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("AllGatherBytes = %v, want [1 2 3 4]", raw)
	}
	src := []int32{5, 6}
	rawDst := make([]int32, len(src))
	if err := AllSumBytes(context.Background(), g, bytesOf(rawDst), bytesOf(src), DTypeInt32); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rawDst, src) {
		t.Fatalf("AllSumBytes = %v, want %v", rawDst, src)
	}
	if err := Send(context.Background(), g, 0, []byte{1}); err == nil {
		t.Fatal("Send in single-rank group succeeded")
	}
	if err := Recv(context.Background(), g, 0, make([]byte, 1)); err == nil {
		t.Fatal("Recv in single-rank group succeeded")
	}
}

func TestPublicTopologyQueries(t *testing.T) {
	cfg := Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1"}, nil, {"rdma_en4"}},
			{{"rdma_en1"}, nil, {"rdma_en2"}, nil},
			{nil, {"rdma_en2"}, nil, {"rdma_en3"}},
			{{"rdma_en4"}, nil, {"rdma_en3"}, nil},
		},
	}
	if !IsValidRing(cfg) {
		t.Fatal("IsValidRing failed")
	}
	if IsValidMesh(cfg) {
		t.Fatal("IsValidMesh succeeded")
	}
}

func TestNilGroupPublicAPI(t *testing.T) {
	if err := Barrier(context.Background(), nil); err == nil {
		t.Fatal("Barrier nil group succeeded")
	}
	if err := Send(context.Background(), nil, 0, nil); err == nil {
		t.Fatal("Send nil group succeeded")
	}
	if err := Recv(context.Background(), nil, 0, nil); err == nil {
		t.Fatal("Recv nil group succeeded")
	}
}

func bytesOf[T Element](x []T) []byte {
	if len(x) == 0 {
		return nil
	}
	size := unsafe.Sizeof(x[0])
	return unsafe.Slice((*byte)(unsafe.Pointer(&x[0])), len(x)*int(size))
}
