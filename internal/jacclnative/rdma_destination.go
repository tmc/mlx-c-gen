package jacclnative

import "fmt"

const maxRDMAPSN = 0xffffff

// rdmaDestination is the queue-pair metadata exchanged on the side channel.
type rdmaDestination struct {
	LID       uint16
	QPN       uint32
	PSN       uint32
	GIDIndex  int
	GID       [16]byte
	ActiveMTU int32
}

type rdmaPortInfo struct {
	Device           string
	PortNum          int
	LID              uint16
	ActiveMTU        int32
	GIDTableLength   int
	GIDScanLimit     int
	SelectedGIDIndex int
	GIDs             []rdmaGIDEntry
}

type rdmaGIDEntry struct {
	Index      int
	GID        [16]byte
	IPv4Mapped bool
	Zero       bool
}

type rdmaRTRPolicy struct {
	ZeroDLIDWhenGlobal bool
	GRHHopLimit        uint8
}

func validateRDMADestinationMatrix(all [][][]rdmaDestination, size int) error {
	if len(all) != size {
		return fmt.Errorf("rdma destinations: got %d ranks, want %d", len(all), size)
	}
	for rank, row := range all {
		if len(row) != size {
			return fmt.Errorf("rdma destinations: rank %d has %d peers, want %d", rank, len(row), size)
		}
		for peer, dsts := range row {
			for wire, dst := range dsts {
				if err := validateRDMADestination(rank, peer, wire, dst); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateRDMADestination(rank, peer, wire int, dst rdmaDestination) error {
	if dst.QPN == 0 {
		return fmt.Errorf("rdma destination rank %d peer %d wire %d: qpn is zero", rank, peer, wire)
	}
	if dst.LID == 0 && dst.GID == ([16]byte{}) {
		return fmt.Errorf("rdma destination rank %d peer %d wire %d: lid and gid are both zero", rank, peer, wire)
	}
	if dst.PSN > maxRDMAPSN {
		return fmt.Errorf("rdma destination rank %d peer %d wire %d: psn %d out of 24-bit range", rank, peer, wire, dst.PSN)
	}
	if dst.GID != ([16]byte{}) && (dst.GIDIndex < 0 || dst.GIDIndex > 255) {
		return fmt.Errorf("rdma destination rank %d peer %d wire %d: gid index %d out of uint8 range", rank, peer, wire, dst.GIDIndex)
	}
	return nil
}
