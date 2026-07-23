# FFmpeg toolchain

SagaFlow invokes `ffmpeg` and `ffprobe` as independent child processes. It
does not link against FFmpeg libraries and does not embed a platform-specific
FFmpeg distribution in the application binary.

## Runtime discovery

SagaFlow looks for a matching pair in this order:

1. beside the SagaFlow executable;
2. `tools/ffmpeg/<goos>-<goarch>/` beside the executable;
3. the current working directory;
4. `tools/ffmpeg/<goos>-<goarch>/` in the current repository;
5. the system `PATH`.

Platform directory names follow Go conventions:

```text
windows-amd64
windows-arm64
linux-amd64
linux-arm64
darwin-amd64
darwin-arm64
```

Windows uses `ffmpeg.exe` and `ffprobe.exe`; other systems use `ffmpeg` and
`ffprobe`.

## Local development

Windows amd64 developers can download a Gyan essentials build into the ignored
local tool directory:

```powershell
.\tools\ffmpeg\install-windows.ps1
```

Linux and macOS developers can install FFmpeg through their package manager:

```bash
# Debian / Ubuntu
sudo apt-get install ffmpeg

# macOS
brew install ffmpeg
```

Third-party binaries under the platform directories are intentionally ignored
by Git. The source repository stays architecture-neutral.

## Release packages

Each published platform package should contain a matching toolchain beside the
SagaFlow executable:

```text
sagaflow-<goos>-<goarch>/
├── sagaflow[.exe]
├── ffmpeg[.exe]
├── ffprobe[.exe]
├── SAGAFLOW-LICENSE.txt
└── FFMPEG-LICENSE.txt
```

Prepare a directory with PowerShell:

```powershell
.\tools\ffmpeg\package-release.ps1 -Platform windows-amd64
```

Or with a POSIX shell:

```bash
sh ./tools/ffmpeg/package-release.sh linux-amd64
```

Both scripts also accept explicit SagaFlow binary, FFmpeg directory and output
directory arguments. This lets release automation use any trusted FFmpeg
distribution without changing SagaFlow source code.

FFmpeg/FFprobe remain separate third-party programs. SagaFlow source code is
licensed under MIT; the FFmpeg license depends on the exact build
configuration. Redistributors must preserve that build's license and satisfy
its corresponding source obligations.
