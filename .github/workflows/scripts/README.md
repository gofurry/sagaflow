# Workflow scripts

- `build.ps1` builds the six headless core packages with `CGO_ENABLED=0`.
- `build-desktop.ps1` runs on a native desktop runner and packages the Fyne
  launcher beside the matching core binary.
- `archive-package.ps1` produces the `.zip`/`.tar.gz` assets consumed by the
  tag-driven release workflow.

Packaging icons live under `packaging/icons/`. Release packages intentionally do
not bundle FFmpeg. At runtime, SagaFlow can discover an existing toolchain or
download the fixed, checksummed package for the current platform into the data
directory.

The desktop build remains separate because Fyne uses CGo and native graphics
libraries. Docker and server packages never compile or include the launcher.
