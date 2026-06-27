# Codegen branch map

The MLX-C / JACCL code-generation work is split across three **stacked** branches,
each strictly building on the previous (verified ancestry). The split is by
generation concern. They are cumulative, not independent — tier N requires tier
N-1 to compile.

```
main
 └─ codegen/mlx-c-generator     (188 commits)  concern 1 + 2a
     └─ codegen/jaccl-purego    (+1 commit)     concern 2b
         └─ codegen/jaccl-native (+112 commits) concern 3 (+ jacclc consumer work)
```

## Tier 1 — `codegen/mlx-c-generator` (tip = old `gen-libjaccl`, `3769253`)

The Go MLX C-binding generator **and** the JACCL C API. These two are fused on
purpose, not by accident: the generic custom-spec subsystem
(`internal/mlxcgen/customspec/`) is hard-wired to JACCL as its only fixture —
`header_integration_test.go` loads `codegen/custom/jaccl.yaml` and diffs against
`mlx/c/jaccl.h`, `spec.go` hard-codes `"jacclc"` as an allowed target, and
`jaccl.yaml` is the sole custom spec. A "generic generator only" tier would fail
its own integration tests, so concern 1 and 2a stay together.

- Generic generator: `tools/mlx-c-gen/`, `internal/mlxcgen/` (parser, types,
  variants, hooks, plan, customspec, apilock, regenreport).
- JACCL C API (concern 2a): `codegen/custom/jaccl.yaml` → `mlx/c/jaccl.h`;
  impl `mlx/c/jaccl.cpp` hand-written; CMake `jacclc` target / build gate.
- Builds: `go build ./internal/mlxcgen/... ./tools/mlx-c-gen/`; tests green.

## Tier 2 — `codegen/jaccl-purego` (tip = `ca6e70c`)

Adds the **purego JACCL Go bindings** generator (concern 2b) in a single
self-contained commit. Generates `internal/jacclc/*.gen.go` from the API lock
(`codegen/mlxc-capi.lock.json`) via `tools/mlx-c-gen-purego-jaccl/` +
`internal/mlxcgen/puregojaccl/`. No MLX-binding machinery is touched.

- Builds: `go build ./internal/jacclc/ ./internal/mlxcgen/puregojaccl/`; tests green.

## Tier 3 — `codegen/jaccl-native` (tip = `dcaeb22`, = working branch `gen-jaccl-purego-native`)

Adds the **pure-Go JACCL implementation** `internal/jacclnative` (concern 3) —
hand-written, NOT generated — over `github.com/tmc/apple/rdma`, plus the public
`jaccl/` package and a large amount of `jacclc` consumer hardening/fast-paths.
(Tier name says "native" but it also carries the jacclc consumer work that
landed in the same span; it's the catch-all "everything after purego gen" tier.)

- Native impl: `internal/jacclnative/` (RDMA backend, collectives, the audited
  teardown/correctness fixes), `jaccl/`.
- Needs the uncommitted go.mod `replace github.com/tmc/apple =>
  /Volumes/tmc/go/src/github.com/tmc/apple-rdma` (worktree, branch apple-rdma)
  until apple is tagged.
- Builds (with replace): `go build ./internal/jacclnative/ ./jaccl/`; tests green.

## Not in this repo: concern "tmc/apple native Go RDMA"

apple/rdma is generated in a *separate* repo (`tmc/appledocs` `applegen` →
`tmc/apple-rdma/rdma/*.gen.go`) from txtar templates; it is a generic ibverbs
binding with zero knowledge of JACCL. jacclnative (tier 3) consumes it. See the
memory notes `apple-rdma-codegen-source` and `jacclnative-rdma-teardown`.

## Safety

Pre-split tips tagged `backup/4way-split/<branch>-<stamp>`. The three `codegen/*`
branches were created at existing tier tips with no history rewrite, so they are
bisect-safe by construction; each was verified to build+test at its tip.
