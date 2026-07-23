[CmdletBinding()]
param(
    [string]$DownloadUrl = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip",
    [string]$Proxy = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$target = Join-Path $root "windows-x64"
$archive = Join-Path ([System.IO.Path]::GetTempPath()) "sagaflow-ffmpeg-release-essentials.zip"
$expanded = Join-Path ([System.IO.Path]::GetTempPath()) ("sagaflow-ffmpeg-" + [guid]::NewGuid().ToString("N"))

try {
    $download = @{ Uri = $DownloadUrl; OutFile = $archive }
    if ($Proxy) {
        $download.Proxy = $Proxy
    }
    Write-Host "Downloading FFmpeg release essentials..."
    Invoke-WebRequest @download
    New-Item -ItemType Directory -Force -Path $expanded | Out-Null
    Expand-Archive -LiteralPath $archive -DestinationPath $expanded -Force

    $ffmpeg = Get-ChildItem -LiteralPath $expanded -Recurse -Filter "ffmpeg.exe" | Select-Object -First 1
    $ffprobe = Get-ChildItem -LiteralPath $expanded -Recurse -Filter "ffprobe.exe" | Select-Object -First 1
    if (-not $ffmpeg -or -not $ffprobe) {
        throw "Downloaded archive does not contain ffmpeg.exe and ffprobe.exe"
    }

    New-Item -ItemType Directory -Force -Path $target | Out-Null
    Copy-Item -LiteralPath $ffmpeg.FullName -Destination (Join-Path $target "ffmpeg.exe") -Force
    Copy-Item -LiteralPath $ffprobe.FullName -Destination (Join-Path $target "ffprobe.exe") -Force

    $license = Get-ChildItem -LiteralPath $expanded -Recurse -File |
        Where-Object { $_.Name -match "^(LICENSE|COPYING)(\.txt)?$" } |
        Select-Object -First 1
    if ($license) {
        Copy-Item -LiteralPath $license.FullName -Destination (Join-Path $target "FFMPEG-LICENSE.txt") -Force
    }

    & (Join-Path $target "ffmpeg.exe") -version | Select-Object -First 1
    Write-Host "Installed to $target"
}
finally {
    Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $expanded -Recurse -Force -ErrorAction SilentlyContinue
}
