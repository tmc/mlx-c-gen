//go:build darwin && arm64

package jacclnative

import (
	"fmt"

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
