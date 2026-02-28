$ErrorActionPreference = "Stop"

$packageVersion = $env:ChocolateyPackageVersion
$packageName    = "devx"
$repo           = "dever-labs/dever"

$arch = if ([System.Environment]::Is64BitOperatingSystem) {
    if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
} else {
    "amd64"
}

$url = "https://github.com/$repo/releases/download/v$packageVersion/devx-windows-$arch.exe"
$checksum = (Invoke-RestMethod "https://github.com/$repo/releases/download/v$packageVersion/checksums.txt") `
    -split "`n" | Where-Object { $_ -match "devx-windows-$arch.exe" } | ForEach-Object { ($_ -split '\s+')[0] }

$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$dest = Join-Path $toolsDir "devx.exe"

Get-ChocolateyWebFile -PackageName $packageName -FileFullPath $dest -Url $url -Checksum $checksum -ChecksumType sha256
