#!/usr/bin/env sh
# sync-gitee-release.sh — copy a GitHub release's assets onto the Gitee mirror.
#
# Usage:
#   GITEE_TOKEN=... scripts/sync-gitee-release.sh v1.5.15-devwork1
#
# Env:
#   GH_REPO          GitHub owner/name (default: devwork2454/1api)
#   GITEE_OWNER      Gitee owner       (default: wbff)
#   GITEE_REPO       Gitee repo name   (default: 1api)
#   GITEE_TOKEN      Gitee private token with projects write (required)
#   RELEASE_DIR      existing asset dir; if unset, download from GitHub
#
# Never prints token values. Requires: curl, python3.

set -eu

TAG="${1:-}"
[ -n "$TAG" ] || { echo "usage: $0 <tag>" >&2; exit 2; }

GH_REPO="${GH_REPO:-devwork2454/1api}"
GITEE_OWNER="${GITEE_OWNER:-wbff}"
GITEE_REPO="${GITEE_REPO:-1api}"
TOKEN="${GITEE_TOKEN:-}"
[ -n "$TOKEN" ] || { echo "error: GITEE_TOKEN is unset" >&2; exit 2; }

ASSETS="1api_darwin_amd64.tar.gz 1api_darwin_arm64.tar.gz 1api_linux_amd64.tar.gz 1api_linux_arm64.tar.gz checksums.txt install.sh"

info() { printf '==> %s\n' "$1"; }
die() { printf 'error: %s\n' "$1" >&2; exit 1; }

if [ -n "${RELEASE_DIR:-}" ]; then
  dir=$RELEASE_DIR
  [ -d "$dir" ] || die "RELEASE_DIR not a directory: $dir"
else
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' EXIT INT TERM
  if command -v gh >/dev/null 2>&1; then
    info "Downloading GitHub assets for $TAG via gh"
    gh release download "$TAG" --repo "$GH_REPO" --dir "$dir"
  else
    info "Downloading GitHub assets for $TAG via curl"
    for f in $ASSETS; do
      url="https://github.com/${GH_REPO}/releases/download/${TAG}/${f}"
      info "  $f"
      curl -fL --retry 3 --retry-delay 2 --connect-timeout 15 --max-time 300 "$url" -o "$dir/$f"
    done
  fi
fi

for f in $ASSETS; do
  [ -s "$dir/$f" ] || die "missing or empty asset: $f"
done

info "Ensuring Gitee release $TAG"
# Look up existing release by tag (list, then match). /releases/tags/{tag} is not
# always present on Gitee; fall back to creating.
rel_json=$(curl -sS -G \
  --data-urlencode "access_token=${TOKEN}" \
  --data-urlencode "per_page=20" \
  "https://gitee.com/api/v5/repos/${GITEE_OWNER}/${GITEE_REPO}/releases") || die "list Gitee releases failed"

rel_id=$(printf '%s' "$rel_json" | TAG="$TAG" python3 -c '
import json, os, sys
tag = os.environ["TAG"]
data = json.load(sys.stdin)
for r in data:
    if r.get("tag_name") == tag:
        print(r.get("id", ""))
        break
')

if [ -z "$rel_id" ]; then
  info "Creating Gitee release $TAG"
  rel_id=$(curl -sS -X POST \
    --data-urlencode "access_token=${TOKEN}" \
    --data-urlencode "tag_name=${TAG}" \
    --data-urlencode "name=${TAG}" \
    --data-urlencode "body=Mirror of GitHub ${TAG}" \
    --data-urlencode "target_commitish=${TAG}" \
    "https://gitee.com/api/v5/repos/${GITEE_OWNER}/${GITEE_REPO}/releases" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin).get("id",""))')
  [ -n "$rel_id" ] || die "create Gitee release returned no id"
fi
info "Gitee release id=$rel_id"

existing=$(curl -sS -G \
  --data-urlencode "access_token=${TOKEN}" \
  "https://gitee.com/api/v5/repos/${GITEE_OWNER}/${GITEE_REPO}/releases/${rel_id}/attach_files" \
  | python3 -c 'import json,sys; print(" ".join(a.get("name","") for a in json.load(sys.stdin)))' \
  2>/dev/null || true)

for f in $ASSETS; do
  case " $existing " in
    *" $f "*) info "skip existing $f"; continue ;;
  esac
  info "Uploading $f"
  # Multipart; token as form field so it never lands in the URL.
  out=$(curl -sS -X POST \
    -F "access_token=${TOKEN}" \
    -F "file=@${dir}/${f}" \
    "https://gitee.com/api/v5/repos/${GITEE_OWNER}/${GITEE_REPO}/releases/${rel_id}/attach_files") || die "upload $f failed"
  echo "$out" | python3 -c 'import json,sys
try:
    d=json.load(sys.stdin)
except Exception:
    sys.exit("upload response not JSON")
name=d.get("name") or d.get("file_name") or ""
if not name:
    print(d, file=sys.stderr)
    sys.exit("upload response missing name")
print("  uploaded", name)
'
done

latest=$(curl -sS -G --data-urlencode "access_token=${TOKEN}" \
  "https://gitee.com/api/v5/repos/${GITEE_OWNER}/${GITEE_REPO}/releases/latest" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("tag_name",""))')
info "Gitee latest=$latest"
[ "$latest" = "$TAG" ] || die "Gitee latest is $latest, want $TAG"
info "OK $TAG synced to gitee.com/${GITEE_OWNER}/${GITEE_REPO}"
