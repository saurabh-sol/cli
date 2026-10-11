#!/usr/bin/env sh
# Build GoCode for all supported platforms into ./dist
set -e
cd "$(dirname "$0")"
mkdir -p dist
build() {
  os=$1; arch=$2; ext=$3
  out="dist/gocode-${os}-${arch}${ext}"
  echo "building $out"
  GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$out" .
}
build darwin  arm64 ""
build darwin  amd64 ""
build linux   amd64 ""
build linux   arm64 ""
build windows amd64 .exe
build windows arm64 .exe
echo "done:"; ls -la dist
