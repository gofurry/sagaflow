[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$PackageDirectory,

    [Parameter(Mandatory = $true)]
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS,

    [string]$OutputDirectory = "release-assets"
)

$ErrorActionPreference = "Stop"
$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path
$packagePath = (Resolve-Path -LiteralPath $PackageDirectory).Path
if (-not [System.IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $repo $OutputDirectory
}
$outputPath = [System.IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $outputPath | Out-Null

$packageName = Split-Path -Leaf $packagePath
if ($TargetOS -eq "windows") {
    $archive = Join-Path $outputPath "$packageName.zip"
    if (Test-Path -LiteralPath $archive) {
        Remove-Item -LiteralPath $archive -Force
    }
    Compress-Archive -LiteralPath $packagePath -DestinationPath $archive -CompressionLevel Optimal
}
else {
    $archive = Join-Path $outputPath "$packageName.tar.gz"
    & tar -czf $archive -C (Split-Path -Parent $packagePath) $packageName
    if ($LASTEXITCODE -ne 0) {
        throw "tar failed for $packageName"
    }
}

Write-Host "Archived $packageName at $archive"
