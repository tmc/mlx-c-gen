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

func ExampleBarrier() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	if err := jaccl.Barrier(context.Background(), g); err != nil {
		panic(err)
	}
	fmt.Println("ok")
	// Output: ok
}

func ExampleAllSumBytes() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	dst := []byte{0, 0, 0}
	if err := jaccl.AllSumBytes(context.Background(), g, dst, []byte{1, 2, 3}, jaccl.DTypeUint8); err != nil {
		panic(err)
	}
	fmt.Println(dst)
	// Output: [1 2 3]
}

func ExampleAllGatherBytes() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	dst := make([]byte, g.Size()*3)
	if err := jaccl.AllGatherBytes(context.Background(), g, dst, []byte{1, 2, 3}); err != nil {
		panic(err)
	}
	fmt.Println(dst)
	// Output: [1 2 3]
}

func ExampleSend() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	err = jaccl.Send(context.Background(), g, 1, []byte{1, 2, 3})
	fmt.Println(err != nil)
	// Output: true
}

func ExampleRecv() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	err = jaccl.Recv(context.Background(), g, 1, make([]byte, 3))
	fmt.Println(err != nil)
	// Output: true
}
