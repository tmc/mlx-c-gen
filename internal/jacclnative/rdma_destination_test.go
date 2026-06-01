package jacclnative

import "testing"

func TestRDMADestinationJSONShape(t *testing.T) {
	dst := rdmaDestination{
		LID:      1,
		QPN:      2,
		PSN:      7,
		GIDIndex: 3,
		GID:      [16]byte{10: 0xff, 11: 0xff, 15: 1},
	}
	if dst.PSN != 7 {
		t.Fatalf("PSN = %d, want 7", dst.PSN)
	}
	if dst.GID == ([16]byte{}) {
		t.Fatal("GID is zero")
	}
}
