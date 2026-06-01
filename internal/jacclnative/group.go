package jacclnative

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var errClosed = errors.New("jacclnative: group closed")
var errRDMAUnavailable = errors.New("jacclnative: rdma unavailable")

// Group is a live communicator.
type Group struct {
	rank int
	size int

	once   sync.Once
	closed chan struct{}
}

// NewGroup initializes a native Go JACCL group.
func NewGroup(ctx context.Context, cfg Config) (*Group, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	size, err := cfg.groupSize()
	if err != nil {
		return nil, err
	}
	if size != 1 {
		if !rdmaAvailable() {
			return nil, errRDMAUnavailable
		}
		return nil, fmt.Errorf("native multi-rank transport not implemented")
	}
	return &Group{rank: cfg.Rank, size: size, closed: make(chan struct{})}, nil
}

// NewGroupFromEnv reads ConfigFromEnv and initializes a group.
func NewGroupFromEnv(ctx context.Context) (*Group, error) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return NewGroup(ctx, cfg)
}

// Rank reports the local rank.
func (g *Group) Rank() int {
	if g == nil {
		return -1
	}
	return g.rank
}

// Size reports the group size.
func (g *Group) Size() int {
	if g == nil {
		return 0
	}
	return g.size
}

// Close releases group resources. It is safe to call more than once.
func (g *Group) Close() error {
	if g == nil {
		return nil
	}
	g.once.Do(func() {
		close(g.closed)
	})
	return nil
}

func (g *Group) check(ctx context.Context, op string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if g == nil {
		return fmt.Errorf("%s: %w", op, errClosed)
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", op, ctx.Err())
	case <-g.closed:
		return fmt.Errorf("%s: %w", op, errClosed)
	default:
		return nil
	}
}

// Barrier waits until every rank enters the same barrier.
func (g *Group) Barrier(ctx context.Context) error {
	return g.check(ctx, "barrier")
}

// Send sends bytes to dst.
func (g *Group) Send(ctx context.Context, dst int, src []byte) error {
	if err := g.check(ctx, "send"); err != nil {
		return err
	}
	return fmt.Errorf("send: native multi-rank transport not implemented")
}

// Recv receives bytes from src.
func (g *Group) Recv(ctx context.Context, src int, dst []byte) error {
	if err := g.check(ctx, "recv"); err != nil {
		return err
	}
	return fmt.Errorf("recv: native multi-rank transport not implemented")
}
