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

For repeated generator checks, use the cached wrapper instead of `go run`:

```shell
tools/mlx-c-gen-cached check --mlx-src=build/_deps/mlx-src
```

The wrapper caches the `mlx-c-gen` binary under the user cache directory and
invalidates it when the generator source or Go build settings change. Other Go
tools can use the same binary cache:

```shell
tools/mlx-c-tool-cached mlx-c-plan-check --types=codegen/types.yaml
```

The generator also caches parsed clang ASTs and clang-format output there by
default. Use `MLX_C_TOOL_CACHE`, `MLX_C_GEN_CACHE`, `MLX_C_AST_CACHE`, or
`MLX_C_FORMAT_CACHE` to override those locations.

## Contributing

Check out the [contribution guidelines](CONTRIBUTING.md) for more information
on contributing to MLX C. See the
[docs](https://ml-explore.github.io/mlx/build/html/install.html) for more
information on building from source, and running tests.

We are grateful for all of [our
contributors](ACKNOWLEDGMENTS.md#Individual-Contributors). If you contribute
to MLX C and wish to be acknowledged, please add your name to the list in your
pull request.
