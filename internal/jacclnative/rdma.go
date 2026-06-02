package jacclnative

// RDMAAvailable reports whether the native Apple RDMA provider is available.
func RDMAAvailable() bool {
	return rdmaAvailable()
}

// RDMADeviceNames reports the RDMA device names visible to the native provider.
func RDMADeviceNames() ([]string, error) {
	return rdmaDeviceNames()
}

// RDMAPortInfo reports provider port and GID table metadata for one device.
type RDMAPortInfo = rdmaPortInfo

// RDMAGIDEntry reports one provider GID table entry.
type RDMAGIDEntry = rdmaGIDEntry

// QueryRDMAPort reports provider port and GID table metadata for device.
func QueryRDMAPort(device string, maxGIDs int) (RDMAPortInfo, error) {
	dev, err := openRDMADevice(device)
	if err != nil {
		return RDMAPortInfo{}, err
	}
	defer dev.Close()
	return queryRDMAPort(dev, maxGIDs)
}
