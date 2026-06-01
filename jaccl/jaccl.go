package jaccl

import (
	"context"

	"github.com/ml-explore/mlx-c/internal/jacclnative"
)

// DType is the JACCL element type used by reductions.
type DType = jacclnative.DType

const (
	DTypeBool      = jacclnative.DTypeBool
	DTypeInt8      = jacclnative.DTypeInt8
	DTypeInt16     = jacclnative.DTypeInt16
	DTypeInt32     = jacclnative.DTypeInt32
	DTypeInt64     = jacclnative.DTypeInt64
	DTypeUint8     = jacclnative.DTypeUint8
	DTypeUint16    = jacclnative.DTypeUint16
	DTypeUint32    = jacclnative.DTypeUint32
	DTypeUint64    = jacclnative.DTypeUint64
	DTypeFloat16   = jacclnative.DTypeFloat16
	DTypeBFloat16  = jacclnative.DTypeBFloat16
	DTypeFloat32   = jacclnative.DTypeFloat32
	DTypeFloat64   = jacclnative.DTypeFloat64
	DTypeComplex64 = jacclnative.DTypeComplex64
)

// Element is a Go element type supported by typed collectives.
type Element = jacclnative.Element

// Config describes one rank in a JACCL group.
type Config = jacclnative.Config

// IsValidMesh reports whether cfg has every pairwise RDMA connection.
func IsValidMesh(cfg Config) bool {
	return cfg.IsValidMesh()
}

// IsValidRing reports whether cfg has the RDMA connections for ring topology.
func IsValidRing(cfg Config) bool {
	return cfg.IsValidRing()
}

// Group is a live communicator.
type Group = jacclnative.Group

// ConfigFromEnv reads JACCL environment variables into a Config.
func ConfigFromEnv() (Config, error) {
	return jacclnative.ConfigFromEnv()
}

// NewGroup initializes a native Go JACCL group.
func NewGroup(ctx context.Context, cfg Config) (*Group, error) {
	return jacclnative.NewGroup(ctx, cfg)
}

// NewGroupFromEnv reads ConfigFromEnv and initializes a group.
func NewGroupFromEnv(ctx context.Context) (*Group, error) {
	return jacclnative.NewGroupFromEnv(ctx)
}

// RDMAAvailable reports whether the native Apple RDMA provider is available.
func RDMAAvailable() bool {
	return jacclnative.RDMAAvailable()
}

// RDMADeviceNames reports the RDMA device names visible to the native provider.
func RDMADeviceNames() ([]string, error) {
	return jacclnative.RDMADeviceNames()
}

// Barrier waits until every rank enters the barrier.
func Barrier(ctx context.Context, g *Group) error {
	return jacclnative.Barrier(ctx, g)
}

// AllSum computes the element-wise sum across all ranks.
func AllSum[T Element](ctx context.Context, g *Group, dst, src []T) error {
	return jacclnative.AllSum(ctx, g, dst, src)
}

// AllSumBytes sum-reduces raw bytes using dtype.
func AllSumBytes(ctx context.Context, g *Group, dst, src []byte, dtype DType) error {
	return jacclnative.AllSumBytes(ctx, g, dst, src, dtype)
}

// AllMax computes the element-wise maximum across all ranks.
func AllMax[T Element](ctx context.Context, g *Group, dst, src []T) error {
	return jacclnative.AllMax(ctx, g, dst, src)
}

// AllMaxBytes max-reduces raw bytes using dtype.
func AllMaxBytes(ctx context.Context, g *Group, dst, src []byte, dtype DType) error {
	return jacclnative.AllMaxBytes(ctx, g, dst, src, dtype)
}

// AllMin computes the element-wise minimum across all ranks.
func AllMin[T Element](ctx context.Context, g *Group, dst, src []T) error {
	return jacclnative.AllMin(ctx, g, dst, src)
}

// AllMinBytes min-reduces raw bytes using dtype.
func AllMinBytes(ctx context.Context, g *Group, dst, src []byte, dtype DType) error {
	return jacclnative.AllMinBytes(ctx, g, dst, src, dtype)
}

// AllGather gathers each rank's src into dst in rank order.
func AllGather[T Element](ctx context.Context, g *Group, dst, src []T) error {
	return jacclnative.AllGather(ctx, g, dst, src)
}

// AllGatherBytes gathers raw bytes from each rank into dst in rank order.
func AllGatherBytes(ctx context.Context, g *Group, dst, src []byte) error {
	return jacclnative.AllGatherBytes(ctx, g, dst, src)
}

// Send sends raw bytes to dst.
func Send(ctx context.Context, g *Group, dst int, src []byte) error {
	return jacclnative.Send(ctx, g, dst, src)
}

// Recv receives raw bytes from src.
func Recv(ctx context.Context, g *Group, src int, dst []byte) error {
	return jacclnative.Recv(ctx, g, src, dst)
}
