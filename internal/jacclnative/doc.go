// Package jacclnative is a pure-Go implementation target for the JACCL C API.
//
// The package is intentionally separate from internal/jacclc. Package jacclc
// binds libjacclc through purego; package jacclnative implements the same
// communicator semantics in Go and is the landing place for the direct Apple
// RDMA backend that calls the provider C ABI through purego.
//
// The current implementation covers API-compatible configuration, dtypes,
// direct point-to-point operations, and all-gather/all-reduce over connected
// RDMA graphs. A full mesh uses direct pairwise exchange; other connected graphs
// use neighbor propagation.
package jacclnative
