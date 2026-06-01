package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ml-explore/mlx-c/internal/jacclnative"
)

func main() {
	op := flag.String("op", "barrier-sum", "operation: barrier, barrier-sum, allgather, allmax, allmin, sendrecv, devices")
	timeout := flag.Duration("timeout", 20*time.Second, "operation timeout")
	flag.Parse()

	if err := run(*op, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(op string, timeout time.Duration) error {
	if op == "devices" {
		names, err := jacclnative.RDMADeviceNames()
		if err != nil {
			return err
		}
		for _, name := range names {
			fmt.Println(name)
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cfg, err := jacclnative.ConfigFromEnv()
	if err != nil {
		return err
	}
	g, err := jacclnative.NewGroup(ctx, cfg)
	if err != nil {
		return err
	}
	defer g.Close()

	switch op {
	case "barrier":
		return g.Barrier(ctx)
	case "barrier-sum":
		if err := g.Barrier(ctx); err != nil {
			return err
		}
		return checkAllSum(ctx, g)
	case "allgather":
		return checkAllGather(ctx, g)
	case "allmax":
		dst := []int32{0}
		if err := jacclnative.AllMax(ctx, g, dst, []int32{int32(g.Rank() + 1)}); err != nil {
			return err
		}
		if dst[0] != int32(g.Size()) {
			return fmt.Errorf("allmax = %d, want %d", dst[0], g.Size())
		}
		return nil
	case "allmin":
		dst := []int32{0}
		if err := jacclnative.AllMin(ctx, g, dst, []int32{int32(g.Rank() + 1)}); err != nil {
			return err
		}
		if dst[0] != 1 {
			return fmt.Errorf("allmin = %d, want 1", dst[0])
		}
		return nil
	case "sendrecv":
		return checkSendRecv(ctx, g)
	default:
		return fmt.Errorf("unknown op %q", op)
	}
}

func checkAllSum(ctx context.Context, g *jacclnative.Group) error {
	dst := []int32{0}
	if err := jacclnative.AllSum(ctx, g, dst, []int32{int32(g.Rank() + 1)}); err != nil {
		return err
	}
	want := int32(g.Size() * (g.Size() + 1) / 2)
	if dst[0] != want {
		return fmt.Errorf("allsum = %d, want %d", dst[0], want)
	}
	return nil
}

func checkAllGather(ctx context.Context, g *jacclnative.Group) error {
	dst := make([]int32, g.Size())
	if err := jacclnative.AllGather(ctx, g, dst, []int32{int32(g.Rank() + 1)}); err != nil {
		return err
	}
	for i, v := range dst {
		if v != int32(i+1) {
			return fmt.Errorf("allgather[%d] = %d, want %d", i, v, i+1)
		}
	}
	return nil
}

func checkSendRecv(ctx context.Context, g *jacclnative.Group) error {
	if g.Size() != 2 {
		return fmt.Errorf("sendrecv requires size 2, got %d", g.Size())
	}
	switch g.Rank() {
	case 0:
		if err := g.Send(ctx, 1, []byte("hello")); err != nil {
			return err
		}
		return g.Recv(ctx, 1, nil)
	case 1:
		buf := make([]byte, 5)
		if err := g.Recv(ctx, 0, buf); err != nil {
			return err
		}
		if string(buf) != "hello" {
			return fmt.Errorf("recv = %q, want hello", buf)
		}
		return g.Send(ctx, 0, nil)
	default:
		return fmt.Errorf("sendrecv rank %d out of range", g.Rank())
	}
}
