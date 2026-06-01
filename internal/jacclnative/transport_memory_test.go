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
}

type memSend struct {
	payload []byte
}

type memTransport struct {
	mu   *sync.Mutex
	cond *sync.Cond

	sendBufBytes []byte
	recvBufBytes []byte
	peer         *memTransport

	pendingRecvs []memRecv
	pendingSends []memSend
	completions  []error
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
	payload := append([]byte(nil), t.sendBufBytes[offset:offset+length]...)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pendingSends = append(t.pendingSends, memSend{payload: payload})
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
	t.pendingRecvs = append(t.pendingRecvs, memRecv{offset: offset, length: length})
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
		if len(s.payload) > r.length {
			err := fmt.Errorf("work completion opcode %d status %d", 128, 1)
			t.completions = append(t.completions, err)
			peer.completions = append(peer.completions, err)
			continue
		}
		copy(peer.recvBufBytes[r.offset:r.offset+len(s.payload)], s.payload)
		t.completions = append(t.completions, nil)
		peer.completions = append(peer.completions, nil)
	}
}

func (t *memTransport) poll(ctx context.Context, n int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for n > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(t.completions) > 0 {
			err := t.completions[0]
			t.completions = t.completions[1:]
			if err != nil {
				return err
			}
			n--
			continue
		}
		waitCond(ctx, t.cond)
	}
	return nil
}

func waitCond(ctx context.Context, cond *sync.Cond) {
	if ctx.Err() != nil {
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

func TestMemCollectives(t *testing.T) {
	groups := memGroups(3)
	runMemCollectives(t, groups)
}

func TestMemLineCollectives(t *testing.T) {
	groups := memLineGroups(3)
	runMemCollectives(t, groups)
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
	rawSums := make([][]byte, len(groups))
	halfSums := make([][]byte, len(groups))
	var wg sync.WaitGroup
	errs := make(chan error, len(groups)*2)
	for rank, g := range groups {
		rank, g := rank, g
		gathers[rank] = make([]int32, len(groups)*len(srcs[rank]))
		rawGathers[rank] = make([]byte, len(groups)*len(rawSrcs[rank]))
		sums[rank] = make([]int32, len(srcs[rank]))
		rawSums[rank] = make([]byte, len(rawSrcs[rank]))
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
			if err := AllSumBytes(ctx, g, rawSums[rank], rawSrcs[rank], DTypeUint8); err != nil {
				errs <- fmt.Errorf("rank %d allsum bytes: %w", rank, err)
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
	wantRawSum := []byte{111, 222}
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
		if !reflect.DeepEqual(rawSums[rank], wantRawSum) {
			t.Fatalf("rank %d raw sum = %v, want %v", rank, rawSums[rank], wantRawSum)
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
