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

To verify shared-library exports, build shared targets and pass the produced
libraries to the cached generator check:

.. code-block:: shell

  cmake -B build-shared -DCMAKE_BUILD_TYPE=RelWithDebInfo -DBUILD_SHARED_LIBS=ON
  cmake --build build-shared --target mlxc jacclc -j
  tools/mlx-c-gen-cached check --mlx-src=build-shared/_deps/mlx-src \
    --symbol mlxc=build-shared/libmlxc.dylib \
    --symbol jacclc=build-shared/libjacclc.dylib

For repeated JACCL C API work, use the cached end-to-end check:

.. code-block:: shell

  tools/mlx-c-jaccl-check-cached

It verifies parser determinism, generated-file drift, the generated C API lock
translation unit, shared-library symbols, and the installed CMake consumer while
reusing a shared CMake build tree, the generator binary cache, parsed ASTs,
clang-format output, and the install-smoke work directory. Set
``MLX_C_CHECK_CACHE``, ``MLX_C_CHECK_BUILD_DIR``, ``MLX_C_CHECK_WORK_DIR``,
``MLX_C_CHECK_PARSE_DIR``, ``MLX_C_CHECK_AST_CACHE``,
``MLX_C_CHECK_LOCK_OBJ``, ``MLX_C_CHECK_REPORT``, or ``MLX_C_SRC`` to
override the defaults.
