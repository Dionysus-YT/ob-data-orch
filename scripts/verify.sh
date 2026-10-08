#!/usr/bin/env sh
set -eu

unformatted=$(gofmt -l cmd contracts internal migrations)
if [ -n "$unformatted" ]; then
  printf 'Go files need formatting:\n%s\n' "$unformatted" >&2
  exit 1
fi

go test ./cmd/... ./contracts/... ./internal/... ./migrations/...
go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...

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
  CGO_ENABLED=0 GOOS=$target_os GOARCH=$target_arch \
    go test -c -o "$build_root/migration-test-$target_os-$target_arch$extension" ./internal/migrate
done

(
  cd web
  npm ci
  npm run lint
  npm run audit:wizards
  npm run typecheck
  npm run test
  npm run build
)

"$(dirname "$0")/check-secrets.sh"
