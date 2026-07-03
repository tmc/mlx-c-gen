package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
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
	dtypeName := flag.String("dtype", "uint8", "allsum element type: uint8, float32, float16, or bfloat16")
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

	// Validate -dtype before any group/RDMA setup so a typo fails fast.
	if _, _, ok := dtypeByName(*dtypeName); !ok {
		fatal("dtype", fmt.Errorf("unknown dtype %q (want uint8, float32, float16, or bfloat16)", *dtypeName))
	}

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
		dtype, elem, _ := dtypeByName(*dtypeName) // validated after flag.Parse
		if *mode == "allgather" && dtype != jaccl.DTypeUint8 {
			fatal("dtype", fmt.Errorf("allgather is byte-only; -dtype applies to allsum"))
		}
		elems := *tokens * *hidden * 2 / elem.size
		input := elem.pattern(elems, *rank)
		sum := make([]byte, len(input))
		gather := make([]byte, *size*len(input))
		start = time.Now()
		for i := 0; i < *iters; i++ {
			switch *mode {
			case "allsum":
				if err := jaccl.AllSumBytes(ctx, group, sum, input, dtype); err != nil {
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
			if err := elem.checkSum(*rank, *size, input, sum); err != nil {
				fatal("check sum", err)
			}
		case "allgather":
			if err := checkGather(*size, len(input), gather); err != nil {
				fatal("check gather", err)
			}
		}
		fmt.Printf("rank %d %s dtype=%s iters=%d elapsed=%s ns_per_iter=%.0f\n", *rank, *mode, *dtypeName, *iters, elapsed, float64(elapsed.Nanoseconds())/float64(*iters))
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

// elemType describes how to build and validate an AllSum payload for one
// element type. For the float types the input is a whole number of exactly
// representable small integers keyed by rank (rank r holds r+1), so a two-rank
// sum is r0+r1 = 1+2 = 3, exact in float32/float16/bfloat16 with no rounding —
// the reduced output can be asserted with exact equality.
type elemType struct {
	size     int
	pattern  func(elems, rank int) []byte
	checkSum func(rank, size int, input, sum []byte) error
}

func dtypeByName(name string) (jaccl.DType, elemType, bool) {
	switch name {
	case "uint8":
		return jaccl.DTypeUint8, elemType{
			size:    1,
			pattern: func(elems, rank int) []byte { return pattern(elems, byte(rank)) },
			checkSum: func(rank, size int, input, sum []byte) error {
				return checkSum(rank, size, input, sum)
			},
		}, true
	case "float32":
		return jaccl.DTypeFloat32, floatElemType(4, encodeFloat32, decodeFloat32), true
	case "float16":
		return jaccl.DTypeFloat16, floatElemType(2, encodeFloat16, decodeFloat16), true
	case "bfloat16":
		return jaccl.DTypeBFloat16, floatElemType(2, encodeBFloat16, decodeBFloat16), true
	}
	return 0, elemType{}, false
}

// floatElemType builds an elemType for a float encoding of the given width. The
// rank-r input is elems copies of float64(r+1); the sum must be elems copies of
// float64(size*(size+1)/2) (1+2+…+size), asserted exactly.
func floatElemType(size int, enc func(float64) []byte, dec func([]byte) float64) elemType {
	return elemType{
		size: size,
		pattern: func(elems, rank int) []byte {
			b := make([]byte, 0, elems*size)
			for i := 0; i < elems; i++ {
				b = append(b, enc(float64(rank+1))...)
			}
			return b
		},
		checkSum: func(rank, size int, input, sum []byte) error {
			if size != 2 {
				return nil
			}
			var want float64
			for r := 0; r < size; r++ {
				want += float64(r + 1)
			}
			for off := 0; off < len(sum); off += len(enc(0)) {
				got := dec(sum[off : off+len(enc(0))])
				if got != want {
					return fmt.Errorf("sum mismatch at element %d: got %v want %v", off/len(enc(0)), got, want)
				}
			}
			return nil
		},
	}
}

func encodeFloat32(f float64) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, math.Float32bits(float32(f)))
	return b
}

func decodeFloat32(b []byte) float64 {
	return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
}

func encodeFloat16(f float64) []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, float32ToFloat16(float32(f), false))
	return b
}

func decodeFloat16(b []byte) float64 {
	return float64(float16ToFloat32(binary.LittleEndian.Uint16(b), false))
}

func encodeBFloat16(f float64) []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, float32ToFloat16(float32(f), true))
	return b
}

func decodeBFloat16(b []byte) float64 {
	return float64(float16ToFloat32(binary.LittleEndian.Uint16(b), true))
}

// float16/bfloat16 conversions limited to the finite, exactly representable
// small integers this tool uses (1, 2, 3, …); they intentionally do not handle
// NaN/Inf/subnormals. bf16 is the high 16 bits of the float32; ieee half uses a
// 5-bit exponent (bias 15) and 10-bit mantissa.
func float32ToFloat16(f float32, bf16 bool) uint16 {
	bits := math.Float32bits(f)
	if bf16 {
		return uint16(bits >> 16)
	}
	sign := uint16((bits >> 16) & 0x8000)
	exp := int((bits>>23)&0xff) - 127 + 15
	frac := uint16((bits >> 13) & 0x3ff)
	return sign | uint16(exp<<10) | frac
}

func float16ToFloat32(h uint16, bf16 bool) float32 {
	if bf16 {
		return math.Float32frombits(uint32(h) << 16)
	}
	sign := uint32(h&0x8000) << 16
	if h&0x7fff == 0 {
		return math.Float32frombits(sign)
	}
	exp := uint32((h>>10)&0x1f) - 15 + 127
	frac := uint32(h&0x3ff) << 13
	return math.Float32frombits(sign | exp<<23 | frac)
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
