# Installing devx

devx is available through several package managers and a direct download.

---

## Quick install

**Linux / macOS**
```sh
curl -fsSL https://raw.githubusercontent.com/dever-labs/dever/main/scripts/install.sh | sh
```

**Windows (PowerShell)**
```powershell
iwr https://raw.githubusercontent.com/dever-labs/dever/main/scripts/install.ps1 | iex
```

---

## Manual setup

Download a pre-built binary from [GitHub Releases](https://github.com/dever-labs/dever/releases/latest):

| Platform | File |
|----------|------|
| Linux amd64   | `devx-linux-amd64` |
| Linux arm64   | `devx-linux-arm64` |
| macOS amd64   | `devx-darwin-amd64` |
| macOS arm64   | `devx-darwin-arm64` |
| Windows amd64 | `devx-windows-amd64.exe` |
| Windows arm64 | `devx-windows-arm64.exe` |

SHA-256 checksums are published alongside each release in `checksums.txt`.

### Linux / macOS

```sh
# Replace <version> and <platform> with e.g. v1.0.0 and linux-amd64
curl -Lo devx https://github.com/dever-labs/dever/releases/download/<version>/devx-<platform>
chmod +x devx
sudo mv devx /usr/local/bin/devx
devx version
```

If you don't have `sudo` or prefer a user-local install:

```sh
mkdir -p "$HOME/.local/bin"
mv devx "$HOME/.local/bin/devx"

# Add to PATH (add to ~/.bashrc, ~/.zshrc, etc.)
export PATH="$HOME/.local/bin:$PATH"
```

### Windows

1. Download `devx-windows-amd64.exe` from the [Releases](https://github.com/dever-labs/dever/releases/latest) page.
2. Rename it to `devx.exe` and move it to a folder of your choice, e.g. `C:\Tools\devx\`.
3. Add that folder to your `PATH`:
   - Open **System Properties → Advanced → Environment Variables**.
   - Under **User variables**, select `Path` and click **Edit**.
   - Add a new entry: `C:\Tools\devx`
   - Click **OK** and restart your terminal.
4. Verify:
   ```powershell
   devx version
   ```

Alternatively, from PowerShell (no admin required):

```powershell
$dir = "$env:LOCALAPPDATA\devx"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
# Download
Invoke-WebRequest https://github.com/dever-labs/dever/releases/latest/download/devx-windows-amd64.exe `
    -OutFile "$dir\devx.exe"
# Add to PATH for current user
$path = [System.Environment]::GetEnvironmentVariable("Path","User")
if ($path -notlike "*$dir*") {
    [System.Environment]::SetEnvironmentVariable("Path", "$path;$dir", "User")
}
# Verify (in a new shell)
devx version
```

---

## npm

```sh
npm install -g @dever-labs/devx
devx version
```

The postinstall script downloads the correct binary for your OS and architecture from the GitHub release.

---

## Homebrew

```sh
brew tap dever-labs/tap https://github.com/dever-labs/homebrew-tap
brew install devx
```

To upgrade:
```sh
brew upgrade devx
```

---

## Chocolatey (Windows)

```powershell
choco install devx
```

To upgrade:
```powershell
choco upgrade devx
```

---

## WinGet (Windows)

```powershell
winget install dever-labs.devx
```

To upgrade:
```powershell
winget upgrade dever-labs.devx
```

---

## Build from source

Requires [Go 1.21+](https://go.dev/dl/).

```sh
git clone https://github.com/dever-labs/dever.git
cd dever
go build ./cmd/devx
./devx version
```

For all platforms at once:
```sh
./scripts/build.sh        # Linux/macOS
.\scripts\build.ps1       # Windows PowerShell
```

Binaries are placed in `dist/`.

---

## Verifying checksums

```sh
# Download the binary and checksums file
curl -LO https://github.com/dever-labs/dever/releases/latest/download/devx-linux-amd64
curl -LO https://github.com/dever-labs/dever/releases/latest/download/checksums.txt

# Verify
sha256sum --check --ignore-missing checksums.txt
```

On macOS use `shasum -a 256 --check checksums.txt`.

---

## Package maintainers

### Publishing a new release

1. Tag the commit: `git tag v1.2.3 && git push origin v1.2.3`
2. The [release workflow](../.github/workflows/release.yml) automatically builds all six targets and creates a GitHub Release with binaries + checksums.
3. Update the following manually after a release:
   - **Homebrew tap** — copy `homebrew/devx.rb` to `dever-labs/homebrew-tap`, fill in the new version and SHA-256 hashes.
   - **WinGet** — copy `winget/` manifests to `microsoft/winget-pkgs` under `manifests/d/dever-labs/devx/<version>/`, fill in version and SHA-256 hashes, open a PR.
   - **Chocolatey** — update the version in `chocolatey/devx.nuspec`, run `choco pack`, then `choco push`.
   - **npm** — update `version` in `npm/package.json` and run `npm publish`.
