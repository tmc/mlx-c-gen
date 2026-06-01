package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ml-explore/mlx-c/jaccl"
)

func main() {
	op := flag.String("op", "barrier-sum", "operation: barrier, barrier-sum, allgather, allmax, allmin, sendrecv, devices")
	timeout := flag.Duration("timeout", 20*time.Second, "operation timeout")
	localDevice := flag.String("local-two-rank-device", "", "run a local two-rank smoke using this RDMA device")
	coordinator := flag.String("coordinator", "127.0.0.1:39400", "coordinator address for -local-two-rank-device")
	flag.Parse()

	if *localDevice != "" {
		if err := runLocalTwoRank(*op, *timeout, *localDevice, *coordinator); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(*op, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runLocalTwoRank(op string, timeout time.Duration, device, coordinator string) error {
	dir, err := os.MkdirTemp("", "jacclnative-smoke.")
	if err != nil {
		return err
	}
	matrix := [][][]string{
		{nil, {device}},
		{{device}, nil},
	}
	data, err := json.Marshal(matrix)
	if err != nil {
		return err
	}
	devicesPath := filepath.Join(dir, "devices.json")
	if err := os.WriteFile(devicesPath, data, 0o600); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+5*time.Second)
	defer cancel()
	errs := make(chan error, 2)
	for rank := 0; rank < 2; rank++ {
		rank := rank
		cmd := exec.CommandContext(ctx, exe, "-op", op, "-timeout", timeout.String())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(),
			"JACCL_RANK="+fmt.Sprint(rank),
			"JACCL_SIZE=2",
			"JACCL_COORDINATOR="+coordinator,
			"JACCL_IBV_DEVICES="+devicesPath,
		)
		if rank == 0 {
			if err := cmd.Start(); err != nil {
				return err
			}
			go func() { errs <- cmd.Wait() }()
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { errs <- cmd.Wait() }()
	}
	var failed bool
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "rank process failed: %v\n", err)
		}
	}
	if failed {
		return fmt.Errorf("local two-rank smoke failed device=%s devices=%s", device, devicesPath)
	}
	return nil
}

func run(op string, timeout time.Duration) error {
	tracef("op=%s timeout=%s", op, timeout)
	if op == "devices" {
		names, err := jaccl.RDMADeviceNames()
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
	cfg, err := jaccl.ConfigFromEnv()
	if err != nil {
		return err
	}
	tracef("config rank=%d size=%d coordinator=%s devices=%s", cfg.Rank, cfg.Size, cfg.Coordinator, summarizeDevices(cfg.Devices))
	g, err := jaccl.NewGroup(ctx, cfg)
	if err != nil {
		return err
	}
	defer g.Close()
	tracef("group ready rank=%d size=%d", g.Rank(), g.Size())

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
		if err := jaccl.AllMax(ctx, g, dst, []int32{int32(g.Rank() + 1)}); err != nil {
			return err
		}
		if dst[0] != int32(g.Size()) {
			return fmt.Errorf("allmax = %d, want %d", dst[0], g.Size())
		}
		return nil
	case "allmin":
		dst := []int32{0}
		if err := jaccl.AllMin(ctx, g, dst, []int32{int32(g.Rank() + 1)}); err != nil {
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

func tracef(format string, args ...any) {
	if os.Getenv("JACCL_NATIVE_TRACE") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "jacclnative-smoke: "+format+"\n", args...)
}

func summarizeDevices(devices [][][]string) string {
	var b strings.Builder
	for i, row := range devices {
		if i > 0 {
			b.WriteString(";")
		}
		for j, wires := range row {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString("[")
			b.WriteString(strings.Join(wires, "|"))
			b.WriteString("]")
		}
	}
	return b.String()
}

func checkAllSum(ctx context.Context, g *jaccl.Group) error {
	dst := []int32{0}
	if err := jaccl.AllSum(ctx, g, dst, []int32{int32(g.Rank() + 1)}); err != nil {
		return err
	}
	want := int32(g.Size() * (g.Size() + 1) / 2)
	if dst[0] != want {
		return fmt.Errorf("allsum = %d, want %d", dst[0], want)
	}
	return nil
}

func checkAllGather(ctx context.Context, g *jaccl.Group) error {
	dst := make([]int32, g.Size())
	if err := jaccl.AllGather(ctx, g, dst, []int32{int32(g.Rank() + 1)}); err != nil {
		return err
	}
	for i, v := range dst {
		if v != int32(i+1) {
			return fmt.Errorf("allgather[%d] = %d, want %d", i, v, i+1)
		}
	}
	return nil
}

func checkSendRecv(ctx context.Context, g *jaccl.Group) error {
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
