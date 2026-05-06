#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/opt/llm-bench"
DATA_DIR="${INSTALL_DIR}/data"
DATA_REPO="${INSTALL_DIR}/data-repo"
REPO_URL="${1:-}"

if [ -z "$REPO_URL" ]; then
    echo "usage: setup.sh <data-repo-git-url>"
    echo "example: setup.sh git@github.com:Jwrede/llm-bench-data.git"
    exit 1
fi

echo "creating directories..."
sudo mkdir -p "$INSTALL_DIR" "$DATA_DIR"
sudo chown "$(whoami)" "$INSTALL_DIR" "$DATA_DIR"

echo "cloning data repo..."
if [ ! -d "$DATA_REPO" ]; then
    git clone "$REPO_URL" "$DATA_REPO"
else
    echo "data repo already exists at $DATA_REPO"
fi

echo "installing llmprobe..."
if ! command -v llmprobe &>/dev/null; then
    go install github.com/Jwrede/llmprobe@latest
    echo "llmprobe installed to $(which llmprobe)"
else
    echo "llmprobe already installed at $(which llmprobe)"
fi

echo "installing ttyd..."
if ! command -v ttyd &>/dev/null; then
    sudo apt-get update && sudo apt-get install -y ttyd
    echo "ttyd installed"
else
    echo "ttyd already installed at $(which ttyd)"
fi

echo "building discover tool..."
cd "$(dirname "$0")/.."
go build -o "${INSTALL_DIR}/discover" ./cmd/discover/

echo "copying config and scripts..."
cp scripts/push-data.sh "${INSTALL_DIR}/push-data.sh"
chmod +x "${INSTALL_DIR}/push-data.sh"

if [ ! -f "${INSTALL_DIR}/probes.yml" ]; then
    echo "generating initial probes.yml..."
    "${INSTALL_DIR}/discover" -o "${INSTALL_DIR}/probes.yml"
fi

echo "installing systemd services..."
sudo cp deploy/llm-bench.service /etc/systemd/system/
sudo cp deploy/llm-bench-ttyd.service /etc/systemd/system/
sudo cp deploy/llm-bench-push.service /etc/systemd/system/
sudo cp deploy/llm-bench-push.timer /etc/systemd/system/
sudo cp deploy/llm-bench-discover.service /etc/systemd/system/
sudo cp deploy/llm-bench-discover.timer /etc/systemd/system/

sudo systemctl daemon-reload

echo ""
echo "setup complete. next steps:"
echo "  1. create ${INSTALL_DIR}/.env with your API keys"
echo "  2. sudo systemctl enable --now llm-bench"
echo "  3. sudo systemctl enable --now llm-bench-ttyd"
echo "  4. sudo systemctl enable --now llm-bench-push.timer"
echo "  5. sudo systemctl enable --now llm-bench-discover.timer"
echo "  6. configure nginx (see deploy/nginx.conf)"
