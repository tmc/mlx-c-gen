package jacclnative

import (
	"context"
	"encoding/binary"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

type memRecv struct {
	offset int
	length int
	id     uint64
}

type memSend struct {
	offset int
	length int
	id     uint64
}

type memCompletion struct {
	wr  rdmaWorkRequest
	err error
}

type memTransport struct {
	mu   *sync.Mutex
	cond *sync.Cond

	sendBufBytes []byte
	recvBufBytes []byte
	peer         *memTransport

	pendingRecvs []memRecv
	pendingSends []memSend
	completions  []memCompletion

	drained  bool
	poisoned bool
}

func newMemPair() (*memTransport, *memTransport) {
	mu := &sync.Mutex{}
	cond := sync.NewCond(mu)
	n := pipelineDepth * rdmaStagingBytes
	a := &memTransport{mu: mu, cond: cond, sendBufBytes: make([]byte, n), recvBufBytes: make([]byte, n)}
	b := &memTransport{mu: mu, cond: cond, sendBufBytes: make([]byte, n), recvBufBytes: make([]byte, n)}
	a.peer = b
	b.peer = a
	return a, b
}

func (t *memTransport) sendBuf() []byte { return t.sendBufBytes }
func (t *memTransport) recvBuf() []byte { return t.recvBufBytes }

func (t *memTransport) postSend(offset, length int, id uint64) error {
	if offset < 0 || length < 0 || offset+length > len(t.sendBufBytes) {
		return fmt.Errorf("post send: range [%d,%d) outside buffer length %d", offset, offset+length, len(t.sendBufBytes))
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pendingSends = append(t.pendingSends, memSend{offset: offset, length: length, id: id})
	t.matchLocked()
	t.cond.Broadcast()
	return nil
}

func (t *memTransport) postRecv(offset, length int, id uint64) error {
	if offset < 0 || length < 0 || offset+length > len(t.recvBufBytes) {
		return fmt.Errorf("post recv: range [%d,%d) outside buffer length %d", offset, offset+length, len(t.recvBufBytes))
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pendingRecvs = append(t.pendingRecvs, memRecv{offset: offset, length: length, id: id})
	t.peer.matchLocked()
	t.cond.Broadcast()
	return nil
}

func (t *memTransport) matchLocked() {
	peer := t.peer
	for len(t.pendingSends) > 0 && len(peer.pendingRecvs) > 0 {
		s := t.pendingSends[0]
		r := peer.pendingRecvs[0]
		t.pendingSends = t.pendingSends[1:]
		peer.pendingRecvs = peer.pendingRecvs[1:]
		if s.length > r.length {
			err := fmt.Errorf("work completion opcode %d status %d", 128, 1)
			t.completions = append(t.completions, memCompletion{wr: rdmaWorkRequest{ID: s.id}, err: err})
			peer.completions = append(peer.completions, memCompletion{wr: rdmaWorkRequest{ID: r.id}, err: err})
			continue
		}
		copy(peer.recvBufBytes[r.offset:r.offset+s.length], t.sendBufBytes[s.offset:s.offset+s.length])
		// A send completion does not report a transfer length; a receive
		// completion reports the bytes the peer actually delivered.
		t.completions = append(t.completions, memCompletion{wr: rdmaWorkRequest{ID: s.id}})
		peer.completions = append(peer.completions, memCompletion{wr: rdmaWorkRequest{ID: r.id, Bytes: s.length}})
	}
}

func (t *memTransport) drain() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.drained = true
	t.poisoned = true
	return nil
}

func (t *memTransport) poll(ctx context.Context, n int) ([]rdmaWorkRequest, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.poisoned {
		return nil, errTransportPoisoned
	}
	done := make([]rdmaWorkRequest, 0, n)
	for len(done) < n {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(t.completions) > 0 {
			c := t.completions[0]
			t.completions = t.completions[1:]
			if c.err != nil {
				return nil, c.err
			}
			done = append(done, c.wr)
			continue
		}
		waitCond(ctx, t.cond)
	}
	return done, nil
}

func waitCond(ctx context.Context, cond *sync.Cond) {
	if ctx.Err() != nil {
		return
	}
	if ctx.Done() == nil {
		cond.Wait()
		return
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			cond.L.Lock()
			cond.Broadcast()
			cond.L.Unlock()
		case <-done:
		}
	}()
	cond.Wait()
	close(done)
}

func memGroups(size int) []*Group {
	backends := make([]*nativeBackend, size)
	for rank := range backends {
		backends[rank] = &nativeBackend{
			rank:  rank,
			size:  size,
			mesh:  true,
			conns: make([]*rdmaConnGroup, size),
		}
	}
	for i := 0; i < size; i++ {
		for j := i + 1; j < size; j++ {
			ij, ji := newMemPair()
			backends[i].conns[j] = &rdmaConnGroup{wires: []*rdmaConn{{t: ij}}}
			backends[j].conns[i] = &rdmaConnGroup{wires: []*rdmaConn{{t: ji}}}
		}
	}
	groups := make([]*Group, size)
	for rank, b := range backends {
		groups[rank] = &Group{rank: rank, size: size, backend: b, closed: make(chan struct{})}
	}
	return groups
}

func memLineGroups(size int) []*Group {
	backends := make([]*nativeBackend, size)
	for rank := range backends {
		backends[rank] = &nativeBackend{
			rank:  rank,
			size:  size,
			conns: make([]*rdmaConnGroup, size),
		}
	}
	for i := 0; i+1 < size; i++ {
		ij, ji := newMemPair()
		backends[i].conns[i+1] = &rdmaConnGroup{wires: []*rdmaConn{{t: ij}}}
		backends[i+1].conns[i] = &rdmaConnGroup{wires: []*rdmaConn{{t: ji}}}
	}
	groups := make([]*Group, size)
	for rank, b := range backends {
		groups[rank] = &Group{rank: rank, size: size, backend: b, closed: make(chan struct{})}
	}
	return groups
}

func memMultiWireGroups(size, wires int) []*Group {
	backends := make([]*nativeBackend, size)
	for rank := range backends {
		backends[rank] = &nativeBackend{
			rank:  rank,
			size:  size,
			mesh:  true,
			conns: make([]*rdmaConnGroup, size),
		}
	}
	for i := 0; i < size; i++ {
		for j := i + 1; j < size; j++ {
			ij := &rdmaConnGroup{wires: make([]*rdmaConn, wires)}
			ji := &rdmaConnGroup{wires: make([]*rdmaConn, wires)}
			for wire := 0; wire < wires; wire++ {
				a, b := newMemPair()
				ij.wires[wire] = &rdmaConn{t: a}
				ji.wires[wire] = &rdmaConn{t: b}
			}
			backends[i].conns[j] = ij
			backends[j].conns[i] = ji
		}
	}
	groups := make([]*Group, size)
	for rank, b := range backends {
		groups[rank] = &Group{rank: rank, size: size, backend: b, closed: make(chan struct{})}
	}
	return groups
}

func TestMemPointToPoint(t *testing.T) {
	groups := memGroups(2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := []byte("native jaccl point to point")
	dst := make([]byte, len(src))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- groups[0].Send(ctx, 1, src)
	}()
	go func() {
		defer wg.Done()
		errs <- groups[1].Recv(ctx, 0, dst)
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if string(dst) != string(src) {
		t.Fatalf("recv = %q, want %q", dst, src)
	}
}

func TestMemPointToPointValidatesZeroLengthPeer(t *testing.T) {
	groups := memGroups(2)
	ctx := context.Background()

	if err := groups[0].Send(ctx, 2, nil); err == nil {
		t.Fatal("zero-length send to invalid rank succeeded")
	}
	if err := groups[0].Recv(ctx, 2, nil); err == nil {
		t.Fatal("zero-length recv from invalid rank succeeded")
	}
	if err := groups[0].Send(ctx, 1, nil); err != nil {
		t.Fatalf("zero-length send to valid rank: %v", err)
	}
	if err := groups[0].Recv(ctx, 1, nil); err != nil {
		t.Fatalf("zero-length recv from valid rank: %v", err)
	}
}

func TestMemPointToPointMultiWireLarge(t *testing.T) {
	groups := memMultiWireGroups(2, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := makePattern(2*rdmaStagingBytes + 12345)
	dst := make([]byte, len(src))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- groups[0].Send(ctx, 1, src)
	}()
	go func() {
		defer wg.Done()
		errs <- groups[1].Recv(ctx, 0, dst)
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(dst, src) {
		t.Fatal("large multi-wire receive does not match send")
	}
}

func TestMemCollectives(t *testing.T) {
	groups := memGroups(3)
	runMemCollectives(t, groups)
}

func TestMemAllReduceAllowsAliasedBuffers(t *testing.T) {
	groups := memGroups(2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srcs := [][]byte{{1, 2, 3}, {10, 20, 30}}
	var wg sync.WaitGroup
	errs := make(chan error, len(groups))
	for rank, g := range groups {
		rank, g := rank, g
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := AllSumBytes(ctx, g, srcs[rank], srcs[rank], DTypeUint8); err != nil {
				errs <- fmt.Errorf("rank %d all sum: %w", rank, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := []byte{11, 22, 33}
	for rank, got := range srcs {
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("rank %d sum = %v, want %v", rank, got, want)
		}
	}
}

func TestMemMultiWireCollectives(t *testing.T) {
	groups := memMultiWireGroups(3, 3)
	runMemCollectives(t, groups)
}

func TestMemMultiWireAllGatherLarge(t *testing.T) {
	groups := memMultiWireGroups(3, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srcs := [][]byte{
		makePattern(rdmaStagingBytes + 101),
		makePattern(rdmaStagingBytes + 101),
		makePattern(rdmaStagingBytes + 101),
	}
	for i := range srcs[1] {
		srcs[1][i] ^= 0x55
		srcs[2][i] ^= 0xaa
	}

	gathers := make([][]byte, len(groups))
	var wg sync.WaitGroup
	errs := make(chan error, len(groups))
	for rank, g := range groups {
		rank, g := rank, g
		gathers[rank] = make([]byte, len(groups)*len(srcs[rank]))
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := AllGatherBytes(ctx, g, gathers[rank], srcs[rank]); err != nil {
				errs <- fmt.Errorf("rank %d allgather bytes: %w", rank, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := append(append(append([]byte(nil), srcs[0]...), srcs[1]...), srcs[2]...)
	for rank := range groups {
		if !reflect.DeepEqual(gathers[rank], want) {
			t.Fatalf("rank %d large gather does not match", rank)
		}
	}
}

func TestMemLineCollectives(t *testing.T) {
	groups := memLineGroups(3)
	runMemCollectives(t, groups)
}

func TestMemLineAllGatherLarge(t *testing.T) {
	groups := memLineGroups(3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	n := graphGatherChunkBytes(len(groups)) + 123
	srcs := [][]byte{
		makePattern(n),
		makePattern(n),
		makePattern(n),
	}
	for i := range srcs[1] {
		srcs[1][i] ^= 0x33
		srcs[2][i] ^= 0xcc
	}

	gathers := make([][]byte, len(groups))
	var wg sync.WaitGroup
	errs := make(chan error, len(groups))
	for rank, g := range groups {
		rank, g := rank, g
		gathers[rank] = make([]byte, len(groups)*len(srcs[rank]))
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := AllGatherBytes(ctx, g, gathers[rank], srcs[rank]); err != nil {
				errs <- fmt.Errorf("rank %d allgather bytes: %w", rank, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := append(append(append([]byte(nil), srcs[0]...), srcs[1]...), srcs[2]...)
	for rank := range groups {
		if !reflect.DeepEqual(gathers[rank], want) {
			t.Fatalf("rank %d large line gather does not match", rank)
		}
	}
}

func TestMemLineAllSumBytesLarge(t *testing.T) {
	groups := memLineGroups(3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	n := graphGatherChunkBytes(len(groups)) + 123
	srcs := [][]byte{
		make([]byte, n),
		make([]byte, n),
		make([]byte, n),
	}
	want := make([]byte, n)
	for i := range want {
		srcs[0][i] = byte(i)
		srcs[1][i] = byte(2 * i)
		srcs[2][i] = byte(3 * i)
		want[i] = srcs[0][i] + srcs[1][i] + srcs[2][i]
	}

	sums := make([][]byte, len(groups))
	var wg sync.WaitGroup
	errs := make(chan error, len(groups))
	for rank, g := range groups {
		rank, g := rank, g
		sums[rank] = make([]byte, len(srcs[rank]))
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := AllSumBytes(ctx, g, sums[rank], srcs[rank], DTypeUint8); err != nil {
				errs <- fmt.Errorf("rank %d allsum bytes: %w", rank, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for rank := range groups {
		if !reflect.DeepEqual(sums[rank], want) {
			t.Fatalf("rank %d large line sum does not match", rank)
		}
	}
}

func BenchmarkMemLLMForward(b *testing.B) {
	for _, profile := range []struct {
		name       string
		tokens     int
		hidden     int
		layers     int
		allReduces int
		allGathers int
	}{
		{
			name:       "Decode_7B_TP2",
			tokens:     1,
			hidden:     4096,
			layers:     32,
			allReduces: 2,
			allGathers: 1,
		},
		{
			name:       "Prefill128_7B_TP2",
			tokens:     128,
			hidden:     4096,
			layers:     32,
			allReduces: 2,
			allGathers: 1,
		},
	} {
		b.Run(profile.name, func(b *testing.B) {
			benchmarkMemLLMForward(b, profile.tokens, profile.hidden, profile.layers, profile.allReduces, profile.allGathers)
		})
	}
}

func BenchmarkMemCollectives(b *testing.B) {
	for _, size := range []struct {
		name string
		n    int
	}{
		{"Decode", 4096 * 2},
		{"Prefill128", 128 * 4096 * 2},
	} {
		b.Run("AllSum/"+size.name, func(b *testing.B) {
			benchmarkMemCollective(b, size.n, size.n, func(ctx context.Context, g *Group, out, in []byte) error {
				return AllSumBytes(ctx, g, out, in, DTypeUint8)
			})
		})
		b.Run("AllGather/"+size.name, func(b *testing.B) {
			benchmarkMemCollective(b, size.n, 2*size.n, func(ctx context.Context, g *Group, out, in []byte) error {
				return AllGatherBytes(ctx, g, out, in)
			})
		})
	}
}

func benchmarkMemCollective(b *testing.B, n, outLen int, run func(context.Context, *Group, []byte, []byte) error) {
	groups := memGroups(2)
	inputs := [][]byte{makePattern(n), makePattern(n)}
	for i := range inputs[1] {
		inputs[1][i] ^= 0x55
	}
	outputs := [][]byte{make([]byte, outLen), make([]byte, outLen)}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b.SetBytes(int64(len(groups) * n))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		errs := make(chan error, len(groups))
		for rank, group := range groups {
			rank, group := rank, group
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := run(ctx, group, outputs[rank], inputs[rank]); err != nil {
					errs <- fmt.Errorf("rank %d: %w", rank, err)
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func benchmarkMemLLMForward(b *testing.B, tokens, hidden, layers, allReduces, allGathers int) {
	groups := memGroups(2)
	n := tokens * hidden * 2
	opsPerForward := layers * (allReduces + allGathers)
	inputs := [][]byte{makePattern(n), makePattern(n)}
	for i := range inputs[1] {
		inputs[1][i] ^= 0x55
	}
	sums := [][]byte{make([]byte, n), make([]byte, n)}
	gathers := [][]byte{make([]byte, len(groups)*n), make([]byte, len(groups)*n)}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b.SetBytes(int64(len(groups) * n * opsPerForward))
	b.ReportMetric(float64(len(groups)*opsPerForward), "collectives/op")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		errs := make(chan error, len(groups))
		for rank, group := range groups {
			rank, group := rank, group
			wg.Add(1)
			go func() {
				defer wg.Done()
				for layer := 0; layer < layers; layer++ {
					for j := 0; j < allReduces; j++ {
						if err := AllSumBytes(ctx, group, sums[rank], inputs[rank], DTypeUint8); err != nil {
							errs <- fmt.Errorf("rank %d allsum layer %d: %w", rank, layer, err)
							return
						}
					}
					for j := 0; j < allGathers; j++ {
						if err := AllGatherBytes(ctx, group, gathers[rank], inputs[rank]); err != nil {
							errs <- fmt.Errorf("rank %d allgather layer %d: %w", rank, layer, err)
							return
						}
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func runMemCollectives(t *testing.T, groups []*Group) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srcs := [][]int32{{1, 2}, {10, 20}, {100, 200}}
	rawSrcs := [][]byte{{1, 2}, {10, 20}, {100, 200}}
	halfSrcs := [][]byte{
		float16Bytes(1, 2),
		float16Bytes(10, 20),
		float16Bytes(100, 200),
	}
	gathers := make([][]int32, len(groups))
	rawGathers := make([][]byte, len(groups))
	sums := make([][]int32, len(groups))
	maxes := make([][]int32, len(groups))
	mins := make([][]int32, len(groups))
	rawSums := make([][]byte, len(groups))
	rawMaxes := make([][]byte, len(groups))
	rawMins := make([][]byte, len(groups))
	halfSums := make([][]byte, len(groups))
	var wg sync.WaitGroup
	errs := make(chan error, len(groups)*2)
	for rank, g := range groups {
		rank, g := rank, g
		gathers[rank] = make([]int32, len(groups)*len(srcs[rank]))
		rawGathers[rank] = make([]byte, len(groups)*len(rawSrcs[rank]))
		sums[rank] = make([]int32, len(srcs[rank]))
		maxes[rank] = make([]int32, len(srcs[rank]))
		mins[rank] = make([]int32, len(srcs[rank]))
		rawSums[rank] = make([]byte, len(rawSrcs[rank]))
		rawMaxes[rank] = make([]byte, len(rawSrcs[rank]))
		rawMins[rank] = make([]byte, len(rawSrcs[rank]))
		halfSums[rank] = make([]byte, len(halfSrcs[rank]))
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := AllGather(ctx, g, gathers[rank], srcs[rank]); err != nil {
				errs <- fmt.Errorf("rank %d allgather: %w", rank, err)
				return
			}
			if err := AllGatherBytes(ctx, g, rawGathers[rank], rawSrcs[rank]); err != nil {
				errs <- fmt.Errorf("rank %d allgather bytes: %w", rank, err)
				return
			}
			if err := AllSum(ctx, g, sums[rank], srcs[rank]); err != nil {
				errs <- fmt.Errorf("rank %d allsum: %w", rank, err)
				return
			}
			if err := AllMax(ctx, g, maxes[rank], srcs[rank]); err != nil {
				errs <- fmt.Errorf("rank %d allmax: %w", rank, err)
				return
			}
			if err := AllMin(ctx, g, mins[rank], srcs[rank]); err != nil {
				errs <- fmt.Errorf("rank %d allmin: %w", rank, err)
				return
			}
			if err := AllSumBytes(ctx, g, rawSums[rank], rawSrcs[rank], DTypeUint8); err != nil {
				errs <- fmt.Errorf("rank %d allsum bytes: %w", rank, err)
				return
			}
			if err := AllMaxBytes(ctx, g, rawMaxes[rank], rawSrcs[rank], DTypeUint8); err != nil {
				errs <- fmt.Errorf("rank %d allmax bytes: %w", rank, err)
				return
			}
			if err := AllMinBytes(ctx, g, rawMins[rank], rawSrcs[rank], DTypeUint8); err != nil {
				errs <- fmt.Errorf("rank %d allmin bytes: %w", rank, err)
				return
			}
			if err := AllSumBytes(ctx, g, halfSums[rank], halfSrcs[rank], DTypeFloat16); err != nil {
				errs <- fmt.Errorf("rank %d allsum float16 bytes: %w", rank, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	wantGather := []int32{1, 2, 10, 20, 100, 200}
	wantRawGather := []byte{1, 2, 10, 20, 100, 200}
	wantSum := []int32{111, 222}
	wantMax := []int32{100, 200}
	wantMin := []int32{1, 2}
	wantRawSum := []byte{111, 222}
	wantRawMax := []byte{100, 200}
	wantRawMin := []byte{1, 2}
	wantHalfSum := float16Bytes(111, 222)
	for rank := range groups {
		if !reflect.DeepEqual(gathers[rank], wantGather) {
			t.Fatalf("rank %d gather = %v, want %v", rank, gathers[rank], wantGather)
		}
		if !reflect.DeepEqual(rawGathers[rank], wantRawGather) {
			t.Fatalf("rank %d raw gather = %v, want %v", rank, rawGathers[rank], wantRawGather)
		}
		if !reflect.DeepEqual(sums[rank], wantSum) {
			t.Fatalf("rank %d sum = %v, want %v", rank, sums[rank], wantSum)
		}
		if !reflect.DeepEqual(maxes[rank], wantMax) {
			t.Fatalf("rank %d max = %v, want %v", rank, maxes[rank], wantMax)
		}
		if !reflect.DeepEqual(mins[rank], wantMin) {
			t.Fatalf("rank %d min = %v, want %v", rank, mins[rank], wantMin)
		}
		if !reflect.DeepEqual(rawSums[rank], wantRawSum) {
			t.Fatalf("rank %d raw sum = %v, want %v", rank, rawSums[rank], wantRawSum)
		}
		if !reflect.DeepEqual(rawMaxes[rank], wantRawMax) {
			t.Fatalf("rank %d raw max = %v, want %v", rank, rawMaxes[rank], wantRawMax)
		}
		if !reflect.DeepEqual(rawMins[rank], wantRawMin) {
			t.Fatalf("rank %d raw min = %v, want %v", rank, rawMins[rank], wantRawMin)
		}
		if !reflect.DeepEqual(halfSums[rank], wantHalfSum) {
			t.Fatalf("rank %d half sum = %v, want %v", rank, halfSums[rank], wantHalfSum)
		}
	}
}

func float16Bytes(values ...float32) []byte {
	b := make([]byte, 2*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint16(b[2*i:], float32ToFloat16(v, DTypeFloat16))
	}
	return b
}

func makePattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*131 + i/7)
	}
	return b
}
