package jaccl_test

import (
	"context"
	"fmt"

	"github.com/ml-explore/mlx-c/jaccl"
)

func Example() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	dst := []int32{0, 0, 0}
	if err := jaccl.AllSum(context.Background(), g, dst, []int32{1, 2, 3}); err != nil {
		panic(err)
	}
	fmt.Println(dst)
	// Output: [1 2 3]
}
