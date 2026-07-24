[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS,

    [Parameter(Mandatory = $true)]
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch,

    [string]$Version = "v0.1.0",
    [string]$OutputDirectory = "dist"
)

$ErrorActionPreference = "Stop"
$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path

if ($Version -notmatch "^[0-9A-Za-z._+-]+$") {
    throw "Version contains unsupported characters: $Version"
}

if (-not [System.IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $repo $OutputDirectory
}

$platform = "$TargetOS-$TargetArch"
$outputRoot = [System.IO.Path]::GetFullPath($OutputDirectory)
$packageDirectory = [System.IO.Path]::GetFullPath((Join-Path $outputRoot "sagaflow-$platform"))
$outputPrefix = $outputRoot.TrimEnd(
    [System.IO.Path]::DirectorySeparatorChar,
    [System.IO.Path]::AltDirectorySeparatorChar
) + [System.IO.Path]::DirectorySeparatorChar
if (-not $packageDirectory.StartsWith($outputPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Package directory escaped the requested output directory: $packageDirectory"
}
if (Test-Path -LiteralPath $packageDirectory) {
    Remove-Item -LiteralPath $packageDirectory -Recurse -Force
}
$binaryName = if ($TargetOS -eq "windows") { "sagaflow.exe" } else { "sagaflow" }
$binaryPath = Join-Path $packageDirectory $binaryName

New-Item -ItemType Directory -Force -Path $packageDirectory | Out-Null

$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
Push-Location $repo
try {
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = "0"
    & go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $binaryPath .\cmd\sagaflow
    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed for $platform"
    }
}
finally {
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}

Copy-Item -LiteralPath (Join-Path $repo "LICENSE") -Destination (Join-Path $packageDirectory "SAGAFLOW-LICENSE.txt") -Force

$iconRoot = Join-Path $repo "packaging\icons"
switch ($TargetOS) {
    "windows" {
        Copy-Item -LiteralPath (Join-Path $iconRoot "windows\sagaflow.ico") -Destination (Join-Path $packageDirectory "sagaflow.ico") -Force
    }
    "darwin" {
        Copy-Item -LiteralPath (Join-Path $iconRoot "macos\sagaflow.icns") -Destination (Join-Path $packageDirectory "sagaflow.icns") -Force
    }
    "linux" {
        $iconsDirectory = Join-Path $packageDirectory "share\icons"
        New-Item -ItemType Directory -Force -Path $iconsDirectory | Out-Null
        Copy-Item -LiteralPath (Join-Path $iconRoot "linux\hicolor") -Destination (Join-Path $iconsDirectory "hicolor") -Recurse -Force
    }
}

Write-Host "Built $platform package at $packageDirectory"
