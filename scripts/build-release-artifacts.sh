#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <goos> <goarch> <outdir>" >&2
  exit 1
fi

goos="$1"
goarch="$2"
outdir="$3"
binary_name="echo-dust-code"
# Pin gopls so release artifacts stay reproducible across tags.
gopls_module="${GOPLS_MODULE:-golang.org/x/tools/gopls@v0.22.0}"

mkdir -p "$outdir"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

binary_path="$workdir/$binary_name"
gopls_path="$workdir/gopls"
if [[ "$goos" == "windows" ]]; then
  binary_path="${binary_path}.exe"
  gopls_path="${gopls_path}.exe"
fi

CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w" -o "$binary_path" ./cmd/agent
GOBIN="$workdir" CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go install "$gopls_module"

archive_name="${binary_name}-${goos}-${goarch}.tar.gz"
tar -C "$workdir" -czf "$outdir/$archive_name" "$(basename "$binary_path")" "$(basename "$gopls_path")"
