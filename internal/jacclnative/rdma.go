package jacclnative

// RDMAAvailable reports whether the native Apple RDMA provider is available.
func RDMAAvailable() bool {
	return rdmaAvailable()
}

// RDMADeviceNames reports the RDMA device names visible to the native provider.
func RDMADeviceNames() ([]string, error) {
	return rdmaDeviceNames()
}
