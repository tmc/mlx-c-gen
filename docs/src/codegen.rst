Code Generation
===============

MLX C keeps the generator inputs in the repository and treats checked-in C and
C++ files as generated artifacts only when the inventory says they are generated.
The goal is to make regeneration repeatable without hiding policy decisions in
templates.

Sources of Truth
----------------

``codegen/manifest.yaml`` is the top-level generation manifest. It selects the
MLX modules to parse, the standalone support generators, the custom hooks, and
the report gates. It also records the expected upstream MLX git ref so generator
reports can show whether a local checkout matches the release target.

``codegen/modules/*.yaml`` records module-level inputs such as upstream headers,
preinclude headers, documentation paths, and declaration policies. These files
decide which upstream C++ declarations are candidates for generated C wrappers.

``codegen/types.yaml`` records C++ to C type mappings and the reasons unsupported
types are skipped. Missing type coverage is a generator error, not an implicit
skip.

``codegen/custom/*.yaml`` records explicit C APIs that are not discovered from
upstream C++ headers. These specs are used for repetitive declarations where the
public shape should be machine-checked but the implementation may remain
handwritten. ``codegen/custom/jaccl.yaml`` owns the generated JACCL header
``mlx/c/jaccl.h`` and records that ``mlx/c/jaccl.cpp`` is handwritten runtime
code.

``codegen/generated-files.txt`` is the ownership inventory. It classifies each
file as header-driven generated API, generated support code, custom-spec
generated code, handwritten runtime code, or code outside generator ownership.
Regeneration and drift checks use this file to avoid treating handwritten code as
generator output.

``codegen/mlxc-capi.lock.json`` and ``codegen/lock.c`` lock the public C API.
The lock catches unintended surface changes across the generated ``mlxc`` and
``jacclc`` libraries.

What Is Generated
-----------------

The header-driven bindings are generated from upstream MLX C++ headers for the
modules selected by ``codegen/manifest.yaml``. Today that includes ops, linalg,
random, fft, fast, io, compile, transforms, transforms implementation, memory,
metal, cuda, graph utilities, and distributed operations.

Standalone support types such as vectors, closures, and maps are generated from
repo-local generator code rather than discovered from upstream headers.

Custom-spec declarations are generated from repo-local YAML specs. JACCL uses
this path: the public declarations in ``mlx/c/jaccl.h`` come from
``codegen/custom/jaccl.yaml`` while the runtime behavior in ``mlx/c/jaccl.cpp``
stays handwritten.

What Remains Handwritten
------------------------

Behavior-sensitive runtime APIs remain handwritten until their behavior can be
generated mechanically and verified. This includes object lifetimes, error
handling, version queries, streams, devices, arrays, I/O helper types, and the
JACCL implementation. Generator hooks are explicit policy exceptions and their
public names are listed in the manifest so the API lock can check them.

Regeneration Workflow
---------------------

To update checked-in generated files, run:

.. code-block:: shell

  tools/mlx-c-regen --mlx-src=build/_deps/mlx-src

To check for generated-file drift without writing the worktree, run:

.. code-block:: shell

  tools/mlx-c-check-generated --mlx-src=build/_deps/mlx-src

For JACCL C API work, run the cached end-to-end gate:

.. code-block:: shell

  tools/mlx-c-jaccl-check-cached

A safe generator change starts by editing the source of truth: the manifest,
module policy, type map, custom spec, hook policy, or generator code. Then
regenerate, run the drift check, and review the generator report. The report
records generated-file status, API-lock status, ``symbol_checks`` when shared
libraries are provided, missing documentation or type coverage, diagnostic
reasons, input digests, custom specs, and the actual MLX checkout ref.

Public API changes should be deliberate. If the API lock or symbol lock changes,
review the generated diff and update the lock only when the new C surface is the
intended one.
