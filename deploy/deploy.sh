#!/usr/bin/env bash
set -euo pipefail

# Configuration
SERVER_HOST="gcp-prau"
REMOTE_DIR="/home/mthidayat/dompet-cerdas-backend"
LOCAL_BUILD_DIR="bin"
BINARY_NAME="dompet-cerdas-go"

echo "🚀 Starting Deployment for DompetCerdas Go Backend..."

# 1. Compile static binary for Linux AMD64 (gcp-prau target)
echo "📦 Cross-compiling Go binary for Linux AMD64..."
mkdir -p "${LOCAL_BUILD_DIR}"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s" -o "${LOCAL_BUILD_DIR}/${BINARY_NAME}" ./cmd/api

echo "✅ Compilation successful. Binary size: $(du -h "${LOCAL_BUILD_DIR}/${BINARY_NAME}" | cut -f1)"

# 2. Ensure remote directory exists
echo "📂 Preparing remote directory at ${SERVER_HOST}:${REMOTE_DIR}..."
ssh "${SERVER_HOST}" "mkdir -p ${REMOTE_DIR}"

# 3. Upload binary and configs via scp
# The old binary is renamed out of the way first: systemd keeps it open while
# running, and scp truncating that path in place fails with ETXTBSY.
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
echo "📤 Uploading binary and deployment files to ${SERVER_HOST}..."
ssh "${SERVER_HOST}" "[ -f ${REMOTE_DIR}/${BINARY_NAME} ] && mv ${REMOTE_DIR}/${BINARY_NAME} ${REMOTE_DIR}/${BINARY_NAME}.bak-${TIMESTAMP} || true"
scp "${LOCAL_BUILD_DIR}/${BINARY_NAME}" "${SERVER_HOST}:${REMOTE_DIR}/"
scp deploy/dompet-cerdas.service "${SERVER_HOST}:${REMOTE_DIR}/"
scp deploy/Caddyfile "${SERVER_HOST}:${REMOTE_DIR}/"

if [ -f ".env.production" ]; then
    echo "🔑 Uploading .env.production to remote .env..."
    scp .env.production "${SERVER_HOST}:${REMOTE_DIR}/.env"
fi

if [ -f "service-account.json" ]; then
    echo "🔑 Uploading service-account.json to remote..."
    scp service-account.json "${SERVER_HOST}:${REMOTE_DIR}/"
fi

# 4. Permissions & Instructions
ssh "${SERVER_HOST}" "chmod +x ${REMOTE_DIR}/${BINARY_NAME}"

echo ""
echo "🎉 Binary uploaded successfully to ${SERVER_HOST}!"
echo ""
echo "📋 NEXT STEPS ON REMOTE SERVER (run once if first time):"
echo "  1. SSH to server: ssh ${SERVER_HOST}"
echo "  2. Symlink systemd service:"
echo "     sudo cp ${REMOTE_DIR}/dompet-cerdas.service /etc/systemd/system/"
echo "     sudo systemctl daemon-reload"
echo "     sudo systemctl enable --now dompet-cerdas"
echo "  3. Install Caddy for automatic HTTPS (if not installed):"
echo "     sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https"
echo "     curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg"
echo "     curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list"
echo "     sudo apt update && sudo apt install caddy"
echo "     sudo cp ${REMOTE_DIR}/Caddyfile /etc/caddy/Caddyfile"
echo "     sudo systemctl reload caddy"
