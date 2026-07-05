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
trap 'chmod -R u+w "$workdir" 2>/dev/null || true; rm -rf "$workdir" 2>/dev/null || true' EXIT

binary_path="$workdir/$binary_name"
gopls_path="$workdir/gopls"
gopath_dir="$workdir/gopath"
if [[ "$goos" == "windows" ]]; then
  binary_path="${binary_path}.exe"
  gopls_path="${gopls_path}.exe"
fi

CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w" -o "$binary_path" ./cmd/agent
GOFLAGS='-modcacherw' GOPATH="$gopath_dir" CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go install "$gopls_module"
installed_gopls="$gopath_dir/bin/$(basename "$gopls_path")"
if [[ ! -f "$installed_gopls" ]]; then
  installed_gopls_alt="$gopath_dir/bin/${goos}_${goarch}/$(basename "$gopls_path")"
  if [[ -f "$installed_gopls_alt" ]]; then
    installed_gopls="$installed_gopls_alt"
  else
    echo "gopls binary not found after go install" >&2
    exit 1
  fi
fi
cp "$installed_gopls" "$gopls_path"

archive_name="${binary_name}-${goos}-${goarch}.tar.gz"
tar -C "$workdir" -czf "$outdir/$archive_name" "$(basename "$binary_path")" "$(basename "$gopls_path")"
