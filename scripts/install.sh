#!/usr/bin/env sh
# install.sh — download a prebuilt 1api binary and put it on your PATH.
#
# Quick install (Linux & macOS, no Go required):
#   # preferred (Gitee; no GitHub connect wait from CN):
#   VERSION=vX.Y.Z sh -c 'curl -fsSL "https://gitee.com/wbff/1api/releases/download/${VERSION}/install.sh" | sh'
#   # or GitHub (may time out from CN):
#   curl -fsSL https://github.com/devwork2454/1api/releases/latest/download/install.sh | sh
#
# Download order: Gitee first; if that fails, fall back to GitHub.
# Override with REPO / GITEE_REPO / VERSION.
#
# Options (environment variables):
#   REPO=owner/name       GitHub repo for release assets (default: devwork2454/1api)
#   GITEE_REPO=owner/name Gitee repo (default: wbff/1api)
#   PREFIX=/usr/local     install under <PREFIX>/bin instead of ~/.local (may need sudo)
#   VERSION=v1.2.3        install a specific release instead of the latest
#
# Requires: curl (or wget), tar, and sha256sum or shasum.

set -eu

REPO="${REPO:-devwork2454/1api}"
# Gitee path may differ from GitHub owner (this fork: wbff/1api).
GITEE_REPO="${GITEE_REPO:-wbff/1api}"
BINARY="1api"
PREFIX="${PREFIX:-$HOME/.local}"
BINDIR="$PREFIX/bin"
VERSION="${VERSION:-latest}"

# --- print banner -------------------------------------------------------------
printf '\033[36mONE API\033[0m\n'
printf '  Ferrying your AI tools between saved profiles\n\n'

info() { printf '\033[36m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[33mwarning:\033[0m %s\n' "$1" >&2; }
die()  { printf '\033[31merror:\033[0m %s\n' "$1" >&2; exit 1; }

# --- pick a downloader -------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
  dl() { curl -fSL --connect-timeout 15 --max-time 300 "$1" -o "$2"; }
  # Quiet body fetch for JSON helpers (stdout only).
  http_get() { curl -fsSL --connect-timeout 10 --max-time 60 "$1"; }
elif command -v wget >/dev/null 2>&1; then
  dl() { wget -qO "$2" --timeout=300 "$1"; }
  http_get() { wget -qO- --timeout=60 "$1"; }
else
  die "need curl or wget to download 1api"
fi
command -v tar >/dev/null 2>&1 || die "tar is required"

# --- detect OS and architecture ---------------------------------------------
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux)  os=linux ;;
  darwin) os=darwin ;;
  *) die "unsupported OS: $os (only linux and darwin have prebuilt binaries)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

info "Detecting platform ... $os ($arch)"

archive="${BINARY}_${os}_${arch}.tar.gz"

# github_release_base: CDN-style release download root for GitHub.
github_release_base() {
  if [ "$VERSION" = "latest" ]; then
    printf 'https://github.com/%s/releases/latest/download' "$REPO"
  else
    printf 'https://github.com/%s/releases/download/%s' "$REPO" "$VERSION"
  fi
}

# resolve_gitee_tag: Gitee has no reliable /latest/download redirect; pin a tag.
# When VERSION=latest, read the newest release tag from Gitee API v5.
resolve_gitee_tag() {
  if [ "$VERSION" != "latest" ]; then
    printf '%s' "$VERSION"
    return 0
  fi
  owner=$(printf '%s' "$GITEE_REPO" | cut -d/ -f1)
  name=$(printf '%s' "$GITEE_REPO" | cut -d/ -f2-)
  # Prefer "tag_name":"v1.2.3" from /releases/latest JSON without requiring jq.
  body=$(http_get "https://gitee.com/api/v5/repos/${owner}/${name}/releases/latest" 2>/dev/null) || return 1
  tag=$(printf '%s' "$body" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)
  [ -n "$tag" ] || return 1
  printf '%s' "$tag"
}

# gitee_release_base: release asset root for Gitee (requires a concrete tag).
gitee_release_base() {
  tag=$(resolve_gitee_tag) || return 1
  printf 'https://gitee.com/%s/releases/download/%s' "$GITEE_REPO" "$tag"
}

# try_download SRC_LABEL BASE DEST — download DEST from BASE/basename(DEST).
try_download() {
  label=$1
  base=$2
  dest=$3
  file=$(basename "$dest")
  info "Trying $label: $base/$file"
  if dl "$base/$file" "$dest"; then
    return 0
  fi
  warn "$label download failed for $file"
  rm -f "$dest"
  return 1
}

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t 1api)
trap 'rm -rf "$tmp"' EXIT INT TERM

info "Downloading $archive ($VERSION) ..."
SOURCE=""
# Gitee first (reachable from CN); GitHub only if Gitee fails.
if gitee_base=$(gitee_release_base) && try_download "Gitee" "$gitee_base" "$tmp/$archive"; then
  SOURCE=gitee
  base=$gitee_base
elif try_download "GitHub" "$(github_release_base)" "$tmp/$archive"; then
  SOURCE=github
  base=$(github_release_base)
else
  die "download failed on Gitee and GitHub — check that $GITEE_REPO (and $REPO) have a release asset for this platform"
fi
info "Using release mirror: $SOURCE"

# --- verify the checksum (best effort; warn if the list is unavailable) ------
if try_download "$SOURCE checksums" "$base" "$tmp/checksums.txt" 2>/dev/null; then
  info "Verifying checksum ..."
  expected=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
  [ -n "$expected" ] || die "no checksum listed for $archive"
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
  else
    warn "no sha256sum/shasum found; skipping checksum verification"
    actual="$expected"
  fi
  if [ "$actual" = "$expected" ]; then
    info "Checksum verified successfully."
  else
    die "checksum mismatch for $archive (expected $expected, got $actual)"
  fi
else
  warn "checksums.txt not available; skipping verification"
fi

# --- unpack and install ------------------------------------------------------
info "Unpacking ..."
tar -xzf "$tmp/$archive" -C "$tmp"
[ -f "$tmp/$BINARY" ] || die "archive did not contain a '$BINARY' binary"

info "Installing to $BINDIR ..."
if ! mkdir -p "$BINDIR" 2>/dev/null || ! install -m 0755 "$tmp/$BINARY" "$BINDIR/$BINARY" 2>/dev/null; then
  warn "could not write to $BINDIR without elevated permissions; retrying with sudo"
  sudo mkdir -p "$BINDIR"
  sudo install -m 0755 "$tmp/$BINARY" "$BINDIR/$BINARY"
fi

info "Installed: $BINDIR/$BINARY"

# --- PATH check --------------------------------------------------------------
info "Checking PATH ..."
case ":$PATH:" in
  *":$BINDIR:"*)
    info "PATH check passed: $BINDIR is already on your PATH."
    ;;
  *)
    warn "$BINDIR is not on your PATH."
    shell_profile="~/.bashrc or ~/.zshrc"
    add_line="export PATH=\"$BINDIR:\$PATH\""
    case "${SHELL:-}" in
      */bash)
        shell_profile="~/.bashrc"
        ;;
      */zsh)
        shell_profile="~/.zshrc"
        ;;
      */fish)
        shell_profile="~/.config/fish/config.fish"
        add_line="fish_add_path $BINDIR"
        ;;
    esac
    printf '  Add this to your shell profile (%s):\n' "$shell_profile"
    printf '    %s\n' "$add_line"
    ;;
esac

printf '\nDone. Run:\n  %s          # interactive menu\n  %s status   # show detected tools\n' "$BINARY" "$BINARY"
