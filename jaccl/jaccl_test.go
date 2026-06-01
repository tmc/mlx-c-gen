package jaccl

import (
	"context"
	"testing"
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
	dst := make([]int32, 2)
	if err := AllMax(context.Background(), g, dst, []int32{3, 4}); err != nil {
		t.Fatal(err)
	}
	if dst[0] != 3 || dst[1] != 4 {
		t.Fatalf("AllMax = %v, want [3 4]", dst)
	}
}
