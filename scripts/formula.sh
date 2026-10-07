#!/bin/sh
# formula.sh VERSION SUMS_FILE > phaethon.rb
#
# The Homebrew formula for a phaethon release, for the tap
# (justin06lee/homebrew-tap). VERSION is the release, with or without its v;
# SUMS_FILE is the SHA256SUMS published with it, which must name a tarball for
# each platform. The formula installs the release binaries rather than
# building: phaethon carries bangboo and hollow, and building it means
# building them too.
set -eu

if [ $# -ne 2 ]; then
	echo "usage: $0 VERSION SUMS_FILE" >&2
	exit 2
fi
version=${1#v}
sums=$2
url="https://github.com/justin06lee/phaethon/releases/download/v$version"

# sha PLATFORM: the checksum SUMS_FILE gives that platform's tarball. sha256sum
# marks a file read in binary mode with a *; either way it is the same file.
sha() {
	s=$(awk -v f="phaethon-$1.tar.gz" '$2 == f || $2 == "*" f { print $1; exit }' "$sums")
	if [ -z "$s" ]; then
		echo "formula.sh: $sums has no checksum for phaethon-$1.tar.gz" >&2
		exit 1
	fi
	echo "$s"
}

darwin_arm64=$(sha darwin-arm64)
darwin_amd64=$(sha darwin-amd64)
linux_arm64=$(sha linux-arm64)
linux_amd64=$(sha linux-amd64)

cat <<RUBY
class Phaethon < Formula
  desc "Computers for every agent on this machine"
  homepage "https://github.com/justin06lee/phaethon"
  license "MIT"

  on_macos do
    on_arm do
      url "$url/phaethon-darwin-arm64.tar.gz"
      sha256 "$darwin_arm64"
    end
    on_intel do
      url "$url/phaethon-darwin-amd64.tar.gz"
      sha256 "$darwin_amd64"
    end
  end

  on_linux do
    on_arm do
      url "$url/phaethon-linux-arm64.tar.gz"
      sha256 "$linux_arm64"
    end
    on_intel do
      url "$url/phaethon-linux-amd64.tar.gz"
      sha256 "$linux_amd64"
    end
  end

  def install
    bin.install "phaethon"
  end

  def caveats
    <<~EOS
      phaethon carries bangboo and hollow. To install them and register bangboo
      with every agent harness on this machine, run:
        phaethon install
    EOS
  end

  test do
    assert_match "phaethon v#{version}", shell_output("#{bin}/phaethon version")
  end
end
RUBY
