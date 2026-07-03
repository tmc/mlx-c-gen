package jacclnative

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// closeBarrierTimeout bounds the teardown barrier so a rank cannot hang
// indefinitely on a peer that has already exited.
const closeBarrierTimeout = 5 * time.Second

const (
	maxMemoryRegions = 100
	rdmaStagingBytes = 4096 << 7
	pipelineDepth    = 4
)

// Work-request kinds encoded into the high bits of a slot work ID so a
// completion can be matched back to the operation that posted it.
const (
	workKindSend = 1
	workKindRecv = 2
)

type nativeBackend struct {
	rank  int
	size  int
	mesh  bool
	side  *sideChannel
	conns []*rdmaConnGroup
}

type rdmaConnGroup struct {
	mu    sync.Mutex
	wires []*rdmaConn
}

type rdmaConn struct {
	dev    *rdmaDevice
	pd     *rdmaProtectionDomain
	cq     *rdmaCompletionQueue
	qp     *rdmaQueuePair
	sendMR *rdmaMemoryRegion
	recvMR *rdmaMemoryRegion
	t      connTransport
}

type connTransport interface {
	sendBuf() []byte
	recvBuf() []byte
	postSend(offset, length int, id uint64) error
	postRecv(offset, length int, id uint64) error
	poll(ctx context.Context, n int) ([]rdmaWorkRequest, error)
	// drain flushes the queue pair to the ERR state and reaps its outstanding
	// completions, then marks the transport poisoned so it is never reused.
	drain() error
}

// errTransportPoisoned reports a transport whose queue pair was left with
// outstanding work requests after a timed-out collective and has been drained.
// The queue pair is in the ERR state and cannot carry further transfers, so
// reusing it would silently stall; callers must rebuild the group instead.
var errTransportPoisoned = errors.New("rdma transport poisoned by a prior timeout; queue pair drained")

type rdmaTransport struct {
	qp       *rdmaQueuePair
	cq       *rdmaCompletionQueue
	sendMR   *rdmaMemoryRegion
	recvMR   *rdmaMemoryRegion
	poisoned bool
}

func (t *rdmaTransport) sendBuf() []byte { return t.sendMR.Buffer() }
func (t *rdmaTransport) recvBuf() []byte { return t.recvMR.Buffer() }

func (t *rdmaTransport) postSend(offset, length int, id uint64) error {
	if t.poisoned {
		return errTransportPoisoned
	}
	return postRDMASend(t.qp, t.sendMR, offset, length, id)
}

func (t *rdmaTransport) postRecv(offset, length int, id uint64) error {
	if t.poisoned {
		return errTransportPoisoned
	}
	return postRDMARecv(t.qp, t.recvMR, offset, length, id)
}

func (t *rdmaTransport) poll(ctx context.Context, n int) ([]rdmaWorkRequest, error) {
	if t.poisoned {
		return nil, errTransportPoisoned
	}
	return pollRDMACompletions(ctx, t.cq, n)
}

// drain flushes the queue pair to ERR, reaps the completions the flush
// produces, and poisons the transport. Apple's Thunderbolt RDMA provider leaves
// a queue pair unusable after a completion times out mid-collective: the next
// post silently stalls (the wedge). Draining reclaims the outstanding work
// requests and poisoning forces a fresh group rather than reusing the dead pair.
func (t *rdmaTransport) drain() error {
	t.poisoned = true
	return drainRDMAQueuePair(t.qp, t.cq)
}

type memoryRegionBudgetError struct {
	required int
	limit    int
}

func (e *memoryRegionBudgetError) Error() string {
	return fmt.Sprintf("memory region budget exceeded: layout needs %d regions, cap is %d", e.required, e.limit)
}

func newNativeBackend(ctx context.Context, cfg Config) (*nativeBackend, error) {
	tracef("backend start rank=%d size=%d coordinator=%s", cfg.Rank, cfg.Size, cfg.Coordinator)
	if !rdmaAvailable() {
		return nil, errRDMAUnavailable
	}
	if err := checkMemoryRegionBudget(cfg); err != nil {
		return nil, err
	}
	if err := checkNoSelfConnections(cfg); err != nil {
		return nil, err
	}
	if err := checkReciprocalConnections(cfg); err != nil {
		return nil, err
	}
	if err := checkGraphConnectivity(cfg); err != nil {
		return nil, err
	}
	if cfg.PreferRing && !cfg.IsValidRing() {
		return nil, fmt.Errorf("preferred ring topology is not valid")
	}
	size, err := cfg.groupSize()
	if err != nil {
		return nil, err
	}
	b := &nativeBackend{
		rank:  cfg.Rank,
		size:  size,
		mesh:  isMesh(cfg),
		conns: make([]*rdmaConnGroup, size),
	}
	if err := b.open(ctx, cfg); err != nil {
		_ = b.close()
		return nil, err
	}
	return b, nil
}

func (b *nativeBackend) open(ctx context.Context, cfg Config) error {
	tracef("rank %d side channel connect", cfg.Rank)
	side, err := newSideChannel(ctx, cfg.Rank, b.size, cfg.Coordinator)
	if err != nil {
		return fmt.Errorf("side channel: %w", err)
	}
	b.side = side
	tracef("rank %d side channel ready", cfg.Rank)

	local, err := b.openLocalConnections(cfg)
	if err != nil {
		return err
	}
	tracef("rank %d local connections open", cfg.Rank)
	all, err := b.exchangeDestinations(ctx, local)
	if err != nil {
		return err
	}
	tracef("rank %d destinations exchanged", cfg.Rank)
	for peer, group := range b.conns {
		if group == nil {
			continue
		}
		remote := all[peer][b.rank]
		if len(remote) != len(group.wires) {
			return fmt.Errorf("peer %d: remote advertised %d wires, local opened %d", peer, len(remote), len(group.wires))
		}
		for wire, conn := range group.wires {
			tracef("rank %d peer %d wire %d RTR", b.rank, peer, wire)
			policy := rdmaRTRPolicy{
				ZeroDLIDWhenGlobal: cfg.ZeroDLIDWhenGlobal,
				GRHHopLimit:        cfg.GRHHopLimit,
			}
			if err := readyToReceiveRDMA(ctx, conn.qp, local[peer][wire], remote[wire], policy); err != nil {
				return fmt.Errorf("peer %d wire %d: %w", peer, wire, err)
			}
			tracef("rank %d peer %d wire %d RTS", b.rank, peer, wire)
			if err := readyToSendRDMA(ctx, conn.qp, local[peer][wire].PSN); err != nil {
				return fmt.Errorf("peer %d wire %d: %w", peer, wire, err)
			}
		}
	}
	if err := b.warmupConnections(ctx); err != nil {
		return fmt.Errorf("warmup: %w", err)
	}
	tracef("rank %d backend ready", b.rank)
	return nil
}

// warmupConnections performs one barrier-synchronized 1-byte exchange on every
// wire before any real collective runs.
//
// jacclnative posts recv and send in the same per-chunk path (see wireExchange)
// with no cross-rank ordering, so on a fresh queue pair a rank can post_send
// before its peer has posted the matching recv. Apple's Thunderbolt provider
// then RNR-NAKs the send and backs off ~100ms, and because confirmCompletions
// keeps the ranks in lockstep the ~100ms phase skew is self-sustaining for the
// rest of the run — the intermittent "wedge" (99.9ms/iter). The provider rejects
// every RNR/retry mask bit (min_rnr_timer, rnr_retry, retry_cnt, timeout all
// EPERM/EINVAL at RTR/RTS), so the skew cannot be tuned away; it must be avoided.
//
// The barrier here guarantees both peers have posted their warm-up recv before
// either posts its send, so the warm-up exchange cannot hit an empty receive
// queue. That phase-locks the ranks once, at ~one round-trip of one-time cost,
// and the lock carries into the real collectives via confirmCompletions.
func (b *nativeBackend) warmupConnections(ctx context.Context) error {
	if b.side == nil {
		return nil
	}
	const warmupID = 1
	for peer, group := range b.conns {
		if group == nil {
			continue
		}
		for wire, conn := range group.wires {
			recvID := slotWorkID(workKindRecv, peer, 0)
			sendID := slotWorkID(workKindSend, peer, 0)
			if err := conn.t.postRecv(0, recvPostLen(warmupID), recvID); err != nil {
				return fmt.Errorf("peer %d wire %d post recv: %w", peer, wire, err)
			}
			// Fence: no send goes out until every rank has posted its recv.
			if err := b.side.Barrier(ctx); err != nil {
				return fmt.Errorf("peer %d wire %d barrier: %w", peer, wire, err)
			}
			conn.t.sendBuf()[0] = 0
			if err := conn.t.postSend(0, warmupID, sendID); err != nil {
				return fmt.Errorf("peer %d wire %d post send: %w", peer, wire, err)
			}
			if err := confirmCompletions(ctx, conn.t,
				expect{id: recvID, bytes: warmupID},
				expect{id: sendID, bytes: warmupID}); err != nil {
				return fmt.Errorf("peer %d wire %d confirm: %w", peer, wire, err)
			}
		}
	}
	return nil
}

func (b *nativeBackend) openLocalConnections(cfg Config) ([][]rdmaDestination, error) {
	local := make([][]rdmaDestination, b.size)
	for peer := 0; peer < b.size; peer++ {
		if peer == b.rank {
			continue
		}
		devices := devicesForPeer(cfg, peer)
		if len(devices) == 0 {
			continue
		}
		group := &rdmaConnGroup{wires: make([]*rdmaConn, len(devices))}
		dsts := make([]rdmaDestination, len(devices))
		for wire, device := range devices {
			tracef("rank %d peer %d wire %d open device=%s", b.rank, peer, wire, device)
			conn, dst, err := openRDMAConn(device)
			if err != nil {
				return nil, fmt.Errorf("peer %d wire %d device %q: %w", peer, wire, device, err)
			}
			group.wires[wire] = conn
			dsts[wire] = dst
		}
		b.conns[peer] = group
		local[peer] = dsts
	}
	return local, nil
}

func (b *nativeBackend) exchangeDestinations(ctx context.Context, local [][]rdmaDestination) ([][][]rdmaDestination, error) {
	payload, err := json.Marshal(local)
	if err != nil {
		return nil, fmt.Errorf("marshal rdma destinations: %w", err)
	}
	allPayloads, err := b.side.AllGather(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("exchange rdma destinations: %w", err)
	}
	all := make([][][]rdmaDestination, b.size)
	for rank, data := range allPayloads {
		if err := json.Unmarshal(data, &all[rank]); err != nil {
			return nil, fmt.Errorf("decode rdma destinations from rank %d: %w", rank, err)
		}
		if len(all[rank]) != b.size {
			return nil, fmt.Errorf("decode rdma destinations from rank %d: got %d peers, want %d", rank, len(all[rank]), b.size)
		}
	}
	if err := validateRDMADestinationMatrix(all, b.size); err != nil {
		return nil, err
	}
	return all, nil
}

func (b *nativeBackend) barrier(ctx context.Context) error {
	return b.side.Barrier(ctx)
}

func (b *nativeBackend) send(ctx context.Context, dst int, src []byte) error {
	group, err := b.conn(dst)
	if err != nil {
		return err
	}
	if len(src) == 0 {
		return nil
	}
	group.mu.Lock()
	defer group.mu.Unlock()

	nWires := len(group.wires)
	var wg sync.WaitGroup
	errs := make([]error, nWires)
	for wire, conn := range group.wires {
		off, length := wireRange(len(src), nWires, wire)
		if length == 0 {
			continue
		}
		wg.Add(1)
		go func(wire int, conn *rdmaConn, sub []byte) {
			defer wg.Done()
			errs[wire] = wireSend(ctx, conn, dst, sub)
		}(wire, conn, src[off:off+length])
	}
	wg.Wait()
	for wire, err := range errs {
		if err != nil {
			return fmt.Errorf("wire %d: %w", wire, err)
		}
	}
	return nil
}

func (b *nativeBackend) recv(ctx context.Context, src int, dst []byte) error {
	group, err := b.conn(src)
	if err != nil {
		return err
	}
	if len(dst) == 0 {
		return nil
	}
	group.mu.Lock()
	defer group.mu.Unlock()

	nWires := len(group.wires)
	var wg sync.WaitGroup
	errs := make([]error, nWires)
	for wire, conn := range group.wires {
		off, length := wireRange(len(dst), nWires, wire)
		if length == 0 {
			continue
		}
		wg.Add(1)
		go func(wire int, conn *rdmaConn, sub []byte) {
			defer wg.Done()
			errs[wire] = wireRecv(ctx, conn, src, sub)
		}(wire, conn, dst[off:off+length])
	}
	wg.Wait()
	for wire, err := range errs {
		if err != nil {
			return fmt.Errorf("wire %d: %w", wire, err)
		}
	}
	return nil
}

func (b *nativeBackend) exchange(ctx context.Context, src []byte) ([][]byte, error) {
	if b.size == 2 {
		return b.exchangeOnePeer(ctx, src)
	}

	recvs := make([][]byte, b.size)
	locked := make([]*rdmaConnGroup, 0, b.size-1)
	defer func() {
		for i := len(locked) - 1; i >= 0; i-- {
			locked[i].mu.Unlock()
		}
	}()
	for peer, group := range b.conns {
		if peer == b.rank || group == nil {
			continue
		}
		group.mu.Lock()
		locked = append(locked, group)
		recvs[peer] = make([]byte, len(src))
	}

	var wg sync.WaitGroup
	errs := make([]error, b.size)
	for peer, group := range b.conns {
		if peer == b.rank || group == nil {
			continue
		}
		wg.Add(1)
		go func(peer int, group *rdmaConnGroup, dst []byte) {
			defer wg.Done()
			errs[peer] = groupExchange(ctx, group, src, func(recvOff int, recv []byte) error {
				copy(dst[recvOff:recvOff+len(recv)], recv)
				return nil
			})
		}(peer, group, recvs[peer])
	}
	wg.Wait()
	for peer, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("peer %d: %w", peer, err)
		}
	}
	return recvs, nil
}

func (b *nativeBackend) exchangeOnePeer(ctx context.Context, src []byte) ([][]byte, error) {
	peer := 1 - b.rank
	group := b.conns[peer]
	if group == nil {
		return make([][]byte, b.size), nil
	}
	dst := make([]byte, len(src))
	group.mu.Lock()
	err := groupExchange(ctx, group, src, func(recvOff int, recv []byte) error {
		copy(dst[recvOff:recvOff+len(recv)], recv)
		return nil
	})
	group.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("peer %d: %w", peer, err)
	}
	recvs := make([][]byte, b.size)
	recvs[peer] = dst
	return recvs, nil
}

func (b *nativeBackend) exchangeOnePeerInto(ctx context.Context, dst, src []byte) error {
	peer := 1 - b.rank
	group := b.conns[peer]
	if group == nil {
		return nil
	}
	group.mu.Lock()
	err := groupExchange(ctx, group, src, func(recvOff int, recv []byte) error {
		copy(dst[recvOff:recvOff+len(recv)], recv)
		return nil
	})
	group.mu.Unlock()
	if err != nil {
		return fmt.Errorf("peer %d: %w", peer, err)
	}
	return nil
}

func (b *nativeBackend) gather(ctx context.Context, src []byte) ([][]byte, error) {
	if b.mesh {
		values, err := b.exchange(ctx, src)
		if err != nil {
			return nil, err
		}
		values[b.rank] = src
		return values, nil
	}
	return b.graphGather(ctx, src)
}

func (b *nativeBackend) graphGather(ctx context.Context, src []byte) ([][]byte, error) {
	maxChunk := graphGatherChunkBytes(b.size)
	if maxChunk <= 0 {
		return nil, fmt.Errorf("graph gather: size %d exceeds staging buffer %d", b.size, rdmaStagingBytes)
	}
	if len(src) <= maxChunk {
		return b.graphGatherChunk(ctx, src)
	}
	values := make([][]byte, b.size)
	for off := 0; off < len(src); off += maxChunk {
		n := minInt(maxChunk, len(src)-off)
		chunk, err := b.graphGatherChunk(ctx, src[off:off+n])
		if err != nil {
			return nil, err
		}
		for rank := range values {
			values[rank] = append(values[rank], chunk[rank]...)
		}
	}
	return values, nil
}

func (b *nativeBackend) graphGatherChunk(ctx context.Context, src []byte) ([][]byte, error) {
	payloadSize := b.size + b.size*len(src)
	if payloadSize > rdmaStagingBytes {
		return nil, fmt.Errorf("graph gather payload %d exceeds staging buffer %d", payloadSize, rdmaStagingBytes)
	}
	values := make([][]byte, b.size)
	known := make([]bool, b.size)
	values[b.rank] = append([]byte(nil), src...)
	known[b.rank] = true
	neighbors := b.neighbors()
	locked := make([]*rdmaConnGroup, 0, len(neighbors))
	defer func() {
		for i := len(locked) - 1; i >= 0; i-- {
			locked[i].mu.Unlock()
		}
	}()
	for _, peer := range neighbors {
		group := b.conns[peer]
		group.mu.Lock()
		locked = append(locked, group)
	}

	send := make([]byte, payloadSize)
	for step := 0; step < b.size-1; step++ {
		encodeGraphGatherPayload(send, known, values, len(src))
		for _, peer := range neighbors {
			conn := b.conns[peer].wires[0]
			if err := conn.t.postRecv(0, recvPostLen(len(send)), slotWorkID(workKindRecv, peer, 0)); err != nil {
				return nil, fmt.Errorf("rank %d post recv: %w", peer, err)
			}
		}
		for _, peer := range neighbors {
			conn := b.conns[peer].wires[0]
			copy(conn.t.sendBuf()[:len(send)], send)
			if err := conn.t.postSend(0, len(send), slotWorkID(workKindSend, peer, 0)); err != nil {
				return nil, fmt.Errorf("rank %d post send: %w", peer, err)
			}
		}
		for _, peer := range neighbors {
			conn := b.conns[peer].wires[0]
			recvID := slotWorkID(workKindRecv, peer, 0)
			sendID := slotWorkID(workKindSend, peer, 0)
			if err := confirmCompletions(ctx, conn.t, expect{id: recvID, bytes: len(send)}, expect{id: sendID, bytes: len(send)}); err != nil {
				return nil, fmt.Errorf("rank %d poll: %w", peer, err)
			}
			if err := mergeGraphGatherPayload(known, values, conn.t.recvBuf()[:len(send)], len(src)); err != nil {
				return nil, fmt.Errorf("rank %d payload: %w", peer, err)
			}
		}
	}
	if b.side != nil {
		if err := b.side.Barrier(ctx); err != nil {
			return nil, fmt.Errorf("graph gather barrier: %w", err)
		}
	}
	for rank, ok := range known {
		if !ok {
			return nil, fmt.Errorf("rank %d value missing after graph gather", rank)
		}
	}
	return values, nil
}

func graphGatherChunkBytes(size int) int {
	if size <= 0 || size >= rdmaStagingBytes {
		return 0
	}
	return (rdmaStagingBytes - size) / size
}

func (b *nativeBackend) neighbors() []int {
	neighbors := make([]int, 0, b.size)
	for peer, group := range b.conns {
		if peer != b.rank && group != nil {
			neighbors = append(neighbors, peer)
		}
	}
	return neighbors
}

func encodeGraphGatherPayload(payload []byte, known []bool, values [][]byte, elemLen int) {
	clear(payload)
	for rank, ok := range known {
		if !ok {
			continue
		}
		payload[rank] = 1
		copy(payload[len(known)+rank*elemLen:len(known)+(rank+1)*elemLen], values[rank])
	}
}

func mergeGraphGatherPayload(known []bool, values [][]byte, payload []byte, elemLen int) error {
	n := len(known)
	if len(payload) != n+n*elemLen {
		return fmt.Errorf("length %d, want %d", len(payload), n+n*elemLen)
	}
	for rank := range known {
		switch payload[rank] {
		case 0:
			continue
		case 1:
		default:
			return fmt.Errorf("rank %d flag %d is invalid", rank, payload[rank])
		}
		if known[rank] {
			continue
		}
		values[rank] = append([]byte(nil), payload[n+rank*elemLen:n+(rank+1)*elemLen]...)
		known[rank] = true
	}
	return nil
}

func groupExchange(ctx context.Context, group *rdmaConnGroup, src []byte, onRecv func(int, []byte) error) error {
	nWires := len(group.wires)
	if nWires == 1 {
		return wireExchange(ctx, group.wires[0], src, 0, onRecv)
	}

	var wg sync.WaitGroup
	errs := make([]error, nWires)
	for wire, conn := range group.wires {
		off, length := wireRange(len(src), nWires, wire)
		if length == 0 {
			continue
		}
		wg.Add(1)
		go func(wire int, conn *rdmaConn, off, length int) {
			defer wg.Done()
			errs[wire] = wireExchange(ctx, conn, src[off:off+length], off, onRecv)
		}(wire, conn, off, length)
	}
	wg.Wait()
	for wire, err := range errs {
		if err != nil {
			return fmt.Errorf("wire %d: %w", wire, err)
		}
	}
	return nil
}

func wireExchange(ctx context.Context, conn *rdmaConn, src []byte, base int, onRecv func(int, []byte) error) error {
	chunks := chunkCount(len(src))
	for chunk := 0; chunk < chunks; chunk++ {
		off := chunk * rdmaStagingBytes
		n := minInt(rdmaStagingBytes, len(src)-off)
		slot := chunk % pipelineDepth
		so := slot * rdmaStagingBytes
		recvID := slotWorkID(workKindRecv, 0, slot)
		sendID := slotWorkID(workKindSend, 0, slot)
		if err := conn.t.postRecv(so, recvPostLen(n), recvID); err != nil {
			return err
		}
		copy(conn.t.sendBuf()[so:so+n], src[off:off+n])
		if err := conn.t.postSend(so, n, sendID); err != nil {
			return err
		}
		if err := confirmCompletions(ctx, conn.t, expect{id: recvID, bytes: n}, expect{id: sendID, bytes: n}); err != nil {
			return err
		}
		if err := onRecv(base+off, conn.t.recvBuf()[so:so+n]); err != nil {
			return err
		}
	}
	return nil
}

func wireSend(ctx context.Context, conn *rdmaConn, peer int, src []byte) error {
	chunks := chunkCount(len(src))
	chunkLen := func(chunk int) int {
		return minInt(rdmaStagingBytes, len(src)-chunk*rdmaStagingBytes)
	}
	stage := func(chunk int) error {
		off := chunk * rdmaStagingBytes
		n := chunkLen(chunk)
		so := (chunk % pipelineDepth) * rdmaStagingBytes
		copy(conn.t.sendBuf()[so:so+n], src[off:off+n])
		return conn.t.postSend(so, n, slotWorkID(workKindSend, peer, chunk%pipelineDepth))
	}
	var outstanding []int
	next := 0
	for ; next < chunks && len(outstanding) < pipelineDepth; next++ {
		if err := stage(next); err != nil {
			return err
		}
		outstanding = append(outstanding, next)
	}
	for len(outstanding) > 0 {
		chunk := outstanding[0]
		outstanding = outstanding[1:]
		id := slotWorkID(workKindSend, peer, chunk%pipelineDepth)
		if err := confirmCompletions(ctx, conn.t, expect{id: id, bytes: chunkLen(chunk)}); err != nil {
			return err
		}
		if next < chunks {
			if err := stage(next); err != nil {
				return err
			}
			outstanding = append(outstanding, next)
			next++
		}
	}
	return nil
}

func wireRecv(ctx context.Context, conn *rdmaConn, peer int, dst []byte) error {
	chunks := chunkCount(len(dst))
	chunkLen := func(chunk int) int {
		return minInt(rdmaStagingBytes, len(dst)-chunk*rdmaStagingBytes)
	}
	post := func(chunk int) error {
		ro := (chunk % pipelineDepth) * rdmaStagingBytes
		return conn.t.postRecv(ro, recvPostLen(chunkLen(chunk)), slotWorkID(workKindRecv, peer, chunk%pipelineDepth))
	}
	var outstanding []int
	next := 0
	for ; next < chunks && len(outstanding) < pipelineDepth; next++ {
		if err := post(next); err != nil {
			return err
		}
		outstanding = append(outstanding, next)
	}
	for len(outstanding) > 0 {
		chunk := outstanding[0]
		outstanding = outstanding[1:]
		n := chunkLen(chunk)
		id := slotWorkID(workKindRecv, peer, chunk%pipelineDepth)
		if err := confirmCompletions(ctx, conn.t, expect{id: id, bytes: n}); err != nil {
			return err
		}
		off := chunk * rdmaStagingBytes
		ro := (chunk % pipelineDepth) * rdmaStagingBytes
		copy(dst[off:off+n], conn.t.recvBuf()[ro:ro+n])
		if next < chunks {
			if err := post(next); err != nil {
				return err
			}
			outstanding = append(outstanding, next)
			next++
		}
	}
	return nil
}

func chunkCount(n int) int {
	return (n + rdmaStagingBytes - 1) / rdmaStagingBytes
}

func recvPostLen(chunkLen int) int {
	return chunkLen
}

func wireRange(total, nWires, wire int) (offset, length int) {
	bytesPerWire := (total + nWires - 1) / nWires
	offset = wire * bytesPerWire
	if offset >= total {
		return total, 0
	}
	length = bytesPerWire
	if offset+length > total {
		length = total - offset
	}
	return offset, length
}

func slotWorkID(kind, peer, slot int) uint64 {
	return uint64(kind)<<32 | uint64(slot)<<16 | uint64(peer)
}

func workKind(id uint64) int {
	return int(id >> 32)
}

// expect describes a work request the caller posted and is waiting on. bytes
// is the posted length; for receives it is the number of bytes the peer is
// expected to deliver and is checked against the completion's reported length.
type expect struct {
	id    uint64
	bytes int
}

// confirmCompletions polls the transport until every expected work ID has
// completed and verifies that each receive delivered exactly the bytes that
// were posted. A completion whose ID was not expected, or a receive whose
// reported length differs from the posted length, is a framing error: the
// completion queue is shared across slots, so consuming a stale or short
// completion would let a caller read an undrained or partially written slot.
//
// want holds one or two entries on every pipelined chunk, so it is matched
// with a linear scan rather than a map to keep this hot path allocation-free.
func confirmCompletions(ctx context.Context, t connTransport, want ...expect) error {
	wrs, err := t.poll(ctx, len(want))
	if err != nil {
		// A context timeout or cancellation leaves the posted work requests
		// outstanding on the queue pair. Reusing that pair on the next
		// collective silently stalls (the Apple Thunderbolt "wedge"), so drain
		// it to the ERR state and poison the transport; the caller surfaces the
		// original error and the group must be rebuilt before further use.
		if ctx.Err() != nil {
			if derr := t.drain(); derr != nil {
				return fmt.Errorf("%w (drain after timeout: %v)", err, derr)
			}
		}
		return err
	}
	matched := 0
	for _, wr := range wrs {
		i := indexExpect(want, wr.ID)
		if i < 0 {
			return fmt.Errorf("unexpected rdma completion id %d opcode %d bytes %d", wr.ID, wr.Opcode, wr.Bytes)
		}
		if workKind(wr.ID) == workKindRecv && wr.Bytes != want[i].bytes {
			return fmt.Errorf("rdma recv id %d delivered %d bytes, posted %d", wr.ID, wr.Bytes, want[i].bytes)
		}
		// Clear the matched entry so a duplicate completion of the same id
		// is reported as unexpected rather than silently double-counted.
		want[i].id = 0
		matched++
	}
	if matched != len(want) {
		return fmt.Errorf("rdma poll returned %d completions but %d work ids did not complete", len(wrs), len(want)-matched)
	}
	return nil
}

// indexExpect returns the index of the first entry in want with the given id,
// or -1 if none matches. A zeroed id never matches; confirmCompletions zeroes
// entries as it consumes them.
func indexExpect(want []expect, id uint64) int {
	if id == 0 {
		return -1
	}
	for i := range want {
		if want[i].id == id {
			return i
		}
	}
	return -1
}

func minInt(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func pollRDMACompletions(ctx context.Context, cq *rdmaCompletionQueue, n int) ([]rdmaWorkRequest, error) {
	if n < 0 {
		return nil, fmt.Errorf("poll rdma completions: count %d is negative", n)
	}
	done := make([]rdmaWorkRequest, 0, n)
	for len(done) < n {
		wrs, err := pollRDMACompletion(ctx, cq)
		if err != nil {
			return nil, err
		}
		done = append(done, wrs...)
	}
	return done, nil
}

func (b *nativeBackend) conn(peer int) (*rdmaConnGroup, error) {
	if peer < 0 || peer >= b.size {
		return nil, fmt.Errorf("rank %d out of range for size %d", peer, b.size)
	}
	group := b.conns[peer]
	if group == nil {
		return nil, fmt.Errorf("rank %d has no RDMA connection", peer)
	}
	return group, nil
}

func requiredMemoryRegions(cfg Config) int {
	regions := 0
	for peer := range cfg.Devices {
		if peer == cfg.Rank {
			continue
		}
		regions += 2 * len(devicesForPeer(cfg, peer))
	}
	return regions
}

func checkMemoryRegionBudget(cfg Config) error {
	if n := requiredMemoryRegions(cfg); n > maxMemoryRegions {
		return &memoryRegionBudgetError{required: n, limit: maxMemoryRegions}
	}
	return nil
}

func checkNoSelfConnections(cfg Config) error {
	size, err := cfg.groupSize()
	if err != nil {
		return err
	}
	for rank := 0; rank < size; rank++ {
		if n := len(devicesForRankPeer(cfg, rank, rank)); n != 0 {
			return fmt.Errorf("rank %d has %d self RDMA wires", rank, n)
		}
	}
	return nil
}

func checkReciprocalConnections(cfg Config) error {
	size, err := cfg.groupSize()
	if err != nil {
		return err
	}
	for rank := 0; rank < size; rank++ {
		for peer := rank + 1; peer < size; peer++ {
			forward := len(devicesForRankPeer(cfg, rank, peer))
			backward := len(devicesForRankPeer(cfg, peer, rank))
			if forward != backward {
				return fmt.Errorf("rank %d has %d RDMA wires to rank %d, rank %d has %d back", rank, forward, peer, peer, backward)
			}
		}
	}
	return nil
}

func checkGraphConnectivity(cfg Config) error {
	size, err := cfg.groupSize()
	if err != nil {
		return err
	}
	seen := make([]bool, size)
	queue := []int{cfg.Rank}
	seen[cfg.Rank] = true
	for len(queue) > 0 {
		rank := queue[0]
		queue = queue[1:]
		for peer := 0; peer < size; peer++ {
			if peer == rank || len(devicesForRankPeer(cfg, rank, peer)) == 0 || seen[peer] {
				continue
			}
			seen[peer] = true
			queue = append(queue, peer)
		}
	}
	for rank, ok := range seen {
		if !ok {
			return fmt.Errorf("rank %d cannot reach rank %d through RDMA graph", cfg.Rank, rank)
		}
	}
	return nil
}

func isMesh(cfg Config) bool {
	size, err := cfg.groupSize()
	if err != nil {
		return false
	}
	for rank := 0; rank < size; rank++ {
		for peer := 0; peer < size; peer++ {
			if peer == rank {
				if len(devicesForRankPeer(cfg, rank, peer)) != 0 {
					return false
				}
				continue
			}
			if len(devicesForRankPeer(cfg, rank, peer)) == 0 {
				return false
			}
		}
	}
	return true
}

func isRing(cfg Config) bool {
	size, err := cfg.groupSize()
	if err != nil {
		return false
	}
	if size < 2 {
		return false
	}
	nWires := len(devicesForRankPeer(cfg, 0, 1))
	for rank := 0; rank < size; rank++ {
		prev := (rank + size - 1) % size
		next := (rank + 1) % size
		if len(devicesForRankPeer(cfg, rank, prev)) != nWires {
			return false
		}
		if prev != next && len(devicesForRankPeer(cfg, rank, next)) != nWires {
			return false
		}
	}
	return true
}

func devicesForRankPeer(cfg Config, rank, peer int) []string {
	if rank < 0 || rank >= len(cfg.Devices) || peer < 0 || peer >= len(cfg.Devices[rank]) {
		return nil
	}
	devices := make([]string, 0, len(cfg.Devices[rank][peer]))
	for _, dev := range cfg.Devices[rank][peer] {
		if strings.TrimSpace(dev) != "" {
			devices = append(devices, dev)
		}
	}
	return devices
}

func devicesForPeer(cfg Config, peer int) []string {
	return devicesForRankPeer(cfg, cfg.Rank, peer)
}

func checkMeshConnectivity(cfg Config) error {
	size, err := cfg.groupSize()
	if err != nil {
		return err
	}
	for peer := 0; peer < size; peer++ {
		if peer == cfg.Rank {
			continue
		}
		if len(devicesForPeer(cfg, peer)) == 0 {
			return fmt.Errorf("rank %d has no RDMA connection to rank %d", cfg.Rank, peer)
		}
	}
	return nil
}

func openRDMAConn(device string) (*rdmaConn, rdmaDestination, error) {
	conn := new(rdmaConn)
	var err error
	defer func() {
		if err != nil {
			_ = conn.close()
		}
	}()
	conn.dev, err = openRDMADevice(device)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	tracef("device %s open", device)
	conn.pd, err = newRDMAProtectionDomain(conn.dev)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	tracef("device %s protection domain", device)
	conn.cq, err = newRDMACompletionQueue(conn.dev, 64)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	tracef("device %s completion queue", device)
	conn.qp, err = newRDMAQueuePair(conn.pd, conn.cq)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	tracef("device %s queue pair qpn=%d", device, conn.qp.Number())
	size := pipelineDepth * rdmaStagingBytes
	conn.sendMR, err = newRDMAMemoryRegion(conn.pd, size)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	tracef("device %s send memory region bytes=%d", device, size)
	conn.recvMR, err = newRDMAMemoryRegion(conn.pd, size)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	tracef("device %s recv memory region bytes=%d", device, size)
	if err = initRDMAQueuePair(conn.qp); err != nil {
		return nil, rdmaDestination{}, err
	}
	tracef("device %s queue pair INIT", device)
	conn.t = &rdmaTransport{qp: conn.qp, cq: conn.cq, sendMR: conn.sendMR, recvMR: conn.recvMR}
	dst, err := localRDMADestination(conn.qp)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	tracef("device %s local destination lid=%d qpn=%d gid_index=%d", device, dst.LID, dst.QPN, dst.GIDIndex)
	return conn, dst, nil
}

func (b *nativeBackend) close() error {
	if b == nil {
		return nil
	}
	var errs []error
	// Wait for every rank to reach teardown before deregistering memory
	// regions or destroying queue pairs. Otherwise a peer that is still
	// transmitting would write into a region this rank has already freed.
	// The barrier is bounded so a rank that exited early cannot block the
	// rest of the group from closing.
	if b.side != nil {
		ctx, cancel := context.WithTimeout(context.Background(), closeBarrierTimeout)
		if err := b.side.Barrier(ctx); err != nil {
			errs = append(errs, fmt.Errorf("teardown barrier: %w", err))
		}
		cancel()
	}
	for _, group := range b.conns {
		if group == nil {
			continue
		}
		for _, conn := range group.wires {
			if err := conn.close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if b.side != nil {
		if err := b.side.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors(errs...)
}

func (c *rdmaConn) close() error {
	if c == nil {
		return nil
	}
	// Flush the queue pair and reap its completions before deregistering the
	// memory regions it referenced. The group already barriers on teardown, so
	// every peer has stopped transmitting; this drains any completion still in
	// the local queue. Drain errors are non-fatal: the resources are torn down
	// regardless.
	var errs []error
	if err := drainRDMAQueuePair(c.qp, c.cq); err != nil && !errors.Is(err, errRDMAUnavailable) {
		errs = append(errs, fmt.Errorf("drain queue pair: %w", err))
	}
	// Closed in reverse construction order: regions, then queue pair and
	// completion queue, then the protection domain and device that owned them.
	for _, closer := range []io.Closer{c.recvMR, c.sendMR, c.qp, c.cq, c.pd, c.dev} {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors(errs...)
}

func joinErrors(errs ...error) error {
	return errors.Join(errs...)
}
