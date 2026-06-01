// Package jacclnative is a pure-Go implementation target for the JACCL C API.
//
// The package is intentionally separate from internal/jacclc. Package jacclc
// binds libjacclc through purego; package jacclnative implements the same
// communicator semantics in Go and is the landing place for the direct Apple
// RDMA backend that calls the provider C ABI through purego.
//
// The current implementation covers the API-compatible configuration, dtype
// model, single-rank group behavior, and local reduction kernels. Multi-rank
// groups currently fail closed until the RDMA transport is ported into this
// repository.
package jacclnative
