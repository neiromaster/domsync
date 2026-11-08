#!/bin/sh
set -eu

REPO="neiromaster/domsync"
BASE_URL="https://github.com/${REPO}/releases"

fail() {
  echo "error: $1" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required to download domsync"

os=$(uname -s)
case "$os" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "unsupported operating system: $os" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported architecture: $arch" ;;
esac

if [ -n "${DOMSYNC_VERSION:-}" ]; then
  tag="${DOMSYNC_VERSION#v}"
  tag="v${tag}"
else
  tag=$(curl -fsSI "${BASE_URL}/latest" | tr -d '\r' | grep -i '^location:' | sed 's|^[Ll]ocation: *||' | sed 's|.*/tag/||')
fi
[ -n "$tag" ] || fail "could not resolve the latest release tag; set DOMSYNC_VERSION manually"
version=${tag#v}

tmp=$(mktemp -d "${TMPDIR:-/tmp}/domsync.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP

archive="domsync_${version}_${os}_${arch}"
download_url="${BASE_URL}/download/v${version}"

echo "Downloading domsync ${version} (${os}/${arch})..."
curl -fsSL -o "${tmp}/${archive}.tar.gz" "${download_url}/${archive}.tar.gz"
curl -fsSL -o "${tmp}/checksums.txt" "${download_url}/checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  sum_line=$(grep "  ${archive}.tar.gz\$" "$tmp/checksums.txt") || fail "checksum for ${archive}.tar.gz missing from checksums.txt"
  printf '%s\n' "$sum_line" | (cd "$tmp" && sha256sum -c -)
elif command -v shasum >/dev/null 2>&1; then
  sum_line=$(grep "  ${archive}.tar.gz\$" "$tmp/checksums.txt") || fail "checksum for ${archive}.tar.gz missing from checksums.txt"
  printf '%s\n' "$sum_line" | (cd "$tmp" && shasum -a 256 -c -)
else
  fail "neither sha256sum nor shasum is available; refusing to install unverified artifacts"
fi

tar -xzf "${tmp}/${archive}.tar.gz" -C "$tmp"
if [ ! -f "${tmp}/domsync" ] || [ -L "${tmp}/domsync" ]; then
  fail "domsync in the archive is not a regular file"
fi
chmod +x "${tmp}/domsync"

if command -v cosign >/dev/null 2>&1; then
  curl -fsSL -o "${tmp}/checksums.txt.sigstore.json" "${download_url}/checksums.txt.sigstore.json"
  (cd "$tmp" && cosign verify-blob \
    --bundle checksums.txt.sigstore.json \
    --certificate-identity-regexp "^https://github.com/${REPO}/" \
    --certificate-oidc-issuer-regexp "^https://token\.actions\.githubusercontent\.com$" \
    checksums.txt)
else
  echo "cosign not found; the SHA-256 checksum was verified, but to also verify the"
  echo "Sigstore bundle, install cosign and run:"
  echo "  curl -fsSL -o checksums.txt ${download_url}/checksums.txt"
  echo "  curl -fsSL -o checksums.txt.sigstore.json ${download_url}/checksums.txt.sigstore.json"
  echo "  cosign verify-blob --bundle checksums.txt.sigstore.json --certificate-identity-regexp '^https://github.com/${REPO}/' --certificate-oidc-issuer-regexp '^https://token\.actions\.githubusercontent\.com\$' checksums.txt"
fi

dest="${DOMSYNC_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$dest" 2>/dev/null || fail "cannot create install directory: ${dest}"
[ -w "$dest" ] || fail "${dest} is not writable; set DOMSYNC_INSTALL_DIR to a writable directory (this installer will not use sudo)"

mv -f "${tmp}/domsync" "${dest}/domsync"

echo "installed ${dest}/domsync (${tag})"
case ":${PATH}:" in
  *":${dest}:"*) ;;
  *) echo "note: ${dest} is not in your PATH" ;;
esac
