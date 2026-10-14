#!/bin/sh
# Installs caveira, the terminal client, on macOS or Linux:
#
#   curl -fsSL https://caveira.ai/install.sh | sh
#
# It downloads the latest release built for this machine from GitHub,
# checks it against the release's checksums, and puts `caveira` in
# ~/.local/bin with a `cav` link beside it. Run again, it updates.
#
#   CAVEIRA_VERSION=v0.2.0       that release instead of the latest
#   CAVEIRA_INSTALL_DIR=<dir>    somewhere other than ~/.local/bin
#   CAVEIRA_DOWNLOAD_URL=<url>   a mirror of a release's files
set -eu

REPO=justin06lee/caveira
DIR=${CAVEIRA_INSTALL_DIR:-$HOME/.local/bin}

fail() {
	printf 'caveira: %s\n' "$*" >&2
	exit 1
}

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "caveira runs on macOS and Linux, not $(uname -s)" ;;
esac
case $(uname -m) in
arm64 | aarch64) arch=arm64 ;;
x86_64 | amd64) arch=amd64 ;;
*) fail "there is no caveira built for $(uname -m)" ;;
esac
# A shell under Rosetta on Apple silicon still gets the native build.
if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || true)" = 1 ]; then
	arch=arm64
fi

if [ -n "${CAVEIRA_DOWNLOAD_URL:-}" ]; then
	base=${CAVEIRA_DOWNLOAD_URL%/}
elif [ -n "${CAVEIRA_VERSION:-}" ]; then
	base=https://github.com/$REPO/releases/download/$CAVEIRA_VERSION
else
	base=https://github.com/$REPO/releases/latest/download
fi
asset=caveira_${os}_${arch}.tar.gz

command -v curl >/dev/null 2>&1 || fail "installing needs curl"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading caveira for $os/$arch"
curl -fsSL "$base/$asset" -o "$tmp/$asset" || fail "could not download $base/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || fail "could not download $base/checksums.txt"

want=$(awk -v f="$asset" '$2 == f { print $1 }' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
else
	got=$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')
fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then
	fail "$asset does not match the release's checksum; nothing was installed"
fi

tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$DIR"
# The old binary is removed rather than written over, so a session
# running it keeps its file.
rm -f "$DIR/caveira" "$DIR/cav"
cp "$tmp/caveira" "$DIR/caveira"
chmod 0755 "$DIR/caveira"
ln -s caveira "$DIR/cav"

echo "Installed $("$DIR/caveira" --version) in $DIR"
case :$PATH: in
*:"$DIR":*) ;;
*)
	case ${SHELL:-} in
	*/zsh) profile=~/.zshrc ;;
	*/bash) profile=~/.bashrc ;;
	*) profile=~/.profile ;;
	esac
	echo
	echo "$DIR is not on your PATH. Add it with:"
	echo "  echo 'export PATH=\"$DIR:\$PATH\"' >> $profile"
	;;
esac
echo
echo "Run caveira in a project and type /login."
