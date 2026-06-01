Building and Installing
=======================

CMake is required to build MLX C. You can install it with `Homebrew <https://brew.sh/>`_:

.. code-block:: shell

  brew install cmake

To build MLX C, run the following commands:

.. code-block:: shell

  cmake -B build -DCMAKE_BUILD_TYPE=Release
  cmake --build build -j

If you have ``sccache`` or ``ccache`` installed, CMake uses it to reuse compiled
C and C++ objects across build trees. Pass ``-DMLX_C_COMPILER_LAUNCHER=`` to
disable the compiler launcher.

MLX C will fetch `MLX <https://github.com/ml-explore/mlx>`_ under the hood,
compile it, and then compile the C API.

To update the checked-in generated files, run:

.. code-block:: shell

  tools/mlx-c-regen --mlx-src=build/_deps/mlx-src

``tools/mlx-c-regen`` writes the header-driven bindings, standalone support
types, and custom-spec headers such as ``mlx/c/jaccl.h`` using the repo
manifest. Set ``MLX_C_SRC`` instead of passing ``--mlx-src`` if the MLX checkout
lives outside the build tree. When the MLX source path is
``build/_deps/mlx-src``, ``tools/mlx-c-regen`` also passes
``build/compile_commands.json`` to the parser if the file exists. Set
``MLX_C_COMPILE_COMMANDS`` to override that path.

To verify generated files without updating them, use the cached wrapper instead
of ``go run``:

.. code-block:: shell

  tools/mlx-c-check-generated --mlx-src=build/_deps/mlx-src

This is the CI drift-check command for generated bindings. It regenerates into
a scratch tree and verifies the checked-in generated files and API lock.

Installed headers include ``mlx/c/config.h``. It defines
``MLX_C_HAS_JACCL`` as ``1`` when the package includes the standalone JACCL C
API and ``0`` otherwise.
When JACCL is built by the backing MLX library but the C API is disabled, the
install still includes the backing ``libjaccl`` runtime dependency, but not
``libjacclc`` or ``mlx/c/jaccl.h``.

To verify shared-library exports, build shared targets and pass the produced
libraries to the cached generator check:

.. code-block:: shell

  cmake -B build-shared -DCMAKE_BUILD_TYPE=RelWithDebInfo -DBUILD_SHARED_LIBS=ON
  cmake --build build-shared --target mlxc jacclc -j
  tools/mlx-c-check-generated --mlx-src=build-shared/_deps/mlx-src \
    --symbol mlxc=build-shared/libmlxc.dylib \
    --symbol jacclc=build-shared/libjacclc.dylib

For repeated JACCL C API work, use the cached end-to-end check:

.. code-block:: shell

  tools/mlx-c-jaccl-check-cached

It verifies parser determinism, generated-file drift, the generated C API lock
translation unit, shared-library symbols, all checked-in examples, the JACCL
example at runtime, the JACCL-enabled installed CMake consumer, and the
JACCL-disabled installed CMake consumer. It also builds a JACCL-required
consumer when JACCL is enabled and checks that the same consumer fails at
configure time with ``MLXC package was built without JACCL`` when JACCL is
disabled. The command reuses shared CMake build trees, the generator binary
cache, parsed ASTs, clang-format output, install-smoke work directories, and a
successful gate result when the full input key is unchanged. Set
``MLX_C_CHECK_CACHE``, ``MLX_C_CHECK_BUILD_DIR``,
``MLX_C_CHECK_NO_JACCL_BUILD_DIR``, ``MLX_C_CHECK_WORK_DIR``,
``MLX_C_CHECK_NO_JACCL_WORK_DIR``,
``MLX_C_CHECK_JACCL_REQUIRED_WORK_DIR``,
``MLX_C_CHECK_NO_JACCL_REQUIRED_WORK_DIR``, ``MLX_C_CHECK_PARSE_DIR``,
``MLX_C_CHECK_AST_CACHE``, ``MLX_C_CHECK_COMPILE_COMMANDS``,
``MLX_C_CHECK_LOCK_OBJ``, ``MLX_C_CHECK_REPORT``, ``MLX_C_CHECK_CC``,
``MLX_C_CHECK_COMPILER_LAUNCHER``, or ``MLX_C_SRC`` to override the defaults.
``MLX_C_CHECK_COMPILER_LAUNCHER`` defaults to ``auto``; set it to empty to
disable the compiler launcher for both the CMake build and lock
translation-unit compile. Set ``MLX_C_CHECK_FORCE=1`` for a live rerun, or
``MLX_C_CHECK_RESULT_CACHE=0`` to disable result caching.
