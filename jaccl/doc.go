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
// The current backend implements direct-mesh point-to-point operations and mesh
// all-gather/all-reduce. Ring, line, and graph collectives are not implemented.
package jaccl
