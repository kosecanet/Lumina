#!/bin/sh
# Compila los binarios en dist/ con la versión indicada: ./build.sh v0.3.0   (por defecto "dev")
set -e
VER=${1:-dev}
mkdir -p dist
for t in linux/amd64 linux/arm64 windows/amd64; do
  os=${t%/*}; arch=${t#*/}; ext=""; [ $os = windows ] && ext=".exe"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$VER" -o dist/lumina-$os-$arch$ext .
done
(cd dist && sha256sum lumina-* > checksums.txt)
echo "listo: dist/ ($VER)"
