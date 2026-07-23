[CmdletBinding()]
param(
    [string]$SagaFlowBinary = "",
    [string]$OutputDirectory = ""
)

$ErrorActionPreference = "Stop"
$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$toolDir = Join-Path $PSScriptRoot "windows-x64"
if (-not $SagaFlowBinary) {
    $SagaFlowBinary = Join-Path $repo "bin\sagaflow.exe"
}
if (-not $OutputDirectory) {
    $OutputDirectory = Join-Path $repo "dist\sagaflow-windows-x64"
}

foreach ($path in @(
    $SagaFlowBinary,
    (Join-Path $toolDir "ffmpeg.exe"),
    (Join-Path $toolDir "ffprobe.exe")
)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Required file is missing: $path"
    }
}

New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
Copy-Item -LiteralPath $SagaFlowBinary -Destination (Join-Path $OutputDirectory "sagaflow.exe") -Force
Copy-Item -LiteralPath (Join-Path $toolDir "ffmpeg.exe") -Destination (Join-Path $OutputDirectory "ffmpeg.exe") -Force
Copy-Item -LiteralPath (Join-Path $toolDir "ffprobe.exe") -Destination (Join-Path $OutputDirectory "ffprobe.exe") -Force
if (Test-Path -LiteralPath (Join-Path $toolDir "FFMPEG-LICENSE.txt")) {
    Copy-Item -LiteralPath (Join-Path $toolDir "FFMPEG-LICENSE.txt") -Destination (Join-Path $OutputDirectory "FFMPEG-LICENSE.txt") -Force
}
Copy-Item -LiteralPath (Join-Path $repo "LICENSE") -Destination (Join-Path $OutputDirectory "SAGAFLOW-LICENSE.txt") -Force
Write-Host "Portable package prepared at $OutputDirectory"
