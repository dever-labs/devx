# Install devx for Windows
# Usage: iwr https://raw.githubusercontent.com/dever-labs/dever/main/scripts/install.ps1 | iex
#    or: iwr https://raw.githubusercontent.com/dever-labs/dever/main/scripts/install.ps1 -OutFile install.ps1; .\install.ps1 -Version v1.0.0
param(
    [string]$Version,
    [string]$InstallDir = "$env:LOCALAPPDATA\devx"
)

$ErrorActionPreference = "Stop"
$Repo = "dever-labs/dever"

function Get-LatestVersion {
    $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    return $release.tag_name
}

function Get-Arch {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    if ($arch -eq "X64") { return "amd64" }
    if ($arch -eq "Arm64") { return "arm64" }
    throw "Unsupported architecture: $arch"
}

$ver = if ($Version) { $Version } else { Get-LatestVersion }
$arch = Get-Arch
$asset = "devx-windows-$arch.exe"
$url = "https://github.com/$Repo/releases/download/$ver/$asset"

Write-Host "Downloading devx $ver for windows-$arch..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$dest = Join-Path $InstallDir "devx.exe"
Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing

# Add to user PATH if not already present
$userPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to your PATH (restart your terminal to use devx)"
}

Write-Host "Installed devx $ver to $dest"
& $dest version
