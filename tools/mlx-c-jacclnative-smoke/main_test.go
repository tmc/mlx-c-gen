package main

import (
	"encoding/binary"
	"testing"
)

func TestLineMatrix(t *testing.T) {
	matrix, err := lineMatrix([]string{"rdma_en1", " rdma_en3 "})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(matrix), 3; got != want {
		t.Fatalf("len(matrix) = %d, want %d", got, want)
	}
	if got := matrix[0][1][0]; got != "rdma_en1" {
		t.Fatalf("matrix[0][1] = %q", got)
	}
	if got := matrix[1][2][0]; got != "rdma_en3" {
		t.Fatalf("matrix[1][2] = %q", got)
	}
	if len(matrix[0][2]) != 0 {
		t.Fatalf("matrix[0][2] = %v, want no direct edge", matrix[0][2])
	}
}

func TestLineMatrixEmptyDevice(t *testing.T) {
	if _, err := lineMatrix([]string{"rdma_en1", ""}); err == nil {
		t.Fatal("lineMatrix succeeded with empty device")
	}
}

func TestRingMatrix(t *testing.T) {
	matrix, err := ringMatrix([]string{"rdma_en1", " rdma_en2 ", "rdma_en3", "rdma_en4"})
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix) != 4 {
		t.Fatalf("len(matrix) = %d, want 4", len(matrix))
	}
	tests := []struct {
		src, dst int
		want     string
	}{
		{0, 1, "rdma_en1"},
		{1, 0, "rdma_en1"},
		{1, 2, "rdma_en2"},
		{2, 1, "rdma_en2"},
		{2, 3, "rdma_en3"},
		{3, 2, "rdma_en3"},
		{3, 0, "rdma_en4"},
		{0, 3, "rdma_en4"},
	}
	for _, tt := range tests {
		got := matrix[tt.src][tt.dst]
		if len(got) != 1 || got[0] != tt.want {
			t.Fatalf("matrix[%d][%d] = %v, want [%s]", tt.src, tt.dst, got, tt.want)
		}
	}
	if len(matrix[0][2]) != 0 {
		t.Fatalf("matrix[0][2] = %v, want no direct edge", matrix[0][2])
	}
	if _, err := ringMatrix([]string{"rdma_en1"}); err == nil {
		t.Fatal("ringMatrix succeeded with one device")
	}
	if _, err := ringMatrix([]string{"rdma_en1", ""}); err == nil {
		t.Fatal("ringMatrix succeeded with empty device")
	}
}

func TestFloat16Bytes(t *testing.T) {
	got := float16Bytes(1, 2)
	if binary.LittleEndian.Uint16(got[0:]) != 0x3c00 {
		t.Fatalf("float16Bytes(1) = %#04x, want 0x3c00", binary.LittleEndian.Uint16(got[0:]))
	}
	if binary.LittleEndian.Uint16(got[2:]) != 0x4000 {
		t.Fatalf("float16Bytes(2) = %#04x, want 0x4000", binary.LittleEndian.Uint16(got[2:]))
	}
}

func TestBFloat16Bytes(t *testing.T) {
	got := bfloat16Bytes(1, 2)
	if binary.LittleEndian.Uint16(got[0:]) != 0x3f80 {
		t.Fatalf("bfloat16Bytes(1) = %#04x, want 0x3f80", binary.LittleEndian.Uint16(got[0:]))
	}
	if binary.LittleEndian.Uint16(got[2:]) != 0x4000 {
		t.Fatalf("bfloat16Bytes(2) = %#04x, want 0x4000", binary.LittleEndian.Uint16(got[2:]))
	}
}
