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

$hostOS = (& go env GOOS).Trim()
$hostArch = (& go env GOARCH).Trim()
if ($hostOS -ne $TargetOS -or $hostArch -ne $TargetArch) {
    throw "Fyne desktop builds are native: requested $TargetOS-$TargetArch on $hostOS-$hostArch"
}

if (-not [System.IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $repo $OutputDirectory
}

$platform = "$TargetOS-$TargetArch"
$outputRoot = [System.IO.Path]::GetFullPath($OutputDirectory)
$packageDirectory = [System.IO.Path]::GetFullPath((Join-Path $outputRoot "sagaflow-desktop-$platform"))
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
New-Item -ItemType Directory -Force -Path $packageDirectory | Out-Null

$coreName = if ($TargetOS -eq "windows") { "sagaflow.exe" } else { "sagaflow" }
$desktopName = if ($TargetOS -eq "windows") { "sagaflow-desktop.exe" } else { "sagaflow-desktop" }
$corePath = Join-Path $packageDirectory $coreName
$desktopPath = Join-Path $packageDirectory $desktopName
if ($TargetOS -eq "darwin") {
    $appContents = Join-Path (Join-Path $packageDirectory "SagaFlow.app") "Contents"
    $appMacOS = Join-Path $appContents "MacOS"
    $appResources = Join-Path $appContents "Resources"
    New-Item -ItemType Directory -Force -Path $appMacOS, $appResources | Out-Null
    $corePath = Join-Path $appMacOS "sagaflow"
    $desktopPath = Join-Path $appMacOS "SagaFlow"
}

$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch

    Push-Location $repo
    try {
        $env:CGO_ENABLED = "0"
        & go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $corePath .\cmd\sagaflow
        if ($LASTEXITCODE -ne 0) {
            throw "Core build failed for $platform"
        }
    }
    finally {
        Pop-Location
    }

    Push-Location (Join-Path $repo "cmd\sagaflow-desktop")
    try {
        $env:CGO_ENABLED = "1"
        $resourceFile = ""
        if ($TargetOS -eq "windows") {
            $resourceFile = Join-Path (Get-Location) "rsrc_windows_$TargetArch.syso"
            & go run github.com/akavel/rsrc@v0.10.2 `
                -arch $TargetArch `
                -ico (Join-Path $repo "packaging\icons\windows\sagaflow.ico") `
                -manifest (Join-Path $repo "packaging\windows\sagaflow-desktop.exe.manifest") `
                -o $resourceFile
            if ($LASTEXITCODE -ne 0) {
                throw "Windows resource generation failed for $platform"
            }
        }
        $desktopLinkerFlags = "-s -w -X main.version=$Version"
        if ($TargetOS -eq "windows") {
            $desktopLinkerFlags += " -H=windowsgui"
        }
        & go build -trimpath -ldflags $desktopLinkerFlags -o $desktopPath .
        if ($LASTEXITCODE -ne 0) {
            throw "Desktop build failed for $platform"
        }
    }
    finally {
        if ($resourceFile -and (Test-Path -LiteralPath $resourceFile)) {
            Remove-Item -LiteralPath $resourceFile -Force
        }
        Pop-Location
    }
}
finally {
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    $env:CGO_ENABLED = $previousCGO
}

Copy-Item -LiteralPath (Join-Path $repo "LICENSE") -Destination (Join-Path $packageDirectory "SAGAFLOW-LICENSE.txt") -Force
$iconRoot = Join-Path $repo "packaging\icons"
switch ($TargetOS) {
    "windows" {
        Copy-Item -LiteralPath (Join-Path $iconRoot "windows\sagaflow.ico") -Destination (Join-Path $packageDirectory "sagaflow.ico") -Force
    }
    "darwin" {
        Copy-Item -LiteralPath (Join-Path $iconRoot "macos\sagaflow.icns") -Destination (Join-Path $packageDirectory "sagaflow.icns") -Force
        Copy-Item -LiteralPath (Join-Path $iconRoot "macos\sagaflow.icns") -Destination (Join-Path $appResources "sagaflow.icns") -Force
        $bundleVersion = ($Version -replace "^v", "") -replace "[^0-9.].*$", ""
        if ([string]::IsNullOrWhiteSpace($bundleVersion)) {
            $bundleVersion = "0.1.0"
        }
        $infoPlist = @"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDisplayName</key><string>SagaFlow</string>
    <key>CFBundleExecutable</key><string>SagaFlow</string>
    <key>CFBundleIconFile</key><string>sagaflow</string>
    <key>CFBundleIdentifier</key><string>io.gofurry.sagaflow</string>
    <key>CFBundleName</key><string>SagaFlow</string>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleShortVersionString</key><string>$bundleVersion</string>
    <key>CFBundleVersion</key><string>$bundleVersion</string>
    <key>LSMinimumSystemVersion</key><string>11.0</string>
    <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
"@
        Set-Content -LiteralPath (Join-Path $appContents "Info.plist") -Value $infoPlist -Encoding utf8 -NoNewline
    }
    "linux" {
        $iconsDirectory = Join-Path $packageDirectory "share\icons"
        New-Item -ItemType Directory -Force -Path $iconsDirectory | Out-Null
        Copy-Item -LiteralPath (Join-Path $iconRoot "linux\hicolor") -Destination (Join-Path $iconsDirectory "hicolor") -Recurse -Force
    }
}

Write-Host "Built $platform desktop package at $packageDirectory"
