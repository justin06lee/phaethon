#!/bin/sh
# Install phaethon without cloning or building anything:
#
#   curl -fsSL https://raw.githubusercontent.com/justin06lee/phaethon/master/install.sh | sh
#
# It downloads the release for this machine from GitHub, checks it against
# the SHA256SUMS published with it, and puts phaethon in ~/.local/bin. Then
# `phaethon install` does the rest: phaethon carries bangboo and hollow.
#
#   PHAETHON_VERSION   the release to install (v0.4.0); the latest otherwise
#   PREFIX             install to $PREFIX/bin rather than ~/.local/bin
#   PHAETHON_BASE_URL  where the release files are, rather than GitHub: a
#                      mirror, or a local directory (file:///...) for testing
#
# Everything happens in main, called on the last line, so a download cut off
# part way through runs nothing.

set -eu

repo=https://github.com/justin06lee/phaethon

say() { printf '  %s\n' "$*" >&2; }

die() {
	printf 'phaethon: %s\n' "$*" >&2
	exit 1
}

# platform sets os and arch to the names the release tarballs use.
platform() {
	case $(uname -s) in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) die "phaethon runs on macOS and Linux, and this is $(uname -s)" ;;
	esac
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) die "there is no phaethon for $(uname -m); there is for x86_64 and arm64" ;;
	esac
	# A shell running under Rosetta says x86_64 on an Apple silicon Mac, but
	# the arm64 build is the one that belongs there.
	if [ "$os" = darwin ] && [ "$arch" = amd64 ] &&
		[ "$(sysctl -n sysctl.proc_translated 2>/dev/null || true)" = 1 ]; then
		arch=arm64
	fi
}

# fetch URL FILE
fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1" || die "could not download $1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1" || die "could not download $1"
	else
		die "neither curl nor wget is here to download phaethon with"
	fi
}

# sha256 FILE
sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{ print $1 }'
	else
		die "neither sha256sum nor shasum is here to check the download with"
	fi
}

on_path() {
	case ":$PATH:" in *":$1:"*) return 0 ;; esac
	return 1
}

main() {
	platform
	asset=phaethon-$os-$arch.tar.gz
	version=${PHAETHON_VERSION:-}
	if [ -n "${PHAETHON_BASE_URL:-}" ]; then
		base=${PHAETHON_BASE_URL%/}
	elif [ -n "$version" ]; then
		base=$repo/releases/download/v${version#v}
	else
		base=$repo/releases/latest/download
	fi
	bindir=${PREFIX:-$HOME/.local}/bin

	tmp=$(mktemp -d 2>/dev/null || mktemp -d -t phaethon)
	trap 'rm -rf "$tmp"' EXIT
	trap 'exit 1' INT TERM

	say "downloading $asset${version:+ (v${version#v})}"
	fetch "$base/$asset" "$tmp/$asset"
	fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS"

	# sha256sum marks a file it read in binary mode with a *; either way it
	# is the same file.
	want=$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/SHA256SUMS")
	[ -n "$want" ] || die "the release's SHA256SUMS has no line for $asset"
	got=$(sha256 "$tmp/$asset")
	[ "$got" = "$want" ] || die "$asset does not match the release's SHA256SUMS (got $got, want $want); nothing was installed"

	mkdir -p "$tmp/x"
	tar -xzf "$tmp/$asset" -C "$tmp/x" || die "could not unpack $asset"
	[ -f "$tmp/x/phaethon" ] || die "$asset has no phaethon in it"

	# Put it beside the old one and rename it over, so a phaethon that is
	# running keeps running and the next start gets the new one.
	mkdir -p "$bindir" || die "could not make $bindir"
	cp "$tmp/x/phaethon" "$bindir/.phaethon.new" || die "could not write to $bindir"
	chmod 0755 "$bindir/.phaethon.new"
	mv -f "$bindir/.phaethon.new" "$bindir/phaethon"

	installed=$("$bindir/phaethon" version 2>/dev/null || true)
	say "${installed:-phaethon} is in $bindir"
	if on_path "$bindir"; then
		cmd=phaethon
	else
		cmd=$bindir/phaethon
		say "$bindir is not on your PATH; add it: export PATH=\"$bindir:\$PATH\""
	fi
	echo >&2
	say "next: $cmd install"
	say "      installs bangboo and hollow, and gives every agent harness here bangboo's tools"
}

main "$@"
