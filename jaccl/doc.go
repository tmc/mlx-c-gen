// Package jaccl provides a Go implementation of JACCL.
//
// The package uses the Apple RDMA provider directly through Go bindings. It
// does not call the C++ JACCL wrapper. A group is configured from explicit
// Config values or from the same environment variables used by MLX/JACCL:
//
//	JACCL_RANK
//	JACCL_SIZE
//	JACCL_COORDINATOR
//	JACCL_IBV_DEVICES
//
// The current backend implements direct point-to-point operations and
// all-gather/all-reduce over any connected RDMA graph. A full mesh uses direct
// pairwise exchange; other connected graphs use neighbor propagation.
package jaccl
