#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════════════════════════
#  GO BUILD SCRIPT (cross-compiling: one Linux job builds both targets)
#
#  Three zones below:
#    🔒 LAUNCHER CONTRACT   — do not edit, the CI workflow depends on this
#    ⚙️  PROJECT CONFIG      — fill in for your project
#    🔨 BUILD STEPS         — edit / add / remove freely
#
#  See BUILD_CONTRACT.md at the repo root for the full contract.
# ═══════════════════════════════════════════════════════════════════════════════
set -euo pipefail

# ╔═══════════════════════════════════════════════════════════════════════════╗
# ║ 🔒  LAUNCHER CONTRACT — DO NOT EDIT                                       ║
# ╚═══════════════════════════════════════════════════════════════════════════╝
APP_VERSION="${APP_VERSION:-dev-local}"
APP_NAME="${APP_NAME:-TDAManager}"
OUTPUT_DIR="../dist"
CODE_DIR="code"

if [ ! -d "$CODE_DIR" ]; then
  echo "ERROR: expected project source code in ./$CODE_DIR"
  exit 1
fi

mkdir -p "dist"
cd "$CODE_DIR"
BUILD_LINUX="${BUILD_LINUX:-1}"
BUILD_WINDOWS="${BUILD_WINDOWS:-1}"

command -v go >/dev/null 2>&1 || { echo "ERROR: go no instalado"; exit 1; }

export GOPATH="${GOPATH:-$PWD/.go}"
export GOMODCACHE="${GOMODCACHE:-$GOPATH/pkg/mod}"
export GOCACHE="${GOCACHE:-$PWD/.cache/go-build}"
mkdir -p "$OUTPUT_DIR" "$GOMODCACHE" "$GOCACHE"


# ╔═══════════════════════════════════════════════════════════════════════════╗
# ║ ⚙️   PROJECT CONFIG — FILL IN                                              ║
# ╚═══════════════════════════════════════════════════════════════════════════╝

# Linker flags. -s -w strips DWARF/symbols; the -X injects APP_VERSION into a
# variable named main.Version. Rename if your version variable lives elsewhere.
LDFLAGS="-s -w -X main.Version=${APP_VERSION}"

# Extra `go build` flags. Add tags, race detector, custom -gcflags here.
EXTRA_BUILD_FLAGS=(-trimpath)
# Example: EXTRA_BUILD_FLAGS+=(-tags "release netgo")

# CGO is off by default for fully static binaries. Set to 1 if you link against C.
CGO_ENABLED_VALUE="${CGO_ENABLED:-0}"

# Package to build. "." = main package at repo root. Change to "./cmd/foo" for
# multi-binary repos.
BUILD_PACKAGE="."


# ╔═══════════════════════════════════════════════════════════════════════════╗
# ║ 🔨  BUILD STEPS — EDIT / ADD / REMOVE FREELY                               ║
# ╚═══════════════════════════════════════════════════════════════════════════╝

# ─── Linux build (delete this block if you don't ship Linux) ───────────────
if [ "$BUILD_LINUX" = "1" ]; then
  CGO_ENABLED="$CGO_ENABLED_VALUE" GOOS=linux GOARCH=amd64 \
    go build "${EXTRA_BUILD_FLAGS[@]}" -ldflags "$LDFLAGS" \
    -o "$OUTPUT_DIR/$APP_NAME" "$BUILD_PACKAGE"
fi

# ─── Windows build (delete this block if you don't ship Windows) ───────────
if [ "$BUILD_WINDOWS" = "1" ]; then
  CGO_ENABLED="$CGO_ENABLED_VALUE" GOOS=windows GOARCH=amd64 \
    go build "${EXTRA_BUILD_FLAGS[@]}" -ldflags "$LDFLAGS" \
    -o "$OUTPUT_DIR/$APP_NAME.exe" "$BUILD_PACKAGE"
fi


# ╔═══════════════════════════════════════════════════════════════════════════╗
# ║ 🔒  LAUNCHER CONTRACT — DO NOT EDIT                                       ║
# ╚═══════════════════════════════════════════════════════════════════════════╝
[ "$BUILD_LINUX"   = "1" ] && { [ -f "$OUTPUT_DIR/$APP_NAME"     ] || { echo "ERROR: falta ELF Linux en $OUTPUT_DIR/";  exit 1; }; }
[ "$BUILD_WINDOWS" = "1" ] && { [ -f "$OUTPUT_DIR/$APP_NAME.exe" ] || { echo "ERROR: falta EXE Windows en $OUTPUT_DIR/"; exit 1; }; }
echo "[+] Artifacts generados en $OUTPUT_DIR/"
