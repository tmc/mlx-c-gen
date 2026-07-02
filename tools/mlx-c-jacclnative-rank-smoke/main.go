package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tmc/mlx-c-gen/jaccl"
)

func main() {
	rank := flag.Int("rank", 0, "rank")
	size := flag.Int("size", 2, "group size")
	coordinator := flag.String("coordinator", "127.0.0.1:39091", "coordinator address")
	devices := flag.String("devices", "[[null,null],[null,null]]", "devices JSON")
	devicesFile := flag.String("devices-file", "", "devices JSON file")
	mode := flag.String("mode", "smoke", "mode: smoke, devices, ports, llm, allsum, or allgather")
	iters := flag.Int("iters", 1, "LLM forward iterations")
	tokens := flag.Int("tokens", 1, "tokens per LLM forward")
	hidden := flag.Int("hidden", 4096, "hidden size")
	layers := flag.Int("layers", 32, "layers per LLM forward")
	timeout := flag.Duration("timeout", 20*time.Second, "operation timeout")
	preferRing := flag.Bool("prefer-ring", false, "prefer ring topology")
	maxGIDs := flag.Int("max-gids", 16, "maximum GIDs to scan in ports mode")
	zeroDLIDWhenGlobal := flag.Bool("zero-dlid-when-global", false, "set DLID=0 when RTR AH uses a global route")
	grhHopLimit := flag.Int("grh-hop-limit", 0, "GRH hop limit override, 0 uses default")
	flag.Parse()

	if *mode == "devices" {
		names, err := jaccl.RDMADeviceNames()
		if err != nil {
			fatal("devices", err)
		}
		for _, name := range names {
			fmt.Println(name)
		}
		return
	}
	if *mode == "ports" {
		names, err := jaccl.RDMADeviceNames()
		if err != nil {
			fatal("devices", err)
		}
		for _, name := range names {
			info, err := jaccl.QueryRDMAPort(name, *maxGIDs)
			if err != nil {
				fmt.Printf("%s error=%q\n", name, err)
				continue
			}
			fmt.Printf("%s lid=%d active_mtu=%d gid_table_len=%d selected_gid_index=%d\n", name, info.LID, info.ActiveMTU, info.GIDTableLength, info.SelectedGIDIndex)
			for _, gid := range info.GIDs {
				fmt.Printf("%s gid[%d]=%s ipv4_mapped=%t zero=%t\n", name, gid.Index, hex.EncodeToString(gid.GID[:]), gid.IPv4Mapped, gid.Zero)
			}
		}
		return
	}

	matrix, err := readDevices(*devices, *devicesFile)
	if err != nil {
		fatal("devices", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	cfg := jaccl.Config{
		Rank:               *rank,
		Size:               *size,
		Coordinator:        *coordinator,
		Devices:            matrix,
		PreferRing:         *preferRing,
		ZeroDLIDWhenGlobal: *zeroDLIDWhenGlobal,
	}
	if *grhHopLimit < 0 || *grhHopLimit > 255 {
		fatal("grh hop limit", fmt.Errorf("%d out of uint8 range", *grhHopLimit))
	}
	cfg.GRHHopLimit = uint8(*grhHopLimit)

	start := time.Now()
	group, err := jaccl.NewGroup(ctx, cfg)
	if err != nil {
		fatal("new group", err)
	}
	defer group.Close()
	if err := jaccl.Barrier(ctx, group); err != nil {
		fatal("barrier", err)
	}
	if *mode == "smoke" {
		fmt.Printf("rank %d ok %s\n", *rank, time.Since(start))
		return
	}
	if *mode == "allsum" || *mode == "allgather" {
		n := *tokens * *hidden * 2
		input := pattern(n, byte(*rank))
		sum := make([]byte, n)
		gather := make([]byte, *size*n)
		start = time.Now()
		for i := 0; i < *iters; i++ {
			switch *mode {
			case "allsum":
				if err := jaccl.AllSumBytes(ctx, group, sum, input, jaccl.DTypeUint8); err != nil {
					fatal("all sum", err)
				}
			case "allgather":
				if err := jaccl.AllGatherBytes(ctx, group, gather, input); err != nil {
					fatal("all gather", err)
				}
			}
		}
		elapsed := time.Since(start)
		switch *mode {
		case "allsum":
			if err := checkSum(*rank, *size, input, sum); err != nil {
				fatal("check sum", err)
			}
		case "allgather":
			if err := checkGather(*size, len(input), gather); err != nil {
				fatal("check gather", err)
			}
		}
		fmt.Printf("rank %d %s iters=%d elapsed=%s ns_per_iter=%.0f\n", *rank, *mode, *iters, elapsed, float64(elapsed.Nanoseconds())/float64(*iters))
		return
	}
	if *mode != "llm" {
		fatal("mode", fmt.Errorf("unknown mode %q", *mode))
	}
	n := *tokens * *hidden * 2
	input := pattern(n, byte(*rank))
	sum := make([]byte, n)
	gather := make([]byte, *size*n)
	start = time.Now()
	for i := 0; i < *iters; i++ {
		for layer := 0; layer < *layers; layer++ {
			if err := jaccl.AllSumBytes(ctx, group, sum, input, jaccl.DTypeUint8); err != nil {
				fatal("all sum", err)
			}
			if err := jaccl.AllSumBytes(ctx, group, sum, input, jaccl.DTypeUint8); err != nil {
				fatal("all sum", err)
			}
			if err := jaccl.AllGatherBytes(ctx, group, gather, input); err != nil {
				fatal("all gather", err)
			}
		}
	}
	elapsed := time.Since(start)
	if err := checkOutput(*rank, *size, input, sum, gather); err != nil {
		fatal("check output", err)
	}
	fmt.Printf("rank %d llm iters=%d elapsed=%s ns_per_iter=%.0f\n", *rank, *iters, elapsed, float64(elapsed.Nanoseconds())/float64(*iters))
}

func readDevices(text, path string) ([][][]string, error) {
	var data []byte
	if path != "" {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
	} else {
		data = []byte(text)
	}
	var matrix [][][]string
	if err := json.Unmarshal(data, &matrix); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return matrix, nil
}

func fatal(op string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", op, err)
	os.Exit(1)
}

func pattern(n int, salt byte) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i) ^ salt
	}
	return p
}

func checkOutput(rank, size int, input, sum, gather []byte) error {
	if size != 2 {
		return nil
	}
	if err := checkSum(rank, size, input, sum); err != nil {
		return err
	}
	return checkGather(size, len(input), gather)
}

func checkSum(rank, size int, input, sum []byte) error {
	if size != 2 {
		return nil
	}
	peer := pattern(len(input), byte(1-rank))
	wantSum := make([]byte, len(input))
	for i := range wantSum {
		wantSum[i] = input[i] + peer[i]
	}
	if !bytes.Equal(sum, wantSum) {
		return fmt.Errorf("sum mismatch")
	}
	return nil
}

func checkGather(size, inputLen int, gather []byte) error {
	if size != 2 {
		return nil
	}
	wantGather := make([]byte, 0, len(gather))
	wantGather = append(wantGather, pattern(inputLen, 0)...)
	wantGather = append(wantGather, pattern(inputLen, 1)...)
	if !bytes.Equal(gather, wantGather) {
		return fmt.Errorf("gather mismatch")
	}
	return nil
}
