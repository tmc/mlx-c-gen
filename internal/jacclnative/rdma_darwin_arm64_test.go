//go:build darwin && arm64

package jacclnative

import (
	"strings"
	"testing"
)

func TestValidateRDMAPostWork(t *testing.T) {
	mr := &rdmaMemoryRegion{buf: make([]byte, 16)}
	tests := []struct {
		name string
		work rdmaPostWork
		ok   bool
	}{
		{"full buffer", rdmaPostWork{Offset: 0, Length: 16}, true},
		{"zero length at end", rdmaPostWork{Offset: 16}, true},
		{"negative offset", rdmaPostWork{Offset: -1, Length: 1}, false},
		{"negative length", rdmaPostWork{Offset: 0, Length: -1}, false},
		{"past end", rdmaPostWork{Offset: 17}, false},
		{"too long", rdmaPostWork{Offset: 8, Length: 9}, false},
	}
	for _, tt := range tests {
		err := validateRDMAPostWork("post rdma send", mr, 0, tt.work)
		if tt.ok && err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if !tt.ok && err == nil {
			t.Fatalf("%s: succeeded", tt.name)
		}
	}
}

func TestRDMAPostEndStringOverflow(t *testing.T) {
	max := int(^uint(0) >> 1)
	got := rdmaPostEndString(rdmaPostWork{Offset: max, Length: 1})
	if got != "overflow" {
		t.Fatalf("rdmaPostEndString overflow = %q, want overflow", got)
	}

	mr := &rdmaMemoryRegion{buf: make([]byte, 16)}
	err := validateRDMAPostWork("post rdma send", mr, 0, rdmaPostWork{Offset: max, Length: 1})
	if err == nil {
		t.Fatal("validateRDMAPostWork overflow succeeded")
	}
	if !strings.Contains(err.Error(), "[") || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("overflow error = %v, want overflow range", err)
	}
}

func TestPostRDMAManySkipsZeroLengthWithoutPoster(t *testing.T) {
	qp := &rdmaQueuePair{handle: 1}
	mr := &rdmaMemoryRegion{handle: 1, buf: make([]byte, 16)}

	if err := postRDMASends(qp, mr, []rdmaPostWork{{Offset: 16}}); err != nil {
		t.Fatalf("post zero-length send: %v", err)
	}
	if err := postRDMARecvs(qp, mr, []rdmaPostWork{{Offset: 16}}); err != nil {
		t.Fatalf("post zero-length recv: %v", err)
	}
}

func TestPostRDMAManyReportsMissingPoster(t *testing.T) {
	qp := &rdmaQueuePair{handle: 1}
	mr := &rdmaMemoryRegion{handle: 1, buf: make([]byte, 16)}

	err := postRDMASends(qp, mr, []rdmaPostWork{{Length: 1}})
	if err == nil || !strings.Contains(err.Error(), "poster is unavailable") {
		t.Fatalf("post send error = %v, want missing poster", err)
	}
	err = postRDMARecvs(qp, mr, []rdmaPostWork{{Length: 1}})
	if err == nil || !strings.Contains(err.Error(), "poster is unavailable") {
		t.Fatalf("post recv error = %v, want missing poster", err)
	}
}

func TestPollRDMACompletionReportsMissingPoller(t *testing.T) {
	_, err := pollRDMACompletion(t.Context(), &rdmaCompletionQueue{handle: 1})
	if err == nil || !strings.Contains(err.Error(), "poller is unavailable") {
		t.Fatalf("poll error = %v, want missing poller", err)
	}
}
