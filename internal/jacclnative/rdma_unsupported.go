//go:build !darwin || !arm64

package jacclnative

import "context"

func rdmaAvailable() bool {
	return false
}

func rdmaDeviceNames() ([]string, error) {
	return nil, errRDMAUnavailable
}

func openRDMADevice(name string) (*rdmaDevice, error) {
	return nil, errRDMAUnavailable
}

func (d *rdmaDevice) Close() error {
	return nil
}

func newRDMAProtectionDomain(dev *rdmaDevice) (*rdmaProtectionDomain, error) {
	return nil, errRDMAUnavailable
}

func (p *rdmaProtectionDomain) Close() error {
	return nil
}

func newRDMACompletionQueue(dev *rdmaDevice, capacity int) (*rdmaCompletionQueue, error) {
	return nil, errRDMAUnavailable
}

func (c *rdmaCompletionQueue) Close() error {
	return nil
}

func newRDMAQueuePair(pd *rdmaProtectionDomain, cq *rdmaCompletionQueue) (*rdmaQueuePair, error) {
	return nil, errRDMAUnavailable
}

func (q *rdmaQueuePair) Close() error {
	return nil
}

func drainRDMAQueuePair(qp *rdmaQueuePair, cq *rdmaCompletionQueue) error {
	return errRDMAUnavailable
}

func (q *rdmaQueuePair) number() uint32 {
	return 0
}

func localRDMADestination(qp *rdmaQueuePair) (rdmaDestination, error) {
	return rdmaDestination{}, errRDMAUnavailable
}

func queryRDMAPort(dev *rdmaDevice, maxGIDs int) (rdmaPortInfo, error) {
	return rdmaPortInfo{}, errRDMAUnavailable
}

func initRDMAQueuePair(qp *rdmaQueuePair) error {
	return errRDMAUnavailable
}

func readyToReceiveRDMA(ctx context.Context, qp *rdmaQueuePair, local, remote rdmaDestination, policy rdmaRTRPolicy) error {
	return errRDMAUnavailable
}

func readyToSendRDMA(ctx context.Context, qp *rdmaQueuePair, psn uint32) error {
	return errRDMAUnavailable
}

func newRDMAMemoryRegion(pd *rdmaProtectionDomain, size int) (*rdmaMemoryRegion, error) {
	return nil, errRDMAUnavailable
}

func (m *rdmaMemoryRegion) Close() error {
	return nil
}

func postRDMASend(qp *rdmaQueuePair, mr *rdmaMemoryRegion, offset, length int, id uint64) error {
	return errRDMAUnavailable
}

func postRDMARecv(qp *rdmaQueuePair, mr *rdmaMemoryRegion, offset, length int, id uint64) error {
	return errRDMAUnavailable
}

func postRDMASends(qp *rdmaQueuePair, mr *rdmaMemoryRegion, works []rdmaPostWork) error {
	return errRDMAUnavailable
}

func postRDMARecvs(qp *rdmaQueuePair, mr *rdmaMemoryRegion, works []rdmaPostWork) error {
	return errRDMAUnavailable
}

func pollRDMACompletion(ctx context.Context, cq *rdmaCompletionQueue) ([]rdmaWorkRequest, error) {
	return nil, errRDMAUnavailable
}
