#!/usr/bin/env bash
# soak-cloud.sh — provision a Hetzner VM, run the soak test, pull results, destroy VM.
#
# Prerequisites:
#   - hcloud CLI installed and configured (HCLOUD_TOKEN env var or hcloud context)
#   - SSH key registered with Hetzner (set HCLOUD_SSH_KEY to its name)
#
# Usage:
#   SOAK_DURATION=1h ./scripts/soak-cloud.sh
#
# Environment:
#   HCLOUD_SSH_KEY     — name of SSH key in Hetzner (required)
#   SOAK_DURATION      — test duration (default: 30m)
#   SOAK_WORKERS       — concurrent goroutines (default: 4)
#   SOAK_SERVER_TYPE   — Hetzner server type (default: cx22)
#   SOAK_IMAGE         — OS image (default: ubuntu-24.04)
#   SOAK_LOCATION      — datacenter location (default: fsn1)
#   SOAK_KEEP_SERVER   — set to "1" to skip server deletion

set -euo pipefail

# --- Config ---
SERVER_NAME="soak-go-postgres-$(date +%s)"
SERVER_TYPE="${SOAK_SERVER_TYPE:-cax11}"
IMAGE="${SOAK_IMAGE:-ubuntu-24.04}"
LOCATION="${SOAK_LOCATION:-nbg1}"
SSH_KEY="${HCLOUD_SSH_KEY:?Set HCLOUD_SSH_KEY to your Hetzner SSH key name}"
DURATION="${SOAK_DURATION:-30m}"
WORKERS="${SOAK_WORKERS:-4}"
RESULTS_DIR="soak-results"
RESULT_FILE="${RESULTS_DIR}/soak-$(date +%Y%m%d-%H%M%S).jsonl"
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

mkdir -p "${PROJECT_DIR}/${RESULTS_DIR}"

echo "==> Creating Hetzner server: ${SERVER_NAME} (${SERVER_TYPE}, ${IMAGE}, ${LOCATION})"
hcloud server create \
  --name "${SERVER_NAME}" \
  --type "${SERVER_TYPE}" \
  --image "${IMAGE}" \
  --location "${LOCATION}" \
  --ssh-key "${SSH_KEY}"

# Get IP
SERVER_IP=$(hcloud server ip "${SERVER_NAME}")
echo "==> Server IP: ${SERVER_IP}"

# Wait for SSH
echo "==> Waiting for SSH..."
for i in $(seq 1 30); do
  if ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 root@"${SERVER_IP}" true 2>/dev/null; then
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "ERROR: SSH not available after 30 attempts"
    hcloud server delete "${SERVER_NAME}"
    exit 1
  fi
  sleep 2
done

SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
SSH="ssh ${SSH_OPTS} root@${SERVER_IP}"
SCP="scp ${SSH_OPTS}"

cleanup() {
  if [ "${SOAK_KEEP_SERVER:-0}" = "1" ]; then
    echo "==> SOAK_KEEP_SERVER=1, keeping server: ${SERVER_NAME} (${SERVER_IP})"
    return
  fi
  echo "==> Destroying server: ${SERVER_NAME}"
  hcloud server delete "${SERVER_NAME}" || true
}
trap cleanup EXIT

echo "==> Installing Go on remote..."
${SSH} bash <<'REMOTE_INSTALL'
set -euo pipefail
apt-get update -qq
apt-get install -y -qq wget >/dev/null 2>&1
GO_VERSION="1.26.0"
ARCH=$(dpkg --print-architecture)
case "${ARCH}" in
  amd64) GO_ARCH="amd64" ;;
  arm64|aarch64) GO_ARCH="arm64" ;;
  *) echo "Unsupported arch: ${ARCH}"; exit 1 ;;
esac
wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
tar -C /usr/local -xzf "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
echo 'export PATH=$PATH:/usr/local/go/bin:/root/go/bin' >> /root/.bashrc
export PATH=$PATH:/usr/local/go/bin
go version
REMOTE_INSTALL

echo "==> Syncing project to remote..."
rsync -az --exclude='.git' --exclude='soak-results' \
  -e "ssh ${SSH_OPTS}" \
  "${PROJECT_DIR}/" root@"${SERVER_IP}":/root/go-postgres/

echo "==> Running soak test (duration=${DURATION}, workers=${WORKERS})..."
${SSH} bash <<REMOTE_RUN
set -euo pipefail
export PATH=\$PATH:/usr/local/go/bin:/root/go/bin
cd /root/go-postgres
SOAK_DURATION=${DURATION} SOAK_WORKERS=${WORKERS} SOAK_OUTPUT=/root/soak-results.jsonl \
  go test -run TestSoak -timeout 0 -count=1 -v 2>&1 | tee /root/soak-test.log
REMOTE_RUN

echo "==> Pulling results..."
${SCP} root@"${SERVER_IP}":/root/soak-results.jsonl "${PROJECT_DIR}/${RESULT_FILE}"
${SCP} root@"${SERVER_IP}":/root/soak-test.log "${PROJECT_DIR}/${RESULTS_DIR}/soak-$(date +%Y%m%d-%H%M%S).log"

echo "==> Results saved to ${RESULT_FILE}"
echo "==> Done."
