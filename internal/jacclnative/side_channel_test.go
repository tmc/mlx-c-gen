package jacclnative

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestSideFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := []byte("rank metadata")
	if err := writeSideFrameRaw(&buf, want); err != nil {
		t.Fatal(err)
	}
	got, err := readSideFrameRaw(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("frame = %q, want %q", got, want)
	}
}

func TestSideChannelAllGather(t *testing.T) {
	size := 3
	addr := freeLocalAddr(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chans := make([]*sideChannel, size)
	errs := make(chan error, size)
	var wg sync.WaitGroup
	for rank := 0; rank < size; rank++ {
		rank := rank
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, err := newSideChannel(ctx, rank, size, addr)
			if err != nil {
				errs <- fmt.Errorf("rank %d: %w", rank, err)
				return
			}
			chans[rank] = ch
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	defer func() {
		for _, ch := range chans {
			_ = ch.Close()
		}
	}()

	got := make([][][]byte, size)
	errs = make(chan error, size)
	for rank := 0; rank < size; rank++ {
		rank := rank
		wg.Add(1)
		go func() {
			defer wg.Done()
			values, err := chans[rank].AllGather(ctx, []byte(fmt.Sprintf("rank-%d", rank)))
			if err != nil {
				errs <- fmt.Errorf("rank %d: %w", rank, err)
				return
			}
			got[rank] = values
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	want := [][]byte{[]byte("rank-0"), []byte("rank-1"), []byte("rank-2")}
	for rank := 0; rank < size; rank++ {
		if !reflect.DeepEqual(got[rank], want) {
			t.Fatalf("rank %d allgather = %q, want %q", rank, got[rank], want)
		}
	}
}

func TestSideChannelBarrier(t *testing.T) {
	size := 3
	chans := newSideChannels(t, size)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errs := make(chan error, size)
	var wg sync.WaitGroup
	for rank := 0; rank < size; rank++ {
		rank := rank
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := chans[rank].Barrier(ctx); err != nil {
				errs <- fmt.Errorf("rank %d: %w", rank, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func newSideChannels(t *testing.T, size int) []*sideChannel {
	t.Helper()
	addr := freeLocalAddr(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chans := make([]*sideChannel, size)
	errs := make(chan error, size)
	var wg sync.WaitGroup
	for rank := 0; rank < size; rank++ {
		rank := rank
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, err := newSideChannel(ctx, rank, size, addr)
			if err != nil {
				errs <- fmt.Errorf("rank %d: %w", rank, err)
				return
			}
			chans[rank] = ch
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, ch := range chans {
			_ = ch.Close()
		}
	})
	return chans
}

func freeLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}
