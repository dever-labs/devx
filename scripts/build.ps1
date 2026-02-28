$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path dist | Out-Null

$ldflags = "-s -w"
if ($env:VERSION) { $ldflags += " -X main.version=$env:VERSION" }

$targets = @(
    @{ GOOS="linux";   GOARCH="amd64"; Out="dist/devx-linux-amd64"       },
    @{ GOOS="linux";   GOARCH="arm64"; Out="dist/devx-linux-arm64"       },
    @{ GOOS="darwin";  GOARCH="amd64"; Out="dist/devx-darwin-amd64"      },
    @{ GOOS="darwin";  GOARCH="arm64"; Out="dist/devx-darwin-arm64"      },
    @{ GOOS="windows"; GOARCH="amd64"; Out="dist/devx-windows-amd64.exe" },
    @{ GOOS="windows"; GOARCH="arm64"; Out="dist/devx-windows-arm64.exe" }
)

foreach ($t in $targets) {
    $env:GOOS = $t.GOOS; $env:GOARCH = $t.GOARCH
    Write-Host "Building $($t.Out)..."
    go build -ldflags $ldflags -o $t.Out ./cmd/devx
}

Remove-Item Env:GOOS, Env:GOARCH
Write-Host "Done. Artifacts in dist/"
Get-ChildItem dist/ | Format-Table Name, Length
