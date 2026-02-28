# Documentation: https://docs.brew.sh/Formula-Cookbook
class Devx < Formula
  desc "Cross-platform dev environment orchestrator"
  homepage "https://github.com/dever-labs/dever"
  version "{{VERSION}}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/dever-labs/dever/releases/download/v#{version}/devx-darwin-arm64"
      sha256 "{{SHA256_DARWIN_ARM64}}"
    else
      url "https://github.com/dever-labs/dever/releases/download/v#{version}/devx-darwin-amd64"
      sha256 "{{SHA256_DARWIN_AMD64}}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/dever-labs/dever/releases/download/v#{version}/devx-linux-arm64"
      sha256 "{{SHA256_LINUX_ARM64}}"
    else
      url "https://github.com/dever-labs/dever/releases/download/v#{version}/devx-linux-amd64"
      sha256 "{{SHA256_LINUX_AMD64}}"
    end
  end

  def install
    os   = OS.mac? ? "darwin" : "linux"
    arch = Hardware::CPU.arm? ? "arm64" : "amd64"
    bin.install "devx-#{os}-#{arch}" => "devx"
  end

  test do
    assert_match "devx v#{version}", shell_output("#{bin}/devx version")
  end
end
