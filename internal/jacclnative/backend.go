package jacclnative

import (
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

func newNativeBackend(cfg Config) (*nativeBackend, error) {
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
	return &nativeBackend{
		rank:  cfg.Rank,
		size:  size,
		conns: make([]*rdmaConnGroup, size),
	}, nil
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

func openRDMAConn(device string) (*rdmaConn, error) {
	conn := new(rdmaConn)
	var err error
	defer func() {
		if err != nil {
			_ = conn.close()
		}
	}()
	conn.dev, err = openRDMADevice(device)
	if err != nil {
		return nil, err
	}
	conn.pd, err = newRDMAProtectionDomain(conn.dev)
	if err != nil {
		return nil, err
	}
	conn.cq, err = newRDMACompletionQueue(conn.dev, 64)
	if err != nil {
		return nil, err
	}
	conn.qp, err = newRDMAQueuePair(conn.pd, conn.cq)
	if err != nil {
		return nil, err
	}
	size := pipelineDepth * rdmaStagingBytes
	conn.sendMR, err = newRDMAMemoryRegion(conn.pd, size)
	if err != nil {
		return nil, err
	}
	conn.recvMR, err = newRDMAMemoryRegion(conn.pd, size)
	if err != nil {
		return nil, err
	}
	return conn, nil
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
