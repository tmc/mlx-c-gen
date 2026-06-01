package jacclnative

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	maxMemoryRegions = 100
	rdmaStagingBytes = 4096 << 7
	pipelineDepth    = 4
)

type nativeBackend struct {
	rank  int
	size  int
	side  *sideChannel
	conns []*rdmaConnGroup
}

type rdmaConnGroup struct {
	wires []*rdmaConn
}

type rdmaConn struct {
	dev    *rdmaDevice
	pd     *rdmaProtectionDomain
	cq     *rdmaCompletionQueue
	qp     *rdmaQueuePair
	sendMR *rdmaMemoryRegion
	recvMR *rdmaMemoryRegion
}

type memoryRegionBudgetError struct {
	required int
	limit    int
}

func (e *memoryRegionBudgetError) Error() string {
	return fmt.Sprintf("memory region budget exceeded: layout needs %d regions, cap is %d", e.required, e.limit)
}

func newNativeBackend(ctx context.Context, cfg Config) (*nativeBackend, error) {
	if !rdmaAvailable() {
		return nil, errRDMAUnavailable
	}
	if err := checkMemoryRegionBudget(cfg); err != nil {
		return nil, err
	}
	size, err := cfg.groupSize()
	if err != nil {
		return nil, err
	}
	b := &nativeBackend{
		rank:  cfg.Rank,
		size:  size,
		conns: make([]*rdmaConnGroup, size),
	}
	if err := b.open(ctx, cfg); err != nil {
		_ = b.close()
		return nil, err
	}
	return b, nil
}

func (b *nativeBackend) open(ctx context.Context, cfg Config) error {
	side, err := newSideChannel(ctx, cfg.Rank, b.size, cfg.Coordinator)
	if err != nil {
		return fmt.Errorf("side channel: %w", err)
	}
	b.side = side

	local, err := b.openLocalConnections(cfg)
	if err != nil {
		return err
	}
	all, err := b.exchangeDestinations(ctx, local)
	if err != nil {
		return err
	}
	for peer, group := range b.conns {
		if group == nil {
			continue
		}
		remote := all[peer][b.rank]
		if len(remote) != len(group.wires) {
			return fmt.Errorf("peer %d: remote advertised %d wires, local opened %d", peer, len(remote), len(group.wires))
		}
		for wire, conn := range group.wires {
			if err := readyToReceiveRDMA(ctx, conn.qp, local[peer][wire], remote[wire]); err != nil {
				return fmt.Errorf("peer %d wire %d: %w", peer, wire, err)
			}
			if err := readyToSendRDMA(ctx, conn.qp, local[peer][wire].PSN); err != nil {
				return fmt.Errorf("peer %d wire %d: %w", peer, wire, err)
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
	return all, nil
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

func devicesForPeer(cfg Config, peer int) []string {
	if cfg.Rank < 0 || cfg.Rank >= len(cfg.Devices) || peer < 0 || peer >= len(cfg.Devices[cfg.Rank]) {
		return nil
	}
	devices := make([]string, 0, len(cfg.Devices[cfg.Rank][peer]))
	for _, dev := range cfg.Devices[cfg.Rank][peer] {
		if strings.TrimSpace(dev) != "" {
			devices = append(devices, dev)
		}
	}
	return devices
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
	conn.pd, err = newRDMAProtectionDomain(conn.dev)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	conn.cq, err = newRDMACompletionQueue(conn.dev, 64)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	conn.qp, err = newRDMAQueuePair(conn.pd, conn.cq)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	size := pipelineDepth * rdmaStagingBytes
	conn.sendMR, err = newRDMAMemoryRegion(conn.pd, size)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	conn.recvMR, err = newRDMAMemoryRegion(conn.pd, size)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	if err = initRDMAQueuePair(conn.qp); err != nil {
		return nil, rdmaDestination{}, err
	}
	dst, err := localRDMADestination(conn.qp)
	if err != nil {
		return nil, rdmaDestination{}, err
	}
	return conn, dst, nil
}

func (b *nativeBackend) close() error {
	if b == nil {
		return nil
	}
	var errs []error
	for _, group := range b.conns {
		if group == nil {
			continue
		}
		for _, conn := range group.wires {
			errs = append(errs, conn.close())
		}
	}
	errs = append(errs, b.side.Close())
	return joinErrors(errs...)
}

func (c *rdmaConn) close() error {
	if c == nil {
		return nil
	}
	return joinErrors(
		c.recvMR.Close(),
		c.sendMR.Close(),
		c.qp.Close(),
		c.cq.Close(),
		c.pd.Close(),
		c.dev.Close(),
	)
}

func joinErrors(errs ...error) error {
	return errors.Join(errs...)
}
