package main

import "testing"

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
