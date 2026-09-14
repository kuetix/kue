#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"; #"
DEPLOY_HOST=${DEPLOY_HOST:-kuetix.com}
REMOTE_DIR=${REMOTE_DIR:-/opt/kuetix/mcp}
REMOTE_BINARY=${REMOTE_BINARY:-mcp-server_arm64}
VERSION=${VERSION:-$(date -u +%Y%m%d%H%M%S)}
SERVICES=${SERVICES:-"mcp-server-http.service mcp-server-sse.service"}

case "$VERSION" in
    ''|*[!A-Za-z0-9._-]*) echo "VERSION contains unsafe characters: $VERSION" >&2; exit 2 ;;
esac

for service in $SERVICES; do
    case "$service" in
        ''|*[!A-Za-z0-9_.@-]*) echo "Invalid service name: $service" >&2; exit 2 ;;
    esac
done

for arg in "$@"; do
    case "$arg" in
        -y|--yes) ;;
        -h|--help)
            echo "Usage: ./deploy.sh [-y|--yes]"
            echo "Deploys the ARM64 MCP server to ${DEPLOY_HOST}:${REMOTE_DIR}."
            exit 0
            ;;
        *) echo "Unknown argument: $arg" >&2; exit 2 ;;
    esac
done

command -v make >/dev/null || { echo "make is required" >&2; exit 1; }
command -v ssh >/dev/null || { echo "ssh is required" >&2; exit 1; }
command -v scp >/dev/null || { echo "scp is required" >&2; exit 1; }

echo "--> Building ARM64 MCP server"
(
    cd "$ROOT_DIR"
    make build_linux_arm
)

LOCAL_BINARY=${LOCAL_BINARY:-${ROOT_DIR}/mcp-server_arm64}
[ -x "$LOCAL_BINARY" ] || { echo "MCP binary not found: $LOCAL_BINARY" >&2; exit 1; }

REMOTE_TMP="${REMOTE_DIR}/.${REMOTE_BINARY}.${VERSION}.tmp"
REMOTE_BACKUP="${REMOTE_DIR}/${REMOTE_BINARY}.${VERSION}.previous"

echo "--> Uploading ${REMOTE_BINARY}"
ssh -o ClearAllForwardings=yes "$DEPLOY_HOST" "mkdir -p '${REMOTE_DIR}'"
scp -o ClearAllForwardings=yes "$LOCAL_BINARY" "${DEPLOY_HOST}:${REMOTE_TMP}"
ssh -o ClearAllForwardings=yes "$DEPLOY_HOST" "
    set -eu
    chmod 0755 '${REMOTE_TMP}'
    if [ -e '${REMOTE_DIR}/${REMOTE_BINARY}' ]; then
        cp -p '${REMOTE_DIR}/${REMOTE_BINARY}' '${REMOTE_BACKUP}'
    fi
    mv -f '${REMOTE_TMP}' '${REMOTE_DIR}/${REMOTE_BINARY}'
    systemctl restart ${SERVICES}
    for service in ${SERVICES}; do
        systemctl is-active --quiet \"\$service\"
    done
"

echo "--> MCP server deployed: ${VERSION}"
