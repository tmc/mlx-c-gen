package jacclnative

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	sideChannelMaxFrameSize = 8 << 10
	sideChannelMagic        = "mlx-c/jacclnative"
	sideChannelVersion      = 1

	sideChannelDialTimeout = time.Second
	sideChannelDialRetry   = 10 * time.Millisecond
)

var (
	errSideChannelFrameTooLarge  = errors.New("jacclnative: side-channel frame exceeds maximum size")
	errSideChannelMalformedFrame = errors.New("jacclnative: side-channel malformed frame")
)

type sideChannel struct {
	rank int
	size int

	listener net.Listener
	peers    []net.Conn
	once     sync.Once
}

type sideChannelHello struct {
	Magic   string `json:"magic"`
	Version int    `json:"version"`
	Rank    int    `json:"rank"`
	Size    int    `json:"size"`
}

func newSideChannel(ctx context.Context, rank, size int, coordinator string) (*sideChannel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if size < 1 {
		return nil, fmt.Errorf("side channel: size %d must be positive", size)
	}
	if rank < 0 || rank >= size {
		return nil, fmt.Errorf("side channel: rank %d out of range for size %d", rank, size)
	}
	c := &sideChannel{rank: rank, size: size, peers: make([]net.Conn, size)}
	if rank == 0 {
		if err := c.listen(ctx, coordinator); err != nil {
			_ = c.Close()
			return nil, err
		}
		return c, nil
	}
	if err := c.dial(ctx, coordinator); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func (c *sideChannel) listen(ctx context.Context, coordinator string) error {
	ln, err := net.Listen("tcp", coordinator)
	if err != nil {
		return fmt.Errorf("side channel: listen %s: %w", coordinator, err)
	}
	c.listener = ln
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = ln.Close()
		case <-done:
		}
	}()
	defer close(done)

	for accepted := 1; accepted < c.size; {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("side channel: accept %s: %w after %d/%d peers", coordinator, ctx.Err(), accepted-1, c.size-1)
			}
			return fmt.Errorf("side channel: accept: %w", err)
		}
		msg, err := readSideJSON[sideChannelHello](ctx, conn)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("side channel: read hello from %s: %w", conn.RemoteAddr(), err)
		}
		if err := checkSideHello(msg); err != nil {
			_ = conn.Close()
			return err
		}
		if msg.Size != c.size {
			_ = conn.Close()
			return fmt.Errorf("side channel: peer rank %d size %d, want %d", msg.Rank, msg.Size, c.size)
		}
		if msg.Rank <= 0 || msg.Rank >= c.size {
			_ = conn.Close()
			return fmt.Errorf("side channel: peer rank %d out of range", msg.Rank)
		}
		if c.peers[msg.Rank] != nil {
			_ = conn.Close()
			return fmt.Errorf("side channel: duplicate rank %d", msg.Rank)
		}
		c.peers[msg.Rank] = conn
		if err := writeSideJSON(ctx, conn, newSideHello(c.rank, c.size)); err != nil {
			return fmt.Errorf("side channel: write hello ack to rank %d: %w", msg.Rank, err)
		}
		accepted++
	}
	return nil
}

func (c *sideChannel) dial(ctx context.Context, coordinator string) error {
	d := net.Dialer{Timeout: sideChannelDialTimeout}
	var last error
	for {
		if err := ctx.Err(); err != nil {
			return sideDialError(coordinator, err, last)
		}
		conn, err := d.DialContext(ctx, "tcp", coordinator)
		if err == nil {
			if err := writeSideJSON(ctx, conn, newSideHello(c.rank, c.size)); err != nil {
				_ = conn.Close()
				return fmt.Errorf("side channel: write hello to %s: %w", coordinator, err)
			}
			ack, err := readSideJSON[sideChannelHello](ctx, conn)
			if err != nil {
				_ = conn.Close()
				return fmt.Errorf("side channel: read coordinator ack from %s: %w", coordinator, err)
			}
			if err := checkSideHello(ack); err != nil {
				_ = conn.Close()
				return err
			}
			if ack.Rank != 0 || ack.Size != c.size {
				_ = conn.Close()
				return fmt.Errorf("side channel: bad coordinator ack rank=%d size=%d", ack.Rank, ack.Size)
			}
			c.peers[0] = conn
			return nil
		}
		last = err
		timer := time.NewTimer(sideChannelDialRetry)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return sideDialError(coordinator, ctx.Err(), last)
		}
	}
}

func sideDialError(addr string, err, last error) error {
	if last != nil {
		return fmt.Errorf("side channel: dial %s: %w after last error: %v", addr, err, last)
	}
	return fmt.Errorf("side channel: dial %s: %w", addr, err)
}

func newSideHello(rank, size int) sideChannelHello {
	return sideChannelHello{Magic: sideChannelMagic, Version: sideChannelVersion, Rank: rank, Size: size}
}

func checkSideHello(msg sideChannelHello) error {
	if msg.Magic != sideChannelMagic || msg.Version != sideChannelVersion {
		return fmt.Errorf("side channel: incompatible protocol magic=%q version=%d", msg.Magic, msg.Version)
	}
	return nil
}

func (c *sideChannel) AllGather(ctx context.Context, local []byte) ([][]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("side channel: closed")
	}
	if len(local) > sideChannelMaxFrameSize {
		return nil, errSideChannelFrameTooLarge
	}
	if c.rank == 0 {
		values := make([][]byte, c.size)
		values[0] = append([]byte(nil), local...)
		for rank := 1; rank < c.size; rank++ {
			payload, err := readSideFrame(ctx, c.peers[rank])
			if err != nil {
				return nil, fmt.Errorf("side channel: allgather rank 0 read rank %d payload: %w", rank, err)
			}
			values[rank] = payload
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return nil, fmt.Errorf("side channel: marshal allgather: %w", err)
		}
		for rank := 1; rank < c.size; rank++ {
			if err := writeSideFrame(ctx, c.peers[rank], encoded); err != nil {
				return nil, fmt.Errorf("side channel: allgather rank 0 broadcast to rank %d: %w", rank, err)
			}
		}
		return values, nil
	}
	if err := writeSideFrame(ctx, c.peers[0], local); err != nil {
		return nil, fmt.Errorf("side channel: allgather rank %d write rank 0 payload: %w", c.rank, err)
	}
	payload, err := readSideFrame(ctx, c.peers[0])
	if err != nil {
		return nil, fmt.Errorf("side channel: allgather rank %d read rank 0 broadcast: %w", c.rank, err)
	}
	var values [][]byte
	if err := json.Unmarshal(payload, &values); err != nil {
		return nil, fmt.Errorf("side channel: decode allgather: %w", err)
	}
	if len(values) != c.size {
		return nil, fmt.Errorf("side channel: gathered %d values, want %d", len(values), c.size)
	}
	return values, nil
}

func (c *sideChannel) Barrier(ctx context.Context) error {
	_, err := c.AllGather(ctx, nil)
	return err
}

func (c *sideChannel) Close() error {
	if c == nil {
		return nil
	}
	c.once.Do(func() {
		if c.listener != nil {
			_ = c.listener.Close()
		}
		for _, p := range c.peers {
			if p != nil {
				_ = p.Close()
			}
		}
	})
	return nil
}

func writeSideJSON[T any](ctx context.Context, conn net.Conn, v T) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeSideFrame(ctx, conn, data)
}

func readSideJSON[T any](ctx context.Context, conn net.Conn) (T, error) {
	var v T
	data, err := readSideFrame(ctx, conn)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("side channel: decode frame: %w", err)
	}
	return v, nil
}

func writeSideFrame(ctx context.Context, conn net.Conn, data []byte) error {
	return withSideContext(ctx, conn, func() error { return writeSideFrameRaw(conn, data) })
}

func readSideFrame(ctx context.Context, conn net.Conn) ([]byte, error) {
	var data []byte
	err := withSideContext(ctx, conn, func() error {
		var err error
		data, err = readSideFrameRaw(conn)
		return err
	})
	return data, err
}

func writeSideFrameRaw(w io.Writer, payload []byte) error {
	if len(payload) > sideChannelMaxFrameSize {
		return errSideChannelFrameTooLarge
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if err := writeFull(w, hdr[:]); err != nil {
		return fmt.Errorf("side channel: write frame header: %w", err)
	}
	if len(payload) == 0 {
		return nil
	}
	if err := writeFull(w, payload); err != nil {
		return fmt.Errorf("side channel: write frame payload: %w", err)
	}
	return nil
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func readSideFrameRaw(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: %v", errSideChannelMalformedFrame, err)
		}
		return nil, fmt.Errorf("side channel: read frame header: %w", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > sideChannelMaxFrameSize {
		return nil, errSideChannelFrameTooLarge
	}
	payload := make([]byte, int(n))
	if n == 0 {
		return payload, nil
	}
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: %v", errSideChannelMalformedFrame, err)
		}
		return nil, fmt.Errorf("side channel: read frame payload: %w", err)
	}
	return payload, nil
}

func withSideContext(ctx context.Context, conn net.Conn, fn func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var touchedDeadline atomic.Bool
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		touchedDeadline.Store(true)
		_ = conn.SetDeadline(deadline)
	}
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		select {
		case <-ctx.Done():
			touchedDeadline.Store(true)
			_ = conn.SetDeadline(time.Now())
		case <-done:
		}
	}()
	err := fn()
	close(done)
	<-exited
	if touchedDeadline.Load() {
		_ = conn.SetDeadline(time.Time{})
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil && hasDeadline && !time.Now().Before(deadline) {
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}
