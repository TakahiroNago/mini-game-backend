#!/bin/sh
# Builds the demo bundle that install.sh consumes:
#
#   deploy/vps/build-bundle.sh <amd64|arm64> [version]
#
# Output: dist/minigame-demo-linux-<arch>.tar.gz containing the two static
# binaries, both UIs built in demo mode and the files of this directory.
# Needs Go and Node; nothing is compiled on the target host.
set -eu

arch=${1:?usage: build-bundle.sh <amd64|arm64> [version]}
version=${2:-dev}
root=$(cd "$(dirname "$0")/../.." && pwd)
out=dist/minigame-demo # relative on purpose: also works under Git Bash on Windows

cd "$root"
# shellcheck disable=SC1091
. deploy/vps/demo.env

rm -rf "$out"
mkdir -p "$out/bin" "$out/web"

CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
    -ldflags="-s -w -X main.version=${version}" -o "$out/bin/api" ./cmd/api
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
    -ldflags="-s -w" -o "$out/bin/migrate" ./cmd/migrate

# Player UI at /, admin UI under /admin/ on the same origin.
(
    cd frontend/player
    [ -d node_modules ] || npm ci --no-audit --no-fund
    VITE_DEMO_MODE=true VITE_ADMIN_URL=/admin/ npm run build
)
(
    cd frontend/admin
    [ -d node_modules ] || npm ci --no-audit --no-fund
    VITE_DEMO_ADMIN_EMAIL="$DEMO_ADMIN_EMAIL" VITE_DEMO_ADMIN_PASSWORD="$DEMO_ADMIN_PASSWORD" VITE_PLAYER_URL=/ \
        npm run build -- --base=/admin/
)
cp -r frontend/player/dist "$out/web/player"
cp -r frontend/admin/dist "$out/web/admin"

cp deploy/vps/install.sh deploy/vps/reset.sh deploy/vps/seed.sql deploy/vps/demo.env \
    deploy/vps/nginx.conf.template deploy/vps/minigame-demo.service \
    deploy/vps/minigame-demo-reset.service deploy/vps/minigame-demo-reset.timer "$out/"
chmod +x "$out/install.sh" "$out/reset.sh" "$out/bin/api" "$out/bin/migrate"
echo "$arch" > "$out/ARCH"
echo "$version" > "$out/VERSION"

tar -C dist -czf "dist/minigame-demo-linux-${arch}.tar.gz" minigame-demo
echo "built dist/minigame-demo-linux-${arch}.tar.gz"
