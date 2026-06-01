// Package jaccl provides a Go implementation of JACCL.
//
// The package uses the Apple RDMA provider directly through Go bindings. It
// does not call the C++ JACCL wrapper. A group is configured from explicit
// Config values or from the same environment variables used by MLX/JACCL:
//
//	JACCL_RANK
//	JACCL_COORDINATOR
//	JACCL_IBV_DEVICES
//	JACCL_RING
//
// MLX_RANK, MLX_JACCL_COORDINATOR, MLX_IBV_DEVICES, and MLX_JACCL_RING are
// accepted as aliases. JACCL_SIZE, MLX_WORLD_SIZE, and MLX_SIZE are optional;
// when a device matrix is provided, the matrix determines the group size.
//
// The current backend implements direct point-to-point operations and
// all-gather/all-reduce over any connected RDMA graph. A full mesh uses direct
// pairwise exchange; other connected graphs use neighbor propagation.
//
// Typed helpers such as AllSum are provided for Go values. Raw byte helpers
// such as AllSumBytes and AllGatherBytes match the standalone JACCL group API,
// where reductions take a DType and gather/send/recv operate on bytes.
package jaccl
