#!/usr/bin/env bash
# gen-tls-cert.sh — generate a self-signed TLS cert+key for local dev/test.
#
# The backend requires TLS in every environment (no HTTP fallback). This
# script produces a deterministic self-signed pair suitable for local
# development and automated tests. Do NOT use these certs in production.
#
# Output (both paths are what the default .env points at):
#   storage/tls/dev.crt
#   storage/tls/dev.key
#
# Safe to run repeatedly — it only regenerates when no pair exists, unless
# called with --force.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TLS_DIR="${REPO_ROOT}/storage/tls"
CERT="${TLS_DIR}/dev.crt"
KEY="${TLS_DIR}/dev.key"

FORCE=0
if [[ "${1:-}" == "--force" ]]; then
  FORCE=1
fi

mkdir -p "${TLS_DIR}"

if [[ -f "${CERT}" && -f "${KEY}" && ${FORCE} -eq 0 ]]; then
  echo "[tls] ${CERT} already exists — skipping (use --force to regenerate)"
  exit 0
fi

if ! command -v openssl >/dev/null 2>&1; then
  echo "error: openssl is required to generate a TLS cert" >&2
  exit 1
fi

echo "[tls] generating self-signed cert at ${CERT}"
openssl req -x509 -newkey rsa:2048 -days 365 -nodes \
  -keyout "${KEY}" -out "${CERT}" \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,DNS:backend,IP:127.0.0.1" \
  >/dev/null 2>&1

chmod 600 "${KEY}"
chmod 644 "${CERT}"
echo "[tls] cert written to ${CERT}"
echo "[tls] key  written to ${KEY}"
