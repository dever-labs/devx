#!/usr/bin/env python3
"""Generate a Homebrew Formula for devx.

Usage:
  generate-formula.py <version> <darwin-arm64-sha> <darwin-amd64-sha> \
                                <linux-arm64-sha>  <linux-amd64-sha>

Outputs the complete formula to stdout so the caller can redirect it to
Formula/devx.rb in the homebrew-tap repository.
"""

import sys

# %(...)s placeholders preserve Ruby's #{version} interpolation verbatim.
FORMULA_TEMPLATE = """\
class Devx < Formula
  desc "Cross-platform dev orchestrator"
  homepage "https://github.com/dever-labs/devx"
  license "MIT"
  version "%(version)s"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/dever-labs/devx/releases/download/v#{version}/devx-darwin-arm64"
      sha256 "%(darwin_arm64_sha)s"
    else
      url "https://github.com/dever-labs/devx/releases/download/v#{version}/devx-darwin-amd64"
      sha256 "%(darwin_amd64_sha)s"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/dever-labs/devx/releases/download/v#{version}/devx-linux-arm64"
      sha256 "%(linux_arm64_sha)s"
    else
      url "https://github.com/dever-labs/devx/releases/download/v#{version}/devx-linux-amd64"
      sha256 "%(linux_amd64_sha)s"
    end
  end

  def install
    os   = OS.mac? ? "darwin" : "linux"
    arch = Hardware::CPU.arm? ? "arm64" : "amd64"
    bin.install "devx-\#{os}-\#{arch}" => "devx"
  end

  test do
    assert_match version.to_s, shell_output("\#{bin}/devx version")
  end
end
"""


def main() -> None:
    if len(sys.argv) != 6:
        print(
            f"Usage: {sys.argv[0]} <version> <darwin-arm64-sha> <darwin-amd64-sha>"
            " <linux-arm64-sha> <linux-amd64-sha>",
            file=sys.stderr,
        )
        sys.exit(1)

    version, darwin_arm64, darwin_amd64, linux_arm64, linux_amd64 = sys.argv[1:]
    print(
        FORMULA_TEMPLATE % {
            "version":         version,
            "darwin_arm64_sha": darwin_arm64,
            "darwin_amd64_sha": darwin_amd64,
            "linux_arm64_sha":  linux_arm64,
            "linux_amd64_sha":  linux_amd64,
        },
        end="",
    )


if __name__ == "__main__":
    main()
