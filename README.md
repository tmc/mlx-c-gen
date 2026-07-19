# MLX C

MLX C is a C API for [MLX](https://github.com/ml-explore/mlx).

MLX is an array framework for machine learning on Apple silicon. MLX C expands
MLX to the C language, making research and experimentation easier on Apple
silicon.

MLX C can be used standalone or as a bridge to bind other languages to
MLX. For example, the [MLX Swift](https://github.com/ml-explore/mlx-swift/)
package uses MLX C to provide a Swift API to MLX.

For more information see the [docs](https://ml-explore.github.io/mlx-c).

## Install

CMake is required to build MLX C. You can install it with [Homebrew](https://brew.sh/):

```shell
brew install cmake
```

To build, run the following commands:

```shell
cmake -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build -j
```

If you have `sccache` or `ccache` installed, CMake uses it to reuse compiled C
and C++ objects across build trees. Pass `-DMLX_C_COMPILER_LAUNCHER=` to
disable the compiler launcher.

From the `build/` directory, you can run an [example](examples/example.c)
that uses MLX C with `./example`.

## Regenerating Bindings

To update the checked-in generated files, run:

```shell
tools/mlx-c-regen --mlx-src=build/_deps/mlx-src
```

`tools/mlx-c-regen` writes the header-driven bindings, standalone support
types, and custom-spec headers such as `mlx/c/jaccl.h` using the repo manifest.
Set `MLX_C_SRC` instead of passing `--mlx-src` if the MLX checkout lives outside
the build tree. When the MLX source path is `build/_deps/mlx-src`,
`tools/mlx-c-regen` also passes `build/compile_commands.json` to the parser if
the file exists. Set `MLX_C_COMPILE_COMMANDS` to override that path.

To verify generated files without updating them, use the cached wrapper instead
of `go run`:

```shell
tools/mlx-c-check-generated --mlx-src=build/_deps/mlx-src
```

This is the CI drift-check command for generated bindings. It regenerates into
a scratch tree and verifies the checked-in generated files and API lock.

Installed headers include `mlx/c/config.h`. It defines `MLX_C_HAS_JACCL` as
`1` when the package includes the standalone JACCL C API and `0` otherwise.
When JACCL is built by the backing MLX library but the C API is disabled, the
install still includes the backing `libjaccl` runtime dependency, but not
`libjacclc` or `mlx/c/jaccl.h`.

The wrapper caches the `mlx-c-gen` binary under the user cache directory and
invalidates it when the generator source or Go build settings change. Other Go
tools can use the same binary cache:

```shell
tools/mlx-c-tool-cached mlx-c-plan-check --types=codegen/types.yaml
```

The generator also caches parsed clang ASTs and clang-format output there by
default. Use `MLX_C_TOOL_CACHE`, `MLX_C_GEN_CACHE`, `MLX_C_AST_CACHE`, or
`MLX_C_FORMAT_CACHE` to override those locations. Use
`MLX_C_COMPILE_COMMANDS` to override the parser compile database.

### Generated-source release candidates

`codegen/manifest.yaml` records both the reviewed MLX ref and its
`release_revision`. The stable identity is
`mlx-v<core-version>-rev<revision>`. A new core starts at revision 1; a changed
generator or binding cut for the same core increments the revision. Preview
identities add `-dev.<commit12>`.

The `Generate candidate` workflow accepts an MLX branch, tag, or commit. It
resolves that input to a commit, generates against the exact checkout, requires
the regeneration report to match the reviewed ref, and opens or updates
`build/<identity>` as a normal review PR. It does not compile libraries, create
tags, or publish releases. Revision-policy CI rejects reused or decreasing
revision numbers.

To verify shared-library exports, build shared targets and pass the produced
libraries to the same cached check:

```shell
cmake -B build-shared -DCMAKE_BUILD_TYPE=RelWithDebInfo -DBUILD_SHARED_LIBS=ON
cmake --build build-shared --target mlxc jacclc -j
tools/mlx-c-check-generated --mlx-src=build-shared/_deps/mlx-src \
  --symbol mlxc=build-shared/libmlxc.dylib \
  --symbol jacclc=build-shared/libjacclc.dylib
```

For the repeated JACCL C API loop, this repository also has a cached end-to-end
check:

```shell
tools/mlx-c-jaccl-check-cached
```

It verifies parser determinism, generated-file drift, the generated C API lock
translation unit, shared-library symbols, all checked-in examples, the JACCL
example at runtime, the JACCL-enabled installed CMake consumer, and the
JACCL-disabled installed CMake consumer. It also builds a JACCL-required
consumer when JACCL is enabled and checks that the same consumer fails at
configure time with `MLXC package was built without JACCL` when JACCL is
disabled. The command reuses shared CMake build trees, the generator binary
cache, parsed ASTs, clang-format output, install-smoke work directories, and a
successful gate result when the full input key is unchanged. Set
`MLX_C_CHECK_CACHE`, `MLX_C_CHECK_BUILD_DIR`,
`MLX_C_CHECK_NO_JACCL_BUILD_DIR`, `MLX_C_CHECK_WORK_DIR`,
`MLX_C_CHECK_NO_JACCL_WORK_DIR`, `MLX_C_CHECK_JACCL_REQUIRED_WORK_DIR`,
`MLX_C_CHECK_NO_JACCL_REQUIRED_WORK_DIR`, `MLX_C_CHECK_PARSE_DIR`,
`MLX_C_CHECK_AST_CACHE`, `MLX_C_CHECK_COMPILE_COMMANDS`,
`MLX_C_CHECK_LOCK_OBJ`, `MLX_C_CHECK_REPORT`, `MLX_C_CHECK_CC`,
`MLX_C_CHECK_COMPILER_LAUNCHER`, or `MLX_C_SRC` to override the defaults.
`MLX_C_CHECK_COMPILER_LAUNCHER` defaults to `auto`; set it to empty to disable
the compiler launcher for both the CMake build and lock translation-unit
compile. Set `MLX_C_CHECK_FORCE=1` for a live rerun, or
`MLX_C_CHECK_RESULT_CACHE=0` to disable result caching.

## Contributing

Check out the [contribution guidelines](CONTRIBUTING.md) for more information
on contributing to MLX C. See the
[docs](https://ml-explore.github.io/mlx/build/html/install.html) for more
information on building from source, and running tests.

We are grateful for all of [our
contributors](ACKNOWLEDGMENTS.md#Individual-Contributors). If you contribute
to MLX C and wish to be acknowledged, please add your name to the list in your
pull request.
