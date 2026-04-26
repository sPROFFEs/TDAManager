#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════════════════════════
#  TDAManager BUILD SCRIPT (Go + Fyne, native dual-runner)
#
#  Fyne requires CGO and links against system OpenGL/X11 libraries — that's why
#  this project uses a native runner per platform instead of a single Linux
#  cross-compile job. The workflow's plan job is configured accordingly
#  (PROJECT_LANG=go is treated as no-crosscompiling here).
#
#  Three zones below:
#    🔒 LAUNCHER CONTRACT   — do not edit, the CI workflow depends on this
#    ⚙️  PROJECT CONFIG      — fill in for your project
#    🔨 BUILD STEPS         — edit / add / remove freely
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
OS_NAME="$(uname -s || echo unknown)"

command -v go >/dev/null 2>&1 || { echo "ERROR: go no instalado"; exit 1; }

export GOPATH="${GOPATH:-$PWD/.go}"
export GOMODCACHE="${GOMODCACHE:-$GOPATH/pkg/mod}"
export GOCACHE="${GOCACHE:-$PWD/.cache/go-build}"
mkdir -p "$OUTPUT_DIR" "$GOMODCACHE" "$GOCACHE"


# ╔═══════════════════════════════════════════════════════════════════════════╗
# ║ ⚙️   PROJECT CONFIG — FILL IN                                              ║
# ╚═══════════════════════════════════════════════════════════════════════════╝

# Entry-point package. The launcher's main lives under cmd/flow-launcher
# (kept that path for now; binary name is APP_NAME above).
BUILD_PACKAGE="./cmd/flow-launcher"

# Linker flags. -H=windowsgui suppresses the console window on Windows.
LDFLAGS_COMMON="-s -w -X main.Version=${APP_VERSION}"
LDFLAGS_WINDOWS="${LDFLAGS_COMMON} -H=windowsgui"

EXTRA_BUILD_FLAGS=(-trimpath -buildvcs=false)

# Windows .exe icon. Place icon.png at the repo root (or assets/icon.png) and
# it will be embedded automatically via go-winres — no extra step needed.


# ╔═══════════════════════════════════════════════════════════════════════════╗
# ║ 🔨  BUILD STEPS — EDIT / ADD / REMOVE FREELY                               ║
# ╚═══════════════════════════════════════════════════════════════════════════╝

LINUX_DONE=0
WINDOWS_DONE=0

# ─── Windows .exe icon embedding (auto, no-op if no icon source found) ──────
ICON_EMBEDDED=0
if [ "$BUILD_WINDOWS" = "1" ]; then
  case "$OS_NAME" in MINGW*|MSYS*|CYGWIN*)
    # Always purge stale syso files first — a leftover from a crashed prior run
    # causes "duplicate leaf" link errors with newer MinGW (15.x+).
    find "$BUILD_PACKAGE" -maxdepth 1 -name "*.syso" -delete 2>/dev/null || true

    PNG_SOURCE=""
    for png in icon.png assets/icon.png; do
      if [ -f "$png" ]; then PNG_SOURCE="$png"; break; fi
    done

    if [ -n "$PNG_SOURCE" ]; then
      if ! command -v go-winres >/dev/null 2>&1; then
        echo "[+] Installing go-winres (pure-Go PNG → .syso embedder)"
        GOBIN="$PWD/.go/bin" go install github.com/tc-hib/go-winres@latest
        export PATH="$PWD/.go/bin:$PATH"
      fi

      if command -v go-winres >/dev/null 2>&1; then
        # go-winres must run inside the package directory so it writes the syso
        # next to the Go source files. The filename it generates
        # (rsrc_windows_amd64.syso) embeds the arch and avoids the i386/x86-64
        # COFF mismatch that rsrc triggers with MinGW 15+.
        ICON_ABS="$(pwd)/$PNG_SOURCE"
        PKG_DIR="$(pwd)/$BUILD_PACKAGE"
        ( cd "$PKG_DIR" && go-winres simply --arch amd64 --icon "$ICON_ABS" )
        WINRES_SYSO="$PKG_DIR/rsrc_windows_amd64.syso"
        trap 'rm -f "'"$WINRES_SYSO"'"' EXIT
        echo "[+] Icon embedded via go-winres → $WINRES_SYSO"
        ICON_EMBEDDED=1
      else
        echo "[!] go-winres install failed, building without embedded icon"
      fi
    else
      echo "[!] No icon source found (icon.png), building without embedded icon"
    fi
    ;;
  esac
fi

# ─── Linux build (only runs on the Linux runner) ───────────────────────────
if [ "$BUILD_LINUX" = "1" ] && [ "$OS_NAME" = "Linux" ]; then
  # Fyne system dependencies. Skip if already present.
  if ! pkg-config --exists gl x11 xcursor xrandr xinerama xi xxf86vm 2>/dev/null; then
    echo "[+] Installing Fyne system dependencies..."
    apt-get update -qq
    apt-get install -y -qq \
      -o Dpkg::Options::=--force-confdef \
      -o Dpkg::Options::=--force-confold \
      gcc pkg-config libgl1-mesa-dev xorg-dev > /dev/null
  fi

  CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build "${EXTRA_BUILD_FLAGS[@]}" -ldflags "$LDFLAGS_COMMON" \
    -o "$OUTPUT_DIR/$APP_NAME" "$BUILD_PACKAGE"
  LINUX_DONE=1
fi

# ─── Windows build (only runs on the Windows runner via Git Bash) ──────────
if [ "$BUILD_WINDOWS" = "1" ]; then
  case "$OS_NAME" in MINGW*|MSYS*|CYGWIN*)
    # The Windows runner ships MinGW; CGO=1 picks up gcc automatically.
    # The launcher embeds a Mesa opengl32.dll for VM/no-GPU environments
    # via cmd/flow-launcher/opengl_windows.go — no extra packaging step needed.
    CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
      go build "${EXTRA_BUILD_FLAGS[@]}" -ldflags "$LDFLAGS_WINDOWS" \
      -o "$OUTPUT_DIR/$APP_NAME.exe" "$BUILD_PACKAGE"
    WINDOWS_DONE=1
    ;;
  esac
fi


# ╔═══════════════════════════════════════════════════════════════════════════╗
# ║ 🔒  LAUNCHER CONTRACT — DO NOT EDIT                                       ║
# ╚═══════════════════════════════════════════════════════════════════════════╝
if [ "$BUILD_LINUX" = "1" ] && [ "$LINUX_DONE" = "1" ]; then
  [ -f "$OUTPUT_DIR/$APP_NAME" ] || { echo "ERROR: falta ELF Linux en $OUTPUT_DIR/"; exit 1; }
fi
if [ "$BUILD_WINDOWS" = "1" ] && [ "$WINDOWS_DONE" = "1" ]; then
  [ -f "$OUTPUT_DIR/$APP_NAME.exe" ] || { echo "ERROR: falta EXE Windows en $OUTPUT_DIR/"; exit 1; }
fi
if [ "$BUILD_LINUX" = "1" ] && [ "$LINUX_DONE" -ne 1 ]; then
  echo "ERROR: BUILD_LINUX=1 pero el job actual no es Linux. Usa runner Linux o BUILD_LINUX=0."
  exit 1
fi
if [ "$BUILD_WINDOWS" = "1" ] && [ "$WINDOWS_DONE" -ne 1 ]; then
  echo "ERROR: BUILD_WINDOWS=1 pero el job actual no es Windows. Fyne no cross-compila de forma fiable."
  exit 1
fi
echo "[+] Artifacts generados en $OUTPUT_DIR/"
