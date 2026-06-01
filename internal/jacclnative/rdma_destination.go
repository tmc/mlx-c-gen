package jacclnative

// rdmaDestination is the queue-pair metadata exchanged on the side channel.
type rdmaDestination struct {
	LID      uint16
	QPN      uint32
	PSN      uint32
	GIDIndex int
	GID      [16]byte
}

type rdmaPortInfo struct {
	Device           string
	PortNum          int
	LID              uint16
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
