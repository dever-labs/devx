# Installing devx

devx is available through several package managers and a direct download.

---

## Quick install

**Linux / macOS**
```sh
curl -fsSL https://raw.githubusercontent.com/dever-labs/devx/main/scripts/install.sh | sh
```

**Windows (PowerShell)**
```powershell
iwr https://raw.githubusercontent.com/dever-labs/devx/main/scripts/install.ps1 | iex
```

---

## Manual setup

Download a pre-built binary from [GitHub Releases](https://github.com/dever-labs/devx/releases/latest):

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
curl -Lo devx https://github.com/dever-labs/devx/releases/download/<version>/devx-<platform>
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

1. Download `devx-windows-amd64.exe` from the [Releases](https://github.com/dever-labs/devx/releases/latest) page.
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
Invoke-WebRequest https://github.com/dever-labs/devx/releases/latest/download/devx-windows-amd64.exe `
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

## Build from source

Requires [Go 1.21+](https://go.dev/dl/).

```sh
git clone https://github.com/dever-labs/devx.git
cd devx
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
curl -LO https://github.com/dever-labs/devx/releases/latest/download/devx-linux-amd64
curl -LO https://github.com/dever-labs/devx/releases/latest/download/checksums.txt

# Verify
sha256sum --check --ignore-missing checksums.txt
```

On macOS use `shasum -a 256 --check checksums.txt`.

---

## Package maintainers

### Publishing a new release

Releases are fully automated:

1. Merge Conventional Commit-style PRs (`feat:`, `fix:`, `fix!:`/`BREAKING CHANGE:`, etc.) to `main`.
2. [release-please](https://github.com/googleapis/release-please) opens/updates a "Release PR" that bumps the version and rewrites `CHANGELOG.md` from those commits.
3. Merging the Release PR makes release-please tag the commit and publish a GitHub Release.
4. That `release: created` event triggers [goreleaser](https://goreleaser.com) (see `.goreleaser.yaml`), which builds all platform binaries, generates `checksums.txt`, attaches them to the release, and pushes an updated formula to `dever-labs/homebrew-tap`.

No manual tagging, changelog editing, or Homebrew formula updates are needed.

