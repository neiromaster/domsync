#!/usr/bin/env bash
set -eu

export GH_TOKEN="$GITHUB_TOKEN"
TAG="${GITHUB_REF_NAME:?GITHUB_REF_NAME is not set}"
VERSION="${TAG#v}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

provenance=false
publish_args=(--access public)
if [ -n "${GITHUB_ACTIONS:-}" ]; then
  provenance=true
  publish_args+=(--provenance)
fi

for pair in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  goos="${pair%/*}"
  goarch="${pair#*/}"

  cpu=arm64
  if [ "$goarch" = amd64 ]; then
    cpu=x64
  fi
  os="$goos"
  if [ "$goos" = windows ]; then
    os=win32
  fi
  platform="$os-$cpu"

  if npm view "@domsync/$platform@$VERSION" version >/dev/null 2>&1; then
    echo "skip $platform (already published)"
    continue
  fi

  ext=tar.gz
  bin=domsync
  if [ "$goos" = windows ]; then
    ext=zip
    bin=domsync.exe
  fi

  download_dir="$WORK/downloads/$platform"
  pkg_dir="$WORK/packages/$platform"
  mkdir -p "$download_dir" "$pkg_dir"

  asset="domsync_${VERSION}_${goos}_${goarch}.${ext}"
  gh release download "$TAG" --repo "$GITHUB_REPOSITORY" \
    --pattern "$asset" --dir "$download_dir"

  if [ "$goos" = windows ]; then
    unzip -q "$download_dir/$asset" -d "$pkg_dir"
  else
    tar -xzf "$download_dir/$asset" -C "$pkg_dir"
  fi
  [ -f "$pkg_dir/$bin" ]

  printf '{"name":"@domsync/%s","version":"%s"}\n' "$platform" "$VERSION" > "$pkg_dir/package.json"

  (
    cd "$pkg_dir"
    npm pkg set \
      "os=$os" \
      "cpu=$cpu" \
      "files[0]=domsync" \
      "files[1]=domsync.exe" \
      "license=MIT" \
      "repository.type=git" \
      "repository.url=git+https://github.com/neiromaster/domsync.git"
    npm pkg set --json "publishConfig.provenance=$provenance" "publishConfig.access=\"public\""
    npm publish "${publish_args[@]}"
  )
done

launcher_dir="$WORK/packages/domsync"
if npm view "@domsync/cli@$VERSION" version >/dev/null 2>&1; then
  echo "skip @domsync/cli (already published)"
else
  cp -R "$ROOT/npm/domsync" "$launcher_dir"
  (
    cd "$launcher_dir"
    npm pkg set "version=$VERSION"
    npm pkg set --json "publishConfig.provenance=$provenance"
    for platform in darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-arm64 win32-x64; do
      npm pkg set "optionalDependencies.@domsync/$platform=$VERSION"
    done
    npm publish "${publish_args[@]}"
  )
fi
