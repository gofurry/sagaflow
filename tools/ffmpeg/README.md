# Local FFmpeg toolchain

SagaFlow invokes `ffmpeg` and `ffprobe` as independent child processes. The
binaries are intentionally ignored by Git so the repository does not carry a
large third-party distribution.

For Windows development:

```powershell
.\tools\ffmpeg\install-windows.ps1
```

This installs the current Gyan FFmpeg release essentials build under
`tools/ffmpeg/windows-x64/`. SagaFlow discovers that directory when running
from the repository. Released desktop packages should place `ffmpeg` and
`ffprobe` beside the SagaFlow executable; that location has the highest
discovery priority.

The downloaded FFmpeg build is a separate GPLv3 program. Keep its license and
source/build attribution with any redistributed desktop package. SagaFlow
does not link to FFmpeg libraries and communicates with the executable only
through files and process pipes.
