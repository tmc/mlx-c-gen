package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tmc/mlx-c-gen/jaccl"
)

func main() {
	op := flag.String("op", "barrier-sum", "operation: barrier, barrier-sum, allgather, allgather-bytes, allgather-large, allsum-bytes, allsum-large, allsum-half, allmax, allmax-bytes, allmax-large, allmax-bfloat, allmin, allmin-bytes, allmin-large, sendrecv, devices")
	timeout := flag.Duration("timeout", 20*time.Second, "operation timeout")
	localDevice := flag.String("local-two-rank-device", "", "run a local two-rank smoke using this RDMA device")
	localLine := flag.String("local-line-devices", "", "run a local line-topology smoke with comma-separated RDMA devices")
	localRing := flag.String("local-ring-devices", "", "run a local ring-topology smoke with comma-separated RDMA devices")
	coordinator := flag.String("coordinator", "127.0.0.1:0", "coordinator address for local launchers")
	flag.Parse()

	if *localDevice != "" {
		if err := runLocal(*op, *timeout, *coordinator, twoRankMatrix(*localDevice), false); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *localLine != "" {
		matrix, err := lineMatrix(strings.Split(*localLine, ","))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := runLocal(*op, *timeout, *coordinator, matrix, false); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *localRing != "" {
		matrix, err := ringMatrix(strings.Split(*localRing, ","))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := runLocal(*op, *timeout, *coordinator, matrix, true); err != nil {
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

func runLocal(op string, timeout time.Duration, coordinator string, matrix [][][]string, preferRing bool) error {
	coordinator, err := localCoordinator(coordinator)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "jacclnative-smoke.")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()
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
	errs := make(chan error, len(matrix))
	for rank := range matrix {
		rank := rank
		cmd := exec.CommandContext(ctx, exe, "-op", op, "-timeout", timeout.String())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(),
			"JACCL_RANK="+fmt.Sprint(rank),
			"JACCL_SIZE="+fmt.Sprint(len(matrix)),
			"JACCL_COORDINATOR="+coordinator,
			"JACCL_IBV_DEVICES="+devicesPath,
		)
		if preferRing {
			cmd.Env = append(cmd.Env, "JACCL_RING=1")
		}
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
	for range matrix {
		if err := <-errs; err != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "rank process failed: %v\n", err)
		}
	}
	if failed {
		cleanup = false
		return fmt.Errorf("local smoke failed coordinator=%s devices=%s", coordinator, devicesPath)
	}
	return nil
}

func localCoordinator(addr string) (string, error) {
	if !strings.HasSuffix(addr, ":0") {
		return addr, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("coordinator: listen %s: %w", addr, err)
	}
	defer ln.Close()
	return ln.Addr().String(), nil
}

func twoRankMatrix(device string) [][][]string {
	return [][][]string{
		{nil, {device}},
		{{device}, nil},
	}
}

func lineMatrix(devices []string) ([][][]string, error) {
	for i, device := range devices {
		devices[i] = strings.TrimSpace(device)
	}
	if len(devices) == 0 || len(devices) == 1 && devices[0] == "" {
		return nil, fmt.Errorf("local line requires at least one device")
	}
	size := len(devices) + 1
	matrix := make([][][]string, size)
	for i := range matrix {
		matrix[i] = make([][]string, size)
	}
	for i, device := range devices {
		if device == "" {
			return nil, fmt.Errorf("local line device %d is empty", i)
		}
		matrix[i][i+1] = []string{device}
		matrix[i+1][i] = []string{device}
	}
	return matrix, nil
}

func ringMatrix(devices []string) ([][][]string, error) {
	for i, device := range devices {
		devices[i] = strings.TrimSpace(device)
	}
	if len(devices) < 2 {
		return nil, fmt.Errorf("local ring requires at least two devices")
	}
	size := len(devices)
	matrix := make([][][]string, size)
	for i := range matrix {
		matrix[i] = make([][]string, size)
	}
	for i, device := range devices {
		if device == "" {
			return nil, fmt.Errorf("local ring device %d is empty", i)
		}
		next := (i + 1) % size
		matrix[i][next] = []string{device}
		matrix[next][i] = []string{device}
	}
	return matrix, nil
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
	tracef("config rank=%d size=%d coordinator=%s prefer_ring=%t devices=%s", cfg.Rank, cfg.Size, cfg.Coordinator, cfg.PreferRing, summarizeDevices(cfg.Devices))
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
	case "allgather-bytes":
		return checkAllGatherBytes(ctx, g)
	case "allgather-large":
		return checkAllGatherLarge(ctx, g)
	case "allsum-bytes":
		return checkAllSumBytes(ctx, g)
	case "allsum-large":
		return checkAllSumLarge(ctx, g)
	case "allsum-half":
		return checkAllSumHalf(ctx, g)
	case "allmax":
		dst := []int32{0}
		if err := jaccl.AllMax(ctx, g, dst, []int32{int32(g.Rank() + 1)}); err != nil {
			return err
		}
		if dst[0] != int32(g.Size()) {
			return fmt.Errorf("allmax = %d, want %d", dst[0], g.Size())
		}
		return nil
	case "allmax-bytes":
		return checkAllMaxBytes(ctx, g)
	case "allmax-large":
		return checkAllMaxLarge(ctx, g)
	case "allmax-bfloat":
		return checkAllMaxBFloat(ctx, g)
	case "allmin":
		dst := []int32{0}
		if err := jaccl.AllMin(ctx, g, dst, []int32{int32(g.Rank() + 1)}); err != nil {
			return err
		}
		if dst[0] != 1 {
			return fmt.Errorf("allmin = %d, want 1", dst[0])
		}
		return nil
	case "allmin-bytes":
		return checkAllMinBytes(ctx, g)
	case "allmin-large":
		return checkAllMinLarge(ctx, g)
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

func checkAllSumBytes(ctx context.Context, g *jaccl.Group) error {
	dst := []byte{0}
	src := []byte{byte(g.Rank() + 1)}
	if err := jaccl.AllSumBytes(ctx, g, dst, src, jaccl.DTypeUint8); err != nil {
		return err
	}
	want := byte(g.Size() * (g.Size() + 1) / 2)
	if dst[0] != want {
		return fmt.Errorf("allsum-bytes = %d, want %d", dst[0], want)
	}
	return nil
}

func checkAllSumLarge(ctx context.Context, g *jaccl.Group) error {
	n := largePayloadBytes(g.Size())
	dst := make([]byte, n)
	src := make([]byte, n)
	want := make([]byte, n)
	for i := range src {
		src[i] = byte((g.Rank() + 1) * (i + 1))
		for rank := 0; rank < g.Size(); rank++ {
			want[i] += byte((rank + 1) * (i + 1))
		}
	}
	if err := jaccl.AllSumBytes(ctx, g, dst, src, jaccl.DTypeUint8); err != nil {
		return err
	}
	if string(dst) != string(want) {
		return fmt.Errorf("allsum-large mismatch bytes=%d", n)
	}
	return nil
}

func checkAllSumHalf(ctx context.Context, g *jaccl.Group) error {
	dst := make([]byte, 2)
	src := float16Bytes(float32(g.Rank() + 1))
	if err := jaccl.AllSumBytes(ctx, g, dst, src, jaccl.DTypeFloat16); err != nil {
		return err
	}
	want := float16Bytes(float32(g.Size() * (g.Size() + 1) / 2))
	if string(dst) != string(want) {
		return fmt.Errorf("allsum-half = %v, want %v", dst, want)
	}
	return nil
}

func checkAllMaxBytes(ctx context.Context, g *jaccl.Group) error {
	dst := []byte{0}
	src := []byte{byte(g.Rank() + 1)}
	if err := jaccl.AllMaxBytes(ctx, g, dst, src, jaccl.DTypeUint8); err != nil {
		return err
	}
	if dst[0] != byte(g.Size()) {
		return fmt.Errorf("allmax-bytes = %d, want %d", dst[0], g.Size())
	}
	return nil
}

func checkAllMaxLarge(ctx context.Context, g *jaccl.Group) error {
	n := largePayloadBytes(g.Size())
	dst := make([]byte, n)
	src := make([]byte, n)
	want := make([]byte, n)
	for i := range src {
		src[i] = byte((g.Rank() + 1) * (i + 1))
		for rank := 0; rank < g.Size(); rank++ {
			v := byte((rank + 1) * (i + 1))
			if v > want[i] {
				want[i] = v
			}
		}
	}
	if err := jaccl.AllMaxBytes(ctx, g, dst, src, jaccl.DTypeUint8); err != nil {
		return err
	}
	if string(dst) != string(want) {
		return fmt.Errorf("allmax-large mismatch bytes=%d", n)
	}
	return nil
}

func checkAllMaxBFloat(ctx context.Context, g *jaccl.Group) error {
	dst := make([]byte, 2)
	src := bfloat16Bytes(float32(g.Rank() + 1))
	if err := jaccl.AllMaxBytes(ctx, g, dst, src, jaccl.DTypeBFloat16); err != nil {
		return err
	}
	want := bfloat16Bytes(float32(g.Size()))
	if string(dst) != string(want) {
		return fmt.Errorf("allmax-bfloat = %v, want %v", dst, want)
	}
	return nil
}

func checkAllMinBytes(ctx context.Context, g *jaccl.Group) error {
	dst := []byte{0}
	src := []byte{byte(g.Rank() + 1)}
	if err := jaccl.AllMinBytes(ctx, g, dst, src, jaccl.DTypeUint8); err != nil {
		return err
	}
	if dst[0] != 1 {
		return fmt.Errorf("allmin-bytes = %d, want 1", dst[0])
	}
	return nil
}

func checkAllMinLarge(ctx context.Context, g *jaccl.Group) error {
	n := largePayloadBytes(g.Size())
	dst := make([]byte, n)
	src := make([]byte, n)
	want := make([]byte, n)
	for i := range src {
		src[i] = byte((g.Rank() + 1) * (i + 1))
		want[i] = byte(i + 1)
	}
	if err := jaccl.AllMinBytes(ctx, g, dst, src, jaccl.DTypeUint8); err != nil {
		return err
	}
	if string(dst) != string(want) {
		return fmt.Errorf("allmin-large mismatch bytes=%d", n)
	}
	return nil
}

func float16Bytes(values ...float32) []byte {
	b := make([]byte, 2*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint16(b[2*i:], float32ToFloat16(v))
	}
	return b
}

func bfloat16Bytes(values ...float32) []byte {
	b := make([]byte, 2*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint16(b[2*i:], float32ToBFloat16(v))
	}
	return b
}

func float32ToBFloat16(f float32) uint16 {
	if math.IsNaN(float64(f)) {
		return 0x7fc0
	}
	bits := math.Float32bits(f)
	bits += (bits >> 16 & 1) + 0x7fff
	return uint16(bits >> 16)
}

func float32ToFloat16(f float32) uint16 {
	bits := math.Float32bits(f)
	sign := uint16((bits >> 16) & 0x8000)
	exp := int((bits >> 23) & 0xff)
	frac := bits & 0x7fffff
	if exp == 0xff {
		if frac != 0 {
			return sign | 0x7d00
		}
		return sign | 0x7c00
	}
	exp16 := exp - 127 + 15
	if exp16 >= 0x1f {
		return sign | 0x7c00
	}
	if exp16 <= 0 {
		if exp16 < -10 {
			return sign
		}
		frac |= 0x800000
		shift := uint(14 - exp16)
		half := uint16(frac >> shift)
		if frac>>(shift-1)&1 != 0 {
			half++
		}
		return sign | half
	}
	half := sign | uint16(exp16<<10) | uint16(frac>>13)
	if frac&0x00001000 != 0 {
		half++
	}
	return half
}

func checkAllGatherBytes(ctx context.Context, g *jaccl.Group) error {
	dst := make([]byte, g.Size())
	src := []byte{byte(g.Rank() + 1)}
	if err := jaccl.AllGatherBytes(ctx, g, dst, src); err != nil {
		return err
	}
	for i, v := range dst {
		if v != byte(i+1) {
			return fmt.Errorf("allgather-bytes[%d] = %d, want %d", i, v, i+1)
		}
	}
	return nil
}

func checkAllGatherLarge(ctx context.Context, g *jaccl.Group) error {
	n := largePayloadBytes(g.Size())
	dst := make([]byte, g.Size()*n)
	src := make([]byte, n)
	for i := range src {
		src[i] = byte((g.Rank() + 1) * (i + 1))
	}
	if err := jaccl.AllGatherBytes(ctx, g, dst, src); err != nil {
		return err
	}
	for rank := 0; rank < g.Size(); rank++ {
		for i := range src {
			want := byte((rank + 1) * (i + 1))
			if got := dst[rank*n+i]; got != want {
				return fmt.Errorf("allgather-large rank=%d byte=%d got=%d want=%d", rank, i, got, want)
			}
		}
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

func largePayloadBytes(size int) int {
	const stagingBytes = 4096 << 7
	if size <= 0 || size >= stagingBytes {
		return stagingBytes
	}
	return (stagingBytes-size)/size + 123
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
