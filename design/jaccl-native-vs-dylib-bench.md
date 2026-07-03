# Pure-Go (jacclnative) vs dylib wrapper (jacclc) — benchmark results

Date: 2026-06-27 · Machine: Apple M4 Max · `goos=darwin goarch=arm64`
Branch: `gen-jaccl-purego-native`

## Methodology

`internal/jacclc/parity_bench_test.go` `BenchmarkCompare*` runs **one** of the two
implementations, selected by `MLX_C_JACCL_BENCH_IMPL={native,jacclc}`. We ran each
suite with `-count=6` and diffed with `benchstat`.

- **native** = `internal/jacclnative`, pure-Go, no cgo.
- **jacclc** = `internal/jacclc` purego bindings → `libjacclc.dylib` → `libjaccl.dylib`.

The dylib was built fresh from this branch's `mlx/c/jaccl.cpp` against the jaccl
C++ lib in `/Users/tmc/ml-explore/mlx` (mlx `v0.31.2-49-g8b778060a`). One build fix
was required: `LocalGroup` in `jaccl.cpp` lacked a `barrier()` override that the
installed `jaccl::Group` interface now declares pure-virtual (added a no-op, correct
for a size-1 group). Dylib staged with its `libjaccl.dylib` sibling in
`/tmp/jacclc-lib`; `MLX_C_JACCLC_TEST_LIB` points there. All `TestParity*` pass, so
the two implementations are functionally identical.

## Scope: two regimes, measured separately

This doc has **two** comparisons:

1. **Single-rank (size-1), local** — the benchstat table just below. These measure
   *call overhead* and *local memory movement* (a size-1 collective is a memcpy on
   both sides), i.e. an **FFI-overhead + local-op** comparison. Native wins.
2. **Two-host, real RDMA over Thunderbolt** — the "Real two-host RDMA results"
   section further down. These measure actual distributed round-trip latency. The
   dylib wins, reversing (1).

The two regimes disagree, and that is the point: which transport is faster depends
entirely on whether the cost is local FFI/memcpy or RDMA round-trips. Read both.

## Headline numbers (sec/op, native vs jacclc)

| Benchmark            | native    | jacclc    | jacclc vs native |
|----------------------|-----------|-----------|------------------|
| **Scalar getters/setters (the FFI tax)** |        |           |                  |
| ConfigSetRank        | 0.26 ns   | 58.5 ns   | **+22000%**      |
| ConfigSetCoordinator | 0.26 ns   | 96.9 ns   | +37000%          |
| ConfigSetDevicesJSON | 0.26 ns   | 599.6 ns  | +233000%         |
| ClearError           | 0.26 ns   | 55.0 ns   | +21000%          |
| ConfigNewClose       | 0.91 ns   | 145.1 ns  | +15800%          |
| GroupRank            | 0.77 ns   | 2.71 ns   | +250%            |
| DTypeSize            | 1.56 ns   | 1.55 ns   | ~ (Go-side both) |
| **Collectives — small** |        |           |                  |
| AllSumBytes/64B      | 22.4 ns   | 9.6 ns    | **−57%** (jacclc faster) |
| AllMaxBytes/64B      | 22.9 ns   | 8.2 ns    | −64%             |
| AllGatherBytes/64B   | 19.8 ns   | 9.6 ns    | −51%             |
| Barrier              | 15.8 ns   | 2.1 ns    | −87%             |
| **Collectives — mid** |          |           |                  |
| AllSumBytes/4KiB     | 60.6 ns   | 61.3 ns   | ~ (tie)          |
| AllGatherBytes/4KiB  | 57.0 ns   | 57.8 ns   | ~ (tie)          |
| **Collectives — large** |        |           |                  |
| AllSumBytes/1MiB     | 13.1 µs   | 20.5 µs   | **+56%** (native faster) |
| AllMaxBytes/1MiB     | 12.9 µs   | 18.6 µs   | +44%             |
| AllGatherBytes/1MiB  | 13.0 µs   | 18.9 µs   | +45%             |
| **Composite** |               |           |                  |
| LLMForward/Decode    | 11.3 µs   | 10.7 µs   | ~ (tie)          |
| LLMForward/Prefill128| 1.25 ms   | 1.85 ms   | +48% (native faster) |

Allocations: native is **0 B/op** on nearly all ops; jacclc allocates 208 B/op on
every config setter/getter and error call (the purego call path boxes args), and
416 B on ConfigNewClose. (One exception: `ConfigFromEnvClose` native allocates more
— 1176 B vs 416 — because the native env parse builds more Go garbage.)

## Reading the data

Three regimes, all explained by **where the work is**:

1. **Tiny scalar ops** (config setters/getters, error, lifecycle): native wins by
   100×–1000×. A native getter is a Go field read (sub-ns, inlined, zero alloc); the
   jacclc path pays a full purego FFI crossing (~50–100 ns) + a 208 B box per call.
   This is the pure FFI tax, and it's enormous in relative terms because the work is
   nothing.

2. **Small collectives (64 B) + Barrier**: jacclc is ~2× faster. Here native's
   per-call Go machinery (context, slice headers, the collective dispatch) costs more
   than one cheap C call that does a tiny memcpy. The crossover is real.

3. **Large collectives (≥1 MiB) + Prefill**: native wins by ~45–56%. Once the payload
   dominates, native does a direct in-Go memcpy while jacclc moves the same bytes
   across the FFI boundary with extra indirection. No allocations on either side at
   this size, so it's pure data-movement efficiency.

The geomean (+419% for jacclc) is dominated by the scalar-op FFI tax and is
misleading for real workloads — those are dominated by collectives, where the
picture is mixed and size-dependent.

## Takeaways

- For a **control-plane-heavy** caller (lots of config/getter/error calls), pure-Go
  is dramatically cheaper — no FFI, no per-call allocation.
- For **small-message collectives**, the dylib's lean C path edges ahead.
- For **large-message collectives** (the LLM-relevant regime), pure-Go is faster and
  allocation-free.
- None of this measures real **multi-rank RDMA** — to compare the actual distributed
  transports you need ≥2 ranks on hardware (rank-smoke tool), which this run did not
  exercise. See the next section for why a single-host run is impossible.

## Why two hosts are required (single-host is impossible)

A real 2-rank RDMA comparison is **not runnable on one machine** — this is why the
results below needed a second Mac. Findings on a single host (Apple M4 Max):

- **Devices present:** `rdma_en1`, `rdma_en2`, `rdma_en3` (Thunderbolt RDMA
  interfaces), all link-up with valid link-local GIDs (`-mode ports`). `rdma_en3`
  also carries an IPv4-mapped GID (`169.254.240.245`, a TB-bridge self-assigned
  address).
- **Null devices don't form a graph:** a 2-rank run with the default
  `[[null,null],[null,null]]` fails with `rank 0 cannot reach rank 1 through RDMA
  graph` — the backend requires real device names to wire ranks together.
- **Two processes can't share the RDMA provider on one host:** a 2-rank mesh
  (`[[null,["rdma_en1"]],[["rdma_en1"],null]]`, both ranks local) fails at
  `ibv_alloc_pd: ... provider returned nil protection domain (return=0, errno=0)`.
  Using distinct devices per rank (`en1` / `en2`) fails identically.
- **The provider itself is fine:** a *single* process (only rank 0 started) gets
  **past** `alloc_pd` and proceeds to the side-channel coordinator, then times out
  waiting for the absent rank 1. So the nil-PD is two-process contention for the
  macOS RDMA provider, not a device fault.

**Conclusion:** the distributed transport needs **two separate hosts** cabled over
Thunderbolt — rank 0 on machine A, rank 1 on machine B, both naming the same TB
link in the device matrix. That run was then done; see below.

## Real two-host RDMA results (the actual distributed comparison)

Two Apple M4 Max, cabled over Thunderbolt (`en3 -> rdma_en3` on both). Coordinator
(TCP side channel) over Tailscale; RDMA data path over the TB cable. Both
transports have a multi-rank driver: `tools/mlx-c-jacclnative-rank-smoke` (pure-Go)
and `tools/mlx-c-jacclc-rank-smoke` (dylib, `NewGroupWithConfig` →
`mlx_jaccl_init_config`). Same `allsum` mode and verification on both.

### One required config fix: GID symmetry

After a peer reboot, the peer's `en3` came up with **no IPv4**, so the two ends
auto-selected **different GID types** (one IPv4-mapped, one link-local fe80::) and
the QP would not go INIT→RTR (`ibv_modify_qp ... errno 22 (EINVAL)`). `selectPortGID`
(rdma_darwin_arm64.go) prefers an IPv4-mapped GID; both ends must therefore have an
IPv4 on `en3`. Fix: give each `en3` a /30 IPv4 (`.245` / `.246`) so both select the
matching IPv4-mapped GID (index 1). Tailscale hijacking the route does not block GID
enumeration, so a manually-assigned (even unroutable) IPv4 is enough.

### AllSum latency, min of 8 trials (sec/op)

Measurement: each trial is a fresh process pair (rank 0 on peer, rank 1 local),
`-iters` 300–8000 by size; we take the **min** across 8 trials. Min is the right
estimator here because macOS RDMA latency is **very jittery** — individual trials
spread 2–6×, and native throws occasional multi-millisecond stalls (see "wedge"
below). Min strips that jitter to expose the true round-trip latency.

The `allsum` mode validates the full output buffer byte-exact against a `+` oracle
at `size==2` (`checkSum`), so these runs prove **byte-exact AllReduce from 64 B to
1 MiB** — but only for **`DTypeUint8`** (the dtype the tool moves). uint8 is the
trivial reduction path (1 byte, no reinterpret); the multi-byte reinterpret dtypes
(`int32`/`float32`/…) and especially the separate software `float16`/`bfloat16`
path (`allReduceFloat16Bytes`) are **not exercised over real RDMA** here. A
float32 + bf16 validated AllReduce is the highest-value missing coverage.

| Payload | native (min) | jacclc (min) | winner            |
|---------|--------------|--------------|-------------------|
| 64 B    | 12.0 µs      | 5.1 µs       | **jacclc ~2.3×**  |
| 256 B   | 9.9 µs       | 4.7 µs       | **jacclc ~2.1×**  |
| 1 KiB   | 24.7 µs      | 9.6 µs       | **jacclc ~2.6×**  |
| 4 KiB   | 22.4 µs      | 10.0 µs      | **jacclc ~2.2×**  |
| 16 KiB  | 12.8 µs      | 9.7 µs       | **jacclc ~1.3×**  |
| 64 KiB  | 32.9 µs      | 20.4 µs      | **jacclc ~1.6×**  |
| 256 KiB | 108 µs       | 63 µs        | **jacclc ~1.7×**  |
| 1 MiB   | 343 µs       | 204 µs       | **jacclc ~1.7×**  |

**The dylib (jacclc) wins at every size over real RDMA, ~1.3–2.6×.** This *reverses*
the single-rank/local result above (where native won large payloads): once the cost
is RDMA round-trips rather than local memcpy, the lean C send/recv loop has lower
latency than the pure-Go path at every size, and is far more stable.

### Stability and the same-process "wedge"

jacclc trials cluster tightly (e.g. 16 KiB: all 8 within 9.7–20.9 µs); native shows
recurring multi-ms stalls — 16 KiB had two trials at ~3.35 ms, 64 KiB two at
~5.04 ms, 256 KiB two at ~12.6 ms — against a ~10–110 µs floor. These are instances
of a **queue-pair wedge**.

A dedicated reinit test (`NewGroup → Barrier → AllSum → Close`, repeated in **one**
process, 2-rank real RDMA) shows the wedge directly and, importantly, it is **not a
pure-Go bug**:

- It is **stochastic**: across runs the wedge struck at cycle 0, cycle 1, or cycle 3,
  and one run survived all 5 cycles cleanly.
- The signature is always: rank 0's AllSum returns (slowly), rank 1 **hangs polling**
  for the RDMA completion and times out (`context deadline exceeded`).
- With per-iteration output validation, **no run ever produced wrong data** — the
  failure is a clean hang→timeout, **fail-closed, never silent corruption**.
- It reproduces in `jacclnative` *despite* a correct `drainRDMAQueuePair` QP→ERR
  teardown, and (per the parallel mlx-go-ccl investigation) in `gojaccl` too. So it
  is a **provider/driver property of sustained same-process queue-pair reuse on
  Apple TB RDMA**, not a teardown bug in either binding.
- The throughput sweep avoids it because each trial is a **fresh process** — the OS
  reclaims device state on exit, which the in-process QP→ERR drain does not fully
  replicate. (This is why min-of-N is the right estimator: the rare wedge spikes are
  discarded, leaving the genuine latency.)

Practical takeaway: for long-lived process that re-creates groups, prefer
fresh-process-per-group (or accept the documented stochastic wedge) until the
provider supports robust same-process QP recycling.

### 2026-07-02 re-run (m4max-34 26.5.2 ↔ tm4sm 27.0) — three findings + a recovery correction

A fresh two-host run on a **new hardware pair with OS skew** (rank-0 host A macOS
26.5.2, rank-1 host B macOS **27.0**), both ranks driven from a byte-identical
`mlx-c-jacclnative-rank-smoke` (module `github.com/tmc/mlx-c-gen`, sha256
`127a66cb…`, rsynced to B). Setup gates all passed (both `rdma_en3 gid[1]
ipv4_mapped=true`).

1. **First cross-rank AllSum over real RDMA — success.** iters=1, 4 KiB
   (tokens=1,hidden=2048): rank-0 269.75 µs RC=0, rank-1 147.67 µs RC=0. The
   tool's `checkSum` fails nonzero on mismatch, so both-RC=0 ⇒ the reduction is
   **byte-exact cross-rank, not identity** — the result single-host can't produce.
2. **The wedge reproduces on a macOS 27.0 peer.** iters=100 (sustained same-QP
   reuse) wedged: rank-0 per-iter latency ~150 µs → **~100 ms** (RC=0), rank-1
   `context deadline exceeded` RC=1. Fail-closed, never silent corruption. Because
   this is the jacclnative direct path and it wedges *with a 27.0 peer*, **macOS 27
   did not fix the wedge** — a clean control for any later "gojaccl direct-only
   looks fixed" claim (27 still wedges here, so a fix can't be attributed to the OS).
3. **Fresh-process recovery did NOT hold (corrects the model above).** The
   "each trial is a fresh process → OS reclaims device state" note above did **not**
   hold after the iters=100 wedge across the 27.0 peer: three careful single-iter
   fresh-process probes all failed asymmetrically — rank-0 RC=0 (~170–200 µs),
   rank-1 `context deadline exceeded` RC=1. Suspicious: rank-0 RC=0 while rank-1
   fails suggests rank-0 may complete against a **stale/half-open QP**, so a
   one-sided RC=0 is not proof of a healthy round trip — require **both ranks RC=0**.

**Wedge isolated to the QP/CQ layer.** Post-wedge, host A is clean (no stale
`:39091` listener, no lingering RDMA-path TCP, no rank-smoke proc) and host B's
`-mode ports` still shows a **healthy** port/GID (`rdma_en3 gid[1]=…f0f6
ipv4_mapped=true`, lid=3, mtu5). So the durable wedge is **not** at the
port/GID/device-presentation layer — it is at the **QP/completion-queue level on
the host-B peer**, invisible to `-mode ports`, while the device still looks
healthy. **Recovery verdict:** ~7–8 min idle did **not** clear it — a 4th
single-iter probe post-idle showed the same asymmetric failure (rank-0 RC=0
130.6 µs / rank-1 `context deadline exceeded`). So the durable QP/CQ state does
**not age out on short idle**; recovery needs a longer park or an explicit
host-B device reset/reboot. (A one-sided rank-0 RC=0 continued to mislead — it
completes against the stale/half-open QP.)

Minor: `IOConnectUnmapMemory kr=0xe00002bc` ×4 at `group.Close()` is **rank-0
only** (asymmetric), post-success, RC=0 — cosmetic MR-unmap noise.

Caveat: `-mode allsum` reduces **raw bytes** (uint8 path), not a typed fp32/bf16
reduction — that remains the open gap and needs a different harness.

### Reproduce the two-host run

```
# both en3 need a /30 IPv4 so both pick the IPv4-mapped GID:
#   host A: sudo ifconfig en3 inet 169.254.240.245 netmask 255.255.255.252
#   host B: sudo ifconfig en3 inet 169.254.240.246 netmask 255.255.255.252
# coordinator = an addr rank 1 can reach. NOTE (2026-07-02): between two TB-cabled
# Macs the peer is often NOT on the same tailnet (tailscale dial -> i/o timeout);
# use the TB bridge link-local instead, e.g. host-A bridge0 169.254.34.156:39091.
DEV='[[null,["rdma_en3"]],[["rdma_en3"],null]]'

# native
mlx-c-jacclnative-rank-smoke -rank 0 -size 2 -coordinator <A-ip>:39091 \
  -devices "$DEV" -mode allsum -iters 2000 -tokens 1 -hidden 2048   # host A
mlx-c-jacclnative-rank-smoke -rank 1 -size 2 -coordinator <A-ip>:39091 \
  -devices "$DEV" -mode allsum -iters 2000 -tokens 1 -hidden 2048   # host B

# dylib (note: no -timeout flag; needs -lib)
mlx-c-jacclc-rank-smoke -lib /tmp/jacclc-lib -rank 0 -size 2 \
  -coordinator <A-ip>:39091 -devices "$DEV" -mode allsum -iters 2000 -tokens 1 -hidden 2048
# host B: same with -rank 1
```

Payload bytes = `tokens*hidden*2`; each rank prints `ns_per_iter`. Take the min over
several trials. `-mode llm` exercises the prefill/decode collective pattern.

## Reproduce (single-rank local)

```
# build the dylib (needs jaccl C++ lib in /Users/tmc/ml-explore/mlx, SDK >= 26.2)
cmake -S . -B /tmp/mlxc-bench-build -DCMAKE_BUILD_TYPE=Release \
  -DFETCHCONTENT_SOURCE_DIR_MLX=/Users/tmc/ml-explore/mlx \
  -DMLX_C_BUILD_JACCL=ON -DBUILD_SHARED_LIBS=ON
cmake --build /tmp/mlxc-bench-build --target jacclc -j8
mkdir -p /tmp/jacclc-lib && cp /tmp/mlxc-bench-build/libjacclc.dylib \
  /tmp/mlxc-bench-build/jaccl/libjaccl.dylib /tmp/jacclc-lib/

for impl in native jacclc; do
  MLX_C_JACCL_BENCH_IMPL=$impl MLX_C_JACCLC_TEST_LIB=/tmp/jacclc-lib \
    go test -run '^$' -bench BenchmarkCompare -benchmem -count=6 \
    ./internal/jacclc/ | grep '^Benchmark' | sed 's/BenchmarkCompare/Benchmark/' > /tmp/bench-$impl.txt
done
benchstat /tmp/bench-native.txt /tmp/bench-jacclc.txt
```
