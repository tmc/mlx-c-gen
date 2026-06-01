//go:build darwin && arm64

package jacclnative

import (
	"context"
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

func localRDMADestination(qp *rdmaQueuePair) (rdmaDestination, error) {
	port, gid, gidIndex, err := localPortGID(qp)
	if err != nil {
		return rdmaDestination{}, err
	}
	return rdmaDestination{
		LID:      port.LID,
		QPN:      qp.Number(),
		PSN:      7,
		GIDIndex: gidIndex,
		GID:      [16]byte(gid),
	}, nil
}

func queryRDMAPort(dev *rdmaDevice, maxGIDs int) (rdmaPortInfo, error) {
	if dev == nil || dev.handle == 0 {
		return rdmaPortInfo{}, fmt.Errorf("query rdma port: nil device")
	}
	if maxGIDs <= 0 {
		return rdmaPortInfo{}, fmt.Errorf("query rdma port: max gids %d must be positive", maxGIDs)
	}
	port, gids, selected, err := queryPortGIDs(applerdma.RDMAContext(dev.handle), maxGIDs)
	if err != nil {
		return rdmaPortInfo{}, err
	}
	info := rdmaPortInfo{
		Device:           dev.name,
		PortNum:          1,
		LID:              port.LID,
		GIDTableLength:   int(port.GIDTblLen),
		GIDScanLimit:     maxGIDs,
		SelectedGIDIndex: selected,
		GIDs:             make([]rdmaGIDEntry, 0, len(gids)),
	}
	for _, entry := range gids {
		gid := [16]byte(entry.gid)
		info.GIDs = append(info.GIDs, rdmaGIDEntry{
			Index:      entry.index,
			GID:        gid,
			IPv4Mapped: isIPv4MappedGID(entry.gid),
			Zero:       gid == ([16]byte{}),
		})
	}
	return info, nil
}

func initRDMAQueuePair(qp *rdmaQueuePair) error {
	if qp == nil || qp.handle == 0 {
		return fmt.Errorf("change rdma queue pair to INIT: nil queue pair")
	}
	attr := applerdma.IbvQPAttr{
		QPState:       applerdma.IBV_QPS_INIT,
		PortNum:       1,
		PKeyIndex:     0,
		QPAccessFlags: applerdma.IBV_ACCESS_LOCAL_WRITE | applerdma.IBV_ACCESS_REMOTE_READ | applerdma.IBV_ACCESS_REMOTE_WRITE,
	}
	mask := applerdma.IBV_QP_STATE | applerdma.IBV_QP_PKEY_INDEX | applerdma.IBV_QP_PORT | applerdma.IBV_QP_ACCESS_FLAGS
	return modifyRDMAQueuePair(qp, &attr, mask, "INIT")
}

func readyToReceiveRDMA(ctx context.Context, qp *rdmaQueuePair, local, remote rdmaDestination) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("change rdma queue pair to RTR: %w (%v)", errRDMATransitionNotAttempted, err)
	}
	if qp == nil || qp.handle == 0 {
		return fmt.Errorf("change rdma queue pair to RTR: nil queue pair")
	}
	attr := applerdma.IbvQPAttr{
		QPState:   applerdma.IBV_QPS_RTR,
		PathMTU:   applerdma.IBV_MTU_1024,
		RQPSN:     remote.PSN,
		DestQPNum: remote.QPN,
		AHAttr: applerdma.IbvAHAttr{
			DLID:    remote.LID,
			PortNum: 1,
		},
	}
	if remote.GID != ([16]byte{}) {
		gidIndex := local.GIDIndex
		if gidIndex < 0 || gidIndex > 255 {
			return fmt.Errorf("local gid index %d out of uint8 range", gidIndex)
		}
		attr.AHAttr.IsGlobal = 1
		attr.AHAttr.GRH.HopLimit = 1
		attr.AHAttr.GRH.DGID = applerdma.IbvGID(remote.GID)
		attr.AHAttr.GRH.SGIDIndex = uint8(gidIndex)
	}
	mask := applerdma.IBV_QP_STATE | applerdma.IBV_QP_AV | applerdma.IBV_QP_PATH_MTU | applerdma.IBV_QP_DEST_QPN | applerdma.IBV_QP_RQ_PSN
	if err := modifyRDMAQueuePair(qp, &attr, mask, "RTR"); err != nil {
		return fmt.Errorf("%w: %w", errRDMATransitionFailed, err)
	}
	return nil
}

func readyToSendRDMA(ctx context.Context, qp *rdmaQueuePair, psn uint32) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("change rdma queue pair to RTS: %w (%v)", errRDMATransitionNotAttempted, err)
	}
	if qp == nil || qp.handle == 0 {
		return fmt.Errorf("change rdma queue pair to RTS: nil queue pair")
	}
	attr := applerdma.IbvQPAttr{
		QPState: applerdma.IBV_QPS_RTS,
		SQPSN:   psn,
	}
	mask := applerdma.IBV_QP_STATE | applerdma.IBV_QP_SQ_PSN
	if err := modifyRDMAQueuePair(qp, &attr, mask, "RTS"); err != nil {
		return fmt.Errorf("%w: %w", errRDMATransitionFailed, err)
	}
	return nil
}

func modifyRDMAQueuePair(qp *rdmaQueuePair, attr *applerdma.IbvQPAttr, mask int, state string) error {
	rc, err := applerdma.Ibv_modify_qp(applerdma.RDMAQP(qp.handle), uintptr(unsafe.Pointer(attr)), mask)
	if err != nil {
		return fmt.Errorf("change rdma queue pair to %s: %w mask=0x%x", state, err, mask)
	}
	if rc != 0 {
		return fmt.Errorf("change rdma queue pair to %s: errno %d mask=0x%x", state, rc, mask)
	}
	return nil
}

func localPortGID(qp *rdmaQueuePair) (applerdma.IbvPortAttr, applerdma.IbvGID, int, error) {
	if qp == nil || qp.handle == 0 || qp.pd == nil || qp.pd.dev == nil {
		return applerdma.IbvPortAttr{}, applerdma.IbvGID{}, 0, fmt.Errorf("local rdma destination: nil queue pair")
	}
	port, gids, selected, err := queryPortGIDs(applerdma.RDMAContext(qp.pd.dev.handle), 0)
	if err != nil {
		return applerdma.IbvPortAttr{}, applerdma.IbvGID{}, 0, err
	}
	for _, entry := range gids {
		if entry.index == selected {
			return port, entry.gid, selected, nil
		}
	}
	return port, applerdma.IbvGID{}, selected, fmt.Errorf("local rdma destination: no trusted route gid")
}

type portGIDEntry struct {
	index int
	gid   applerdma.IbvGID
}

func queryPortGIDs(ctx applerdma.RDMAContext, maxGIDs int) (applerdma.IbvPortAttr, []portGIDEntry, int, error) {
	var port applerdma.IbvPortAttr
	if rc, err := applerdma.Ibv_query_port(ctx, 1, uintptr(unsafe.Pointer(&port))); err != nil {
		return applerdma.IbvPortAttr{}, nil, 0, fmt.Errorf("query rdma port: %w", err)
	} else if rc != 0 {
		return applerdma.IbvPortAttr{}, nil, 0, fmt.Errorf("query rdma port: errno %d", rc)
	}

	n := int(port.GIDTblLen)
	if maxGIDs > 0 && maxGIDs < n {
		n = maxGIDs
	}
	var gids []portGIDEntry
	for i := 0; i < n; i++ {
		var candidate applerdma.IbvGID
		rc, err := applerdma.Ibv_query_gid(ctx, 1, i, uintptr(unsafe.Pointer(&candidate)))
		if err != nil || rc != 0 {
			continue
		}
		gids = append(gids, portGIDEntry{index: i, gid: candidate})
	}
	selected := selectPortGID(gids)
	return port, gids, selected, nil
}

func selectPortGID(gids []portGIDEntry) int {
	for _, entry := range gids {
		if isZeroGID(entry.gid) {
			continue
		}
		if entry.index == 0 {
			continue
		}
		if isIPv4MappedGID(entry.gid) {
			return entry.index
		}
	}
	for _, entry := range gids {
		if entry.index == 1 && !isZeroGID(entry.gid) {
			return entry.index
		}
	}
	return -1
}

func isZeroGID(gid applerdma.IbvGID) bool {
	for _, b := range gid {
		if b != 0 {
			return false
		}
	}
	return true
}

func isIPv4MappedGID(gid applerdma.IbvGID) bool {
	for i := 0; i < 10; i++ {
		if gid[i] != 0 {
			return false
		}
	}
	return gid[10] == 0xff && gid[11] == 0xff
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
