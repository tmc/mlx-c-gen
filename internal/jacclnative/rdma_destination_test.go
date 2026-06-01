package jacclnative

import (
	"strings"
	"testing"
)

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

func TestValidateRDMADestinationMatrix(t *testing.T) {
	all := [][][]rdmaDestination{
		{nil, {{QPN: 11, PSN: 7, GIDIndex: -1, LID: 1}}},
		{{{QPN: 12, PSN: 7, GIDIndex: 2, GID: [16]byte{15: 1}}}, nil},
	}
	if err := validateRDMADestinationMatrix(all, 2); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRDMADestinationMatrixRejectsBadMetadata(t *testing.T) {
	tests := []struct {
		name string
		all  [][][]rdmaDestination
		want string
	}{
		{
			name: "bad rank count",
			all:  [][][]rdmaDestination{{nil}},
			want: "got 1 ranks, want 2",
		},
		{
			name: "bad peer count",
			all:  [][][]rdmaDestination{{nil}, {nil, nil}},
			want: "rank 0 has 1 peers, want 2",
		},
		{
			name: "zero qpn",
			all: [][][]rdmaDestination{
				{nil, {{LID: 1}}},
				{nil, nil},
			},
			want: "qpn is zero",
		},
		{
			name: "zero address",
			all: [][][]rdmaDestination{
				{nil, {{QPN: 1}}},
				{nil, nil},
			},
			want: "lid and gid are both zero",
		},
		{
			name: "bad psn",
			all: [][][]rdmaDestination{
				{nil, {{QPN: 1, LID: 1, PSN: maxRDMAPSN + 1}}},
				{nil, nil},
			},
			want: "psn 16777216 out of 24-bit range",
		},
		{
			name: "bad gid index",
			all: [][][]rdmaDestination{
				{nil, {{QPN: 1, GIDIndex: 300, GID: [16]byte{15: 1}}}},
				{nil, nil},
			},
			want: "gid index 300 out of uint8 range",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRDMADestinationMatrix(tt.all, 2)
			if err == nil {
				t.Fatal("validateRDMADestinationMatrix succeeded")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
