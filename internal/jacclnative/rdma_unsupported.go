//go:build !darwin || !arm64

package jacclnative

func rdmaAvailable() bool {
	return false
}

func rdmaDeviceNames() ([]string, error) {
	return nil, errRDMAUnavailable
}
