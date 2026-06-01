# Native JACCL

This package is a Go implementation of JACCL over the Apple RDMA provider.
It calls the provider verbs through `github.com/tmc/apple/rdma` and does not
call the C++ JACCL wrapper.

The implemented transport is a direct mesh:

- TCP side channel for rank metadata and RDMA destination exchange.
- Apple RDMA device, protection-domain, completion-queue, queue-pair, and
  memory-region allocation.
- Queue-pair INIT, RTR, and RTS transitions.
- SEND/RECV work requests with completion polling.
- Point-to-point send/recv and mesh all-gather/all-reduce.

Ring, line, and graph collectives are not implemented yet. Until they are,
the backend requires every non-local rank to have at least one direct RDMA wire
in the device matrix.

## Smoke

List provider devices:

```sh
go run ./tools/mlx-c-jacclnative-smoke -op devices
```

Run a local two-rank barrier using one RDMA device:

```sh
JACCL_NATIVE_TRACE=1 go run ./tools/mlx-c-jacclnative-smoke \
  -local-two-rank-device rdma_en1 \
  -op barrier \
  -timeout 8s
```

The command creates a temporary two-rank device matrix and launches both ranks.
Use `-op barrier-sum`, `-op allgather`, or `-op sendrecv` after `barrier` has
proven provider setup.

Current local evidence on this machine:

- `rdma_en1` and `rdma_en3` open, then `Ibv_alloc_pd` returns a nil handle.
- `rdma_en2` opens, then blocks in `Ibv_alloc_pd` until the outer timeout kills
  the rank process.

The standalone `gojaccl` `jacclctl rdma-alloc` command shows the same provider
behavior on the same devices, so this is not currently proof of a native package
regression.
