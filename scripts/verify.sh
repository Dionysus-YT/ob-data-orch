#!/usr/bin/env sh
set -eu

unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
  printf 'Go files need formatting:\n%s\n' "$unformatted" >&2
  exit 1
fi

go test ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...

build_root=$(mktemp -d)
trap 'rm -rf "$build_root"' EXIT INT TERM
for target in windows/amd64 linux/amd64 linux/arm64; do
  target_os=${target%/*}
  target_arch=${target#*/}
  extension=''
  if [ "$target_os" = 'windows' ]; then extension='.exe'; fi
  CGO_ENABLED=0 GOOS=$target_os GOARCH=$target_arch \
    go build -trimpath -o "$build_root/control-plane-$target_os-$target_arch$extension" ./cmd/control-plane
  CGO_ENABLED=0 GOOS=$target_os GOARCH=$target_arch \
    go build -trimpath -o "$build_root/agent-$target_os-$target_arch$extension" ./cmd/agent
done

(
  cd web
  npm ci
  npm run lint
  npm run typecheck
  npm run test
  npm run build
)

"$(dirname "$0")/check-secrets.sh"
