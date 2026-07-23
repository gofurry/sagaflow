[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("windows-amd64", "windows-arm64", "linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64")]
    [string]$Platform,
    [string]$SagaFlowBinary = "",
    [string]$FFmpegDirectory = "",
    [string]$OutputDirectory = ""
)

$ErrorActionPreference = "Stop"
$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$targetIsWindows = $Platform.StartsWith("windows-")
$applicationName = if ($targetIsWindows) { "sagaflow.exe" } else { "sagaflow" }
$ffmpegName = if ($targetIsWindows) { "ffmpeg.exe" } else { "ffmpeg" }
$ffprobeName = if ($targetIsWindows) { "ffprobe.exe" } else { "ffprobe" }

if (-not $SagaFlowBinary) {
    $SagaFlowBinary = Join-Path $repo "bin\$applicationName"
}
if (-not $FFmpegDirectory) {
    $FFmpegDirectory = Join-Path $PSScriptRoot $Platform
}
if (-not $OutputDirectory) {
    $OutputDirectory = Join-Path $repo "dist\sagaflow-$Platform"
}

$required = @(
    $SagaFlowBinary,
    (Join-Path $FFmpegDirectory $ffmpegName),
    (Join-Path $FFmpegDirectory $ffprobeName),
    (Join-Path $FFmpegDirectory "FFMPEG-LICENSE.txt")
)
foreach ($path in $required) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Required release file is missing: $path"
    }
}

New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
Copy-Item -LiteralPath $SagaFlowBinary -Destination (Join-Path $OutputDirectory $applicationName) -Force
Copy-Item -LiteralPath (Join-Path $FFmpegDirectory $ffmpegName) -Destination (Join-Path $OutputDirectory $ffmpegName) -Force
Copy-Item -LiteralPath (Join-Path $FFmpegDirectory $ffprobeName) -Destination (Join-Path $OutputDirectory $ffprobeName) -Force
Copy-Item -LiteralPath (Join-Path $FFmpegDirectory "FFMPEG-LICENSE.txt") -Destination (Join-Path $OutputDirectory "FFMPEG-LICENSE.txt") -Force
Copy-Item -LiteralPath (Join-Path $repo "LICENSE") -Destination (Join-Path $OutputDirectory "SAGAFLOW-LICENSE.txt") -Force
Write-Host "Prepared $Platform release directory at $OutputDirectory"
