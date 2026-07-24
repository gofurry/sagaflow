# Workflow scripts

- `build.ps1` builds the six platform/architecture packages used by CI.

Packaging icons live under `packaging/icons/`. Release packages intentionally do
not bundle FFmpeg. At runtime, SagaFlow can discover an existing toolchain or
download the fixed, checksummed package for the current platform into the data
directory.
