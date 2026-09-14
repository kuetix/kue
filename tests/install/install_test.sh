#!/bin/bash
# End-to-end test for scripts/install.sh — the public curl-pipeable
# installer. Builds a real kue binary for the host platform, packages it
# exactly like a GoReleaser release would (tar.gz + checksums.txt), serves
# it from a local HTTP server, and runs the real install.sh against it via
# the KUE_INSTALL_API_URL / KUE_INSTALL_BASE_URL test overrides. No network
# access and no real GitHub release required.
#
# Usage: tests/install/install_test.sh   (also wired into `make test-install`)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
INSTALLER="$ROOT/scripts/install.sh"
WORK="$(mktemp -d)"
SERVER_PID=""

cleanup() {
    [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
    rm -rf "$WORK"
}
trap cleanup EXIT

pass=0
fail=0
ok()   { pass=$((pass + 1)); echo "  ok   - $1"; }
bad()  { fail=$((fail + 1)); echo "  FAIL - $1"; }

# --- host OS/arch, same mapping install.sh uses -----------------------

case "$(uname -s)" in
    Linux) os="linux" ;;
    Darwin) os="darwin" ;;
    *) echo "skipping: unsupported test host OS $(uname -s)"; exit 0 ;;
esac
case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) echo "skipping: unsupported test host arch $(uname -m)"; exit 0 ;;
esac

# --- build the real binary + package it like a release -----------------

echo "Building kue for ${os}/${arch}..."
mkdir -p "$WORK/build"
# -mod=vendor: kue resolves sibling workspace modules (engine, std-*) via ../
# replace directives backed by a committed vendor/ tree — same as ci.yml.
( cd "$ROOT" && go build -mod=vendor -o "$WORK/build/kue" ./cmd/cli )

version="0.9.9-test"
tag="v${version}"
good_dir="$WORK/server/download/${tag}"
mkdir -p "$good_dir"
archive_name="kue_${version}_${os}_${arch}.tar.gz"
tar -C "$WORK/build" -czf "$good_dir/$archive_name" kue

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
    else shasum -a 256 "$1" | awk '{print $1}'
    fi
}
echo "$(sha256 "$good_dir/$archive_name")  $archive_name" > "$good_dir/checksums.txt"

mkdir -p "$WORK/server"
cat > "$WORK/server/api.json" <<EOF
{"tag_name": "${tag}"}
EOF

# A second release with a deliberately wrong checksum, to test the failure path.
bad_tag="v0.9.9-badsum"
bad_dir="$WORK/server/download/${bad_tag}"
mkdir -p "$bad_dir"
bad_archive="kue_0.9.9-badsum_${os}_${arch}.tar.gz"
cp "$good_dir/$archive_name" "$bad_dir/$bad_archive"
echo "0000000000000000000000000000000000000000000000000000000000000000  $bad_archive" > "$bad_dir/checksums.txt"

# --- serve it locally ----------------------------------------------------

port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
( cd "$WORK/server" && exec python3 -m http.server "$port" --bind 127.0.0.1 >"$WORK/server.log" 2>&1 ) &
SERVER_PID=$!

for _ in 1 2 3 4 5 6 7 8 9 10; do
    curl -fsS "http://127.0.0.1:${port}/api.json" >/dev/null 2>&1 && break
    sleep 0.3
done

API_URL="http://127.0.0.1:${port}/api.json"
BASE_URL="http://127.0.0.1:${port}/download"

run_installer() {
    # run_installer <install_dir> [extra args...]
    local dir="$1"; shift
    KUE_INSTALL_API_URL="$API_URL" KUE_INSTALL_BASE_URL="$BASE_URL" \
        "$INSTALLER" --dir "$dir" "$@"
}

echo ""
echo "=== install.sh end-to-end tests (${os}/${arch}) ==="

# 1. Install the "latest" release (no --version) into a fresh dir.
dir1="$WORK/install-latest"
if out=$(run_installer "$dir1" 2>&1); then
    if [ -x "$dir1/kue" ] && "$dir1/kue" --help >/dev/null 2>&1; then
        ok "installs the latest release and the binary runs"
    else
        bad "installed binary missing or not runnable:\n$out"
    fi
else
    bad "install (latest) exited non-zero:\n$out"
fi

# 2. Install an explicit --version.
dir2="$WORK/install-explicit-version"
if out=$(run_installer "$dir2" --version "$tag" 2>&1); then
    if [ -x "$dir2/kue" ]; then
        ok "installs an explicit --version"
    else
        bad "explicit --version: binary missing"
    fi
else
    bad "install (--version $tag) exited non-zero:\n$out"
fi

# 3. Checksum mismatch must fail closed and must NOT install anything.
dir3="$WORK/install-badsum"
if out=$(run_installer "$dir3" --version "$bad_tag" 2>&1); then
    bad "checksum mismatch should have failed but exited 0:\n$out"
else
    if [ -e "$dir3/kue" ]; then
        bad "checksum mismatch exited non-zero but still wrote a binary"
    elif ! printf '%s' "$out" | grep -qi "checksum mismatch"; then
        bad "checksum mismatch failed, but for the wrong reason:\n$out"
    else
        ok "rejects a bad checksum without installing anything"
    fi
fi

# 4. A version that doesn't exist on the server must fail closed (404).
dir4="$WORK/install-missing-version"
if out=$(run_installer "$dir4" --version "v0.0.0-does-not-exist" 2>&1); then
    bad "missing version should have failed but exited 0:\n$out"
else
    if [ ! -e "$dir4/kue" ]; then
        ok "rejects a version with no matching release asset"
    else
        bad "missing-version case still wrote a binary"
    fi
fi

echo ""
echo "=== $pass passed, $fail failed ==="
[ "$fail" -eq 0 ]
