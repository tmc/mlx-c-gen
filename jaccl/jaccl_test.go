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

func TestPublicDTypeSize(t *testing.T) {
	tests := []struct {
		name string
		dt   DType
		want int
	}{
		{"bool", DTypeBool, 1},
		{"int8", DTypeInt8, 1},
		{"int16", DTypeInt16, 2},
		{"int32", DTypeInt32, 4},
		{"int64", DTypeInt64, 8},
		{"uint8", DTypeUint8, 1},
		{"uint16", DTypeUint16, 2},
		{"uint32", DTypeUint32, 4},
		{"uint64", DTypeUint64, 8},
		{"float16", DTypeFloat16, 2},
		{"bfloat16", DTypeBFloat16, 2},
		{"float32", DTypeFloat32, 4},
		{"float64", DTypeFloat64, 8},
		{"complex64", DTypeComplex64, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.dt.Size()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("%v.Size() = %d, want %d", tt.dt, got, tt.want)
			}
		})
	}
	if _, err := DType(-1).Size(); err == nil {
		t.Fatal("invalid dtype Size succeeded")
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
