//go:build !darwin || !arm64

package jacclnative

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

func (q *rdmaQueuePair) number() uint32 {
	return 0
}

func newRDMAMemoryRegion(pd *rdmaProtectionDomain, size int) (*rdmaMemoryRegion, error) {
	return nil, errRDMAUnavailable
}

func (m *rdmaMemoryRegion) Close() error {
	return nil
}
