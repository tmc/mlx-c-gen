package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tmc/mlx-c-gen/internal/jacclc"
)

func main() {
	lib := flag.String("lib", "", "libjacclc path")
	rank := flag.Int("rank", 0, "rank")
	size := flag.Int("size", 2, "group size")
	coordinator := flag.String("coordinator", "127.0.0.1:39091", "coordinator address")
	devices := flag.String("devices", "[[null,null],[null,null]]", "devices JSON")
	mode := flag.String("mode", "smoke", "mode: smoke, llm, allsum, or allgather")
	iters := flag.Int("iters", 1, "LLM forward iterations")
	tokens := flag.Int("tokens", 1, "tokens per LLM forward")
	hidden := flag.Int("hidden", 4096, "hidden size")
	layers := flag.Int("layers", 32, "layers per LLM forward")
	flag.Parse()

	if *lib != "" {
		if err := jacclc.LoadPath(*lib); err != nil {
			fatal("load", err)
		}
	}
	config, err := jacclc.NewConfig()
	if err != nil {
		fatal("new config", err)
	}
	defer config.Close()
	if err := config.SetRank(*rank); err != nil {
		fatal("set rank", err)
	}
	if err := config.SetCoordinator(*coordinator); err != nil {
		fatal("set coordinator", err)
	}
	if *size != 2 {
		fatal("size", fmt.Errorf("only size 2 smoke is supported"))
	}
	if err := config.SetDevicesJSON(*devices); err != nil {
		fatal("set devices", err)
	}

	start := time.Now()
	group, err := jacclc.NewGroupWithConfig(config, false)
	if err != nil {
		fatal("new group", err)
	}
	defer group.Close()
	if err := group.Barrier(); err != nil {
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
				if err := group.AllSumBytes(input, sum, jacclc.DTypeUint8); err != nil {
					fatal("all sum", err)
				}
			case "allgather":
				if err := group.AllGatherBytes(input, gather); err != nil {
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
			if err := group.AllSumBytes(input, sum, jacclc.DTypeUint8); err != nil {
				fatal("all sum", err)
			}
			if err := group.AllSumBytes(input, sum, jacclc.DTypeUint8); err != nil {
				fatal("all sum", err)
			}
			if err := group.AllGatherBytes(input, gather); err != nil {
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
