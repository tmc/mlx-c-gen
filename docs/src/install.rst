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
