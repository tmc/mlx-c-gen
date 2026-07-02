package jaccl_test

import (
	"context"
	"fmt"

	"github.com/tmc/mlx-c-gen/jaccl"
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

func ExampleGroupSize() {
	cfg := jaccl.Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1"}},
			{{"rdma_en1"}, nil},
		},
	}
	size, err := jaccl.GroupSize(cfg)
	if err != nil {
		panic(err)
	}
	fmt.Println(size)
	// Output: 2
}

func ExampleIsValidMesh() {
	cfg := jaccl.Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1"}},
			{{"rdma_en1"}, nil},
		},
	}
	fmt.Println(jaccl.IsValidMesh(cfg))
	// Output: true
}

func ExampleIsValidRing() {
	cfg := jaccl.Config{
		Rank: 0,
		Devices: [][][]string{
			{nil, {"rdma_en1"}, nil, {"rdma_en4"}},
			{{"rdma_en1"}, nil, {"rdma_en2"}, nil},
			{nil, {"rdma_en2"}, nil, {"rdma_en3"}},
			{{"rdma_en4"}, nil, {"rdma_en3"}, nil},
		},
	}
	fmt.Println(jaccl.IsValidRing(cfg))
	// Output: true
}

func ExampleAllMax() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	dst := []int32{0, 0, 0}
	if err := jaccl.AllMax(context.Background(), g, dst, []int32{1, 3, 2}); err != nil {
		panic(err)
	}
	fmt.Println(dst)
	// Output: [1 3 2]
}

func ExampleAllMin() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	dst := []int32{0, 0, 0}
	if err := jaccl.AllMin(context.Background(), g, dst, []int32{3, 1, 2}); err != nil {
		panic(err)
	}
	fmt.Println(dst)
	// Output: [3 1 2]
}

func ExampleAllGather() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	dst := make([]int32, g.Size()*3)
	if err := jaccl.AllGather(context.Background(), g, dst, []int32{1, 2, 3}); err != nil {
		panic(err)
	}
	fmt.Println(dst)
	// Output: [1 2 3]
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

func ExampleAllMaxBytes() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	dst := []byte{0, 0, 0}
	if err := jaccl.AllMaxBytes(context.Background(), g, dst, []byte{1, 2, 3}, jaccl.DTypeUint8); err != nil {
		panic(err)
	}
	fmt.Println(dst)
	// Output: [1 2 3]
}

func ExampleAllMinBytes() {
	g, err := jaccl.NewGroup(context.Background(), jaccl.Config{Rank: 0, Size: 1})
	if err != nil {
		panic(err)
	}
	defer g.Close()

	dst := []byte{0, 0, 0}
	if err := jaccl.AllMinBytes(context.Background(), g, dst, []byte{1, 2, 3}, jaccl.DTypeUint8); err != nil {
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
