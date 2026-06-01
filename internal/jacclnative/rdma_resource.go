package jacclnative

import (
	"fmt"
	"sync"
	"unsafe"
)

type rdmaDevice struct {
	handle uintptr
	name   string
	once   sync.Once
}

type rdmaProtectionDomain struct {
	dev    *rdmaDevice
	handle uintptr
	once   sync.Once
}

type rdmaCompletionQueue struct {
	dev    *rdmaDevice
	handle uintptr
	poller any
	once   sync.Once
}

type rdmaQueuePair struct {
	pd     *rdmaProtectionDomain
	cq     *rdmaCompletionQueue
	handle uintptr
	poster any
	once   sync.Once
}

type rdmaMemoryRegion struct {
	pd     *rdmaProtectionDomain
	handle uintptr
	buf    []byte
	lkey   uint32
	rkey   uint32
	mapped bool
	once   sync.Once
}

type rdmaPostWork struct {
	Offset int
	Length int
	ID     uint64
}

type rdmaWorkRequest struct {
	ID     uint64
	Opcode int
	Bytes  int
	Status int
}

func (d *rdmaDevice) Name() string {
	if d == nil {
		return ""
	}
	return d.name
}

func (q *rdmaQueuePair) Number() uint32 {
	if q == nil {
		return 0
	}
	return q.number()
}

func (m *rdmaMemoryRegion) Buffer() []byte {
	if m == nil {
		return nil
	}
	return m.buf
}

func (m *rdmaMemoryRegion) LKey() uint32 {
	if m == nil {
		return 0
	}
	return m.lkey
}

func (m *rdmaMemoryRegion) RKey() uint32 {
	if m == nil {
		return 0
	}
	return m.rkey
}

func (m *rdmaMemoryRegion) Addr() uint64 {
	if m == nil || len(m.buf) == 0 {
		return 0
	}
	return uint64(uintptr(unsafe.Pointer(&m.buf[0])))
}

func rdmaProviderNilHandleError(op string, dev *rdmaDevice) error {
	if dev != nil && dev.name != "" {
		return fmt.Errorf("%s device=%s: %w", op, dev.name, errRDMAProviderNilHandle)
	}
	return fmt.Errorf("%s: %w", op, errRDMAProviderNilHandle)
}
