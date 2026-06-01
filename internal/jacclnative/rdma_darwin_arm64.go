//go:build darwin && arm64

package jacclnative

import (
	"fmt"
	"syscall"
	"unsafe"

	applerdma "github.com/tmc/apple/rdma"
)

func rdmaAvailable() bool {
	return applerdma.Available()
}

func rdmaDeviceNames() ([]string, error) {
	if !rdmaAvailable() {
		return nil, errRDMAUnavailable
	}
	devices, err := applerdma.Devices()
	if err != nil {
		return nil, fmt.Errorf("list rdma devices: %w", err)
	}
	names := make([]string, 0, len(devices))
	for _, dev := range devices {
		names = append(names, dev.Name)
	}
	return names, nil
}

func openRDMADevice(name string) (*rdmaDevice, error) {
	if !rdmaAvailable() {
		return nil, errRDMAUnavailable
	}
	devices, err := applerdma.Devices()
	if err != nil {
		return nil, fmt.Errorf("list rdma devices: %w", err)
	}
	for _, dev := range devices {
		if name != "" && dev.Name != name {
			continue
		}
		ctx, err := applerdma.Ibv_open_device(dev.Handle)
		if err != nil {
			return nil, fmt.Errorf("open rdma device %q: %w", dev.Name, err)
		}
		if ctx == 0 {
			return nil, fmt.Errorf("open rdma device %q: %w", dev.Name, errRDMAProviderNilHandle)
		}
		return &rdmaDevice{handle: uintptr(ctx), name: dev.Name}, nil
	}
	if name == "" {
		return nil, fmt.Errorf("open rdma device: no devices")
	}
	return nil, fmt.Errorf("open rdma device %q: not found", name)
}

func (d *rdmaDevice) Close() error {
	if d == nil || d.handle == 0 {
		return nil
	}
	var err error
	d.once.Do(func() {
		rc, e := applerdma.Ibv_close_device(applerdma.RDMAContext(d.handle))
		if e != nil {
			err = fmt.Errorf("close rdma device %q: %w", d.name, e)
			return
		}
		if rc != 0 {
			err = fmt.Errorf("close rdma device %q: errno %d", d.name, rc)
			return
		}
		d.handle = 0
	})
	return err
}

func newRDMAProtectionDomain(dev *rdmaDevice) (*rdmaProtectionDomain, error) {
	if dev == nil || dev.handle == 0 {
		return nil, fmt.Errorf("alloc rdma protection domain: nil device")
	}
	pd, err := applerdma.Ibv_alloc_pd(applerdma.RDMAContext(dev.handle))
	if err != nil {
		return nil, fmt.Errorf("alloc rdma protection domain: %w", err)
	}
	if pd == 0 {
		return nil, rdmaProviderNilHandleError("alloc rdma protection domain", dev)
	}
	return &rdmaProtectionDomain{dev: dev, handle: uintptr(pd)}, nil
}

func (p *rdmaProtectionDomain) Close() error {
	if p == nil || p.handle == 0 {
		return nil
	}
	var err error
	p.once.Do(func() {
		rc, e := applerdma.Ibv_dealloc_pd(applerdma.RDMAPD(p.handle))
		if e != nil {
			err = fmt.Errorf("dealloc rdma protection domain: %w", e)
			return
		}
		if rc != 0 {
			err = fmt.Errorf("dealloc rdma protection domain: errno %d", rc)
			return
		}
		p.handle = 0
	})
	return err
}

func newRDMACompletionQueue(dev *rdmaDevice, capacity int) (*rdmaCompletionQueue, error) {
	if dev == nil || dev.handle == 0 {
		return nil, fmt.Errorf("create rdma completion queue: nil device")
	}
	if capacity <= 0 {
		return nil, fmt.Errorf("create rdma completion queue: capacity %d must be positive", capacity)
	}
	cq, err := applerdma.Ibv_create_cq(applerdma.RDMAContext(dev.handle), capacity, 0, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("create rdma completion queue: %w", err)
	}
	if cq == 0 {
		return nil, rdmaProviderNilHandleError("create rdma completion queue", dev)
	}
	poller, err := applerdma.NewIbvCQPoller(cq)
	if err != nil {
		_, _ = applerdma.Ibv_destroy_cq(cq)
		return nil, fmt.Errorf("create rdma completion queue poller: %w", err)
	}
	return &rdmaCompletionQueue{dev: dev, handle: uintptr(cq), poller: poller}, nil
}

func (c *rdmaCompletionQueue) Close() error {
	if c == nil || c.handle == 0 {
		return nil
	}
	var err error
	c.once.Do(func() {
		rc, e := applerdma.Ibv_destroy_cq(applerdma.RDMACQ(c.handle))
		if e != nil {
			err = fmt.Errorf("destroy rdma completion queue: %w", e)
			return
		}
		if rc != 0 {
			err = fmt.Errorf("destroy rdma completion queue: errno %d", rc)
			return
		}
		c.handle = 0
	})
	return err
}

func newRDMAQueuePair(pd *rdmaProtectionDomain, cq *rdmaCompletionQueue) (*rdmaQueuePair, error) {
	if pd == nil || pd.handle == 0 {
		return nil, fmt.Errorf("create rdma queue pair: nil protection domain")
	}
	if cq == nil || cq.handle == 0 {
		return nil, fmt.Errorf("create rdma queue pair: nil completion queue")
	}
	attr := applerdma.IbvQPInitAttr{
		SendCQ: applerdma.RDMACQ(cq.handle),
		RecvCQ: applerdma.RDMACQ(cq.handle),
		Cap: applerdma.IbvQPCap{
			MaxSendWR:  64,
			MaxRecvWR:  64,
			MaxSendSGE: 1,
			MaxRecvSGE: 1,
		},
		QPType:   applerdma.IBV_QPT_UC,
		SQSigAll: 1,
	}
	qp, err := applerdma.Ibv_create_qp(applerdma.RDMAPD(pd.handle), uintptr(unsafe.Pointer(&attr)))
	if err != nil {
		return nil, fmt.Errorf("create rdma queue pair: %w", err)
	}
	if qp == 0 {
		return nil, rdmaProviderNilHandleError("create rdma queue pair", pd.dev)
	}
	poster, err := applerdma.NewIbvQPPoster(qp)
	if err != nil {
		_, _ = applerdma.Ibv_destroy_qp(qp)
		return nil, fmt.Errorf("create rdma queue pair poster: %w", err)
	}
	return &rdmaQueuePair{pd: pd, cq: cq, handle: uintptr(qp), poster: poster}, nil
}

func (q *rdmaQueuePair) Close() error {
	if q == nil || q.handle == 0 {
		return nil
	}
	var err error
	q.once.Do(func() {
		rc, e := applerdma.Ibv_destroy_qp(applerdma.RDMAQP(q.handle))
		if e != nil {
			err = fmt.Errorf("destroy rdma queue pair: %w", e)
			return
		}
		if rc != 0 {
			err = fmt.Errorf("destroy rdma queue pair: errno %d", rc)
			return
		}
		q.handle = 0
	})
	return err
}

func (q *rdmaQueuePair) number() uint32 {
	if q == nil || q.handle == 0 {
		return 0
	}
	return applerdma.Ibv_qp_num(applerdma.RDMAQP(q.handle))
}

func newRDMAMemoryRegion(pd *rdmaProtectionDomain, size int) (*rdmaMemoryRegion, error) {
	if pd == nil || pd.handle == 0 {
		return nil, fmt.Errorf("alloc rdma memory region: nil protection domain")
	}
	if size <= 0 {
		return nil, fmt.Errorf("alloc rdma memory region: size %d must be positive", size)
	}
	buf, err := syscall.Mmap(-1, 0, roundRDMAPage(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		return nil, fmt.Errorf("alloc rdma memory region: mmap: %w", err)
	}
	mr, err := registerMappedRDMAMemory(pd, buf)
	if err != nil {
		_ = syscall.Munmap(buf)
		return nil, err
	}
	mr.mapped = true
	return mr, nil
}

func registerMappedRDMAMemory(pd *rdmaProtectionDomain, buf []byte) (*rdmaMemoryRegion, error) {
	mr, err := applerdma.Ibv_reg_mr(applerdma.RDMAPD(pd.handle), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), applerdma.IBV_ACCESS_LOCAL_WRITE|applerdma.IBV_ACCESS_REMOTE_WRITE|applerdma.IBV_ACCESS_REMOTE_READ)
	if err != nil {
		return nil, fmt.Errorf("register rdma memory: %w", err)
	}
	if mr == 0 {
		return nil, rdmaProviderNilHandleError("register rdma memory", pd.dev)
	}
	return &rdmaMemoryRegion{
		pd:     pd,
		handle: uintptr(mr),
		buf:    buf,
		lkey:   applerdma.Ibv_mr_lkey(mr),
		rkey:   applerdma.Ibv_mr_rkey(mr),
	}, nil
}

func (m *rdmaMemoryRegion) Close() error {
	if m == nil || m.handle == 0 {
		return nil
	}
	var err error
	m.once.Do(func() {
		rc, e := applerdma.Ibv_dereg_mr(applerdma.RDMAMR(m.handle))
		if e != nil {
			err = fmt.Errorf("dereg rdma memory: %w", e)
			return
		}
		if rc != 0 {
			err = fmt.Errorf("dereg rdma memory: errno %d", rc)
			return
		}
		if m.mapped {
			if e := syscall.Munmap(m.buf); e != nil {
				err = fmt.Errorf("unmap rdma memory: %w", e)
				return
			}
		}
		m.handle = 0
		m.buf = nil
	})
	return err
}

func roundRDMAPage(n int) int {
	const page = 16 * 1024
	if n%page == 0 {
		return n
	}
	return n + page - n%page
}
