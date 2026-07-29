[CmdletBinding()]
param(
    [string]$InstallDirectory = ""
)

$ErrorActionPreference = "Stop"
$toolchainVersion = "20260616"
$archiveName = "llvm-mingw-$toolchainVersion-ucrt-aarch64.zip"
$archiveSHA256 = "312593669435bd0bfc1a43ac3fba23c8b27e0610bade88b2738e5a01702a99ba"
$downloadURL = "https://github.com/mstorsjo/llvm-mingw/releases/download/$toolchainVersion/$archiveName"

$hostOS = (& go env GOHOSTOS).Trim()
$hostArch = (& go env GOHOSTARCH).Trim()
if ($hostOS -ne "windows" -or $hostArch -ne "arm64") {
    throw "The Windows ARM64 CGO toolchain setup must run on a native windows/arm64 runner; detected $hostOS/$hostArch"
}
if ([string]::IsNullOrWhiteSpace($env:RUNNER_TEMP)) {
    throw "RUNNER_TEMP is not set"
}
if ([string]::IsNullOrWhiteSpace($env:GITHUB_PATH) -or [string]::IsNullOrWhiteSpace($env:GITHUB_ENV)) {
    throw "GITHUB_PATH and GITHUB_ENV are required"
}

$runnerTemp = [System.IO.Path]::GetFullPath($env:RUNNER_TEMP)
if ([string]::IsNullOrWhiteSpace($InstallDirectory)) {
    $InstallDirectory = Join-Path $runnerTemp "llvm-mingw-$toolchainVersion"
}
$installRoot = [System.IO.Path]::GetFullPath($InstallDirectory)
$runnerPrefix = $runnerTemp.TrimEnd(
    [System.IO.Path]::DirectorySeparatorChar,
    [System.IO.Path]::AltDirectorySeparatorChar
) + [System.IO.Path]::DirectorySeparatorChar
if (-not $installRoot.StartsWith($runnerPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Toolchain directory must stay under RUNNER_TEMP: $installRoot"
}

$archivePath = Join-Path $runnerTemp $archiveName
Invoke-WebRequest -Uri $downloadURL -OutFile $archivePath
$actualSHA256 = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualSHA256 -ne $archiveSHA256) {
    throw "llvm-mingw checksum mismatch: expected $archiveSHA256, received $actualSHA256"
}

New-Item -ItemType Directory -Force -Path $installRoot | Out-Null
Expand-Archive -LiteralPath $archivePath -DestinationPath $installRoot -Force
$compilers = @(Get-ChildItem -LiteralPath $installRoot -Recurse -File -Filter "aarch64-w64-mingw32-clang.exe")
if ($compilers.Count -ne 1) {
    throw "Expected one aarch64-w64-mingw32-clang.exe, found $($compilers.Count)"
}
$compilerDirectory = $compilers[0].Directory.FullName
$cxxPath = Join-Path $compilerDirectory "aarch64-w64-mingw32-clang++.exe"
if (-not (Test-Path -LiteralPath $cxxPath -PathType Leaf)) {
    throw "Missing ARM64 C++ compiler: $cxxPath"
}

$compilerDirectory | Out-File -LiteralPath $env:GITHUB_PATH -Encoding utf8 -Append
"CC=aarch64-w64-mingw32-clang" | Out-File -LiteralPath $env:GITHUB_ENV -Encoding utf8 -Append
"CXX=aarch64-w64-mingw32-clang++" | Out-File -LiteralPath $env:GITHUB_ENV -Encoding utf8 -Append

& $compilers[0].FullName --version
Write-Host "Configured llvm-mingw $toolchainVersion for Windows ARM64 CGO"
