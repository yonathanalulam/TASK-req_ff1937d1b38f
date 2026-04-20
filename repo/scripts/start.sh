#!/usr/bin/env bash
# start.sh — single-command application launcher
# Usage: ./scripts/start.sh
# Brings up MySQL, runs migrations, seeds data, then starts the backend API and frontend.
# Requires: Docker + either `docker-compose` (v1) or `docker compose` (v2 plugin)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_DIR="${REPO_ROOT}/deploy/docker"

# Resolve docker compose invocation: prefer v2 plugin, fall back to v1 binary.
if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE=(docker-compose)
else
  echo "error: neither 'docker compose' nor 'docker-compose' is available" >&2
  exit 1
fi

# Ensure .env exists — try .env.example first, generate a default if absent.
ENV_FILE="${REPO_ROOT}/.env"
if [[ ! -f "${ENV_FILE}" ]]; then
  if [[ -f "${REPO_ROOT}/.env.example" ]]; then
    echo "[setup] .env not found — copying from .env.example"
    cp "${REPO_ROOT}/.env.example" "${ENV_FILE}"
  else
    echo "[setup] .env not found and no .env.example — generating default .env"
    cat > "${ENV_FILE}" <<'ENVEOF'
DB_ROOT_PASSWORD=rootpassword
DB_NAME=service_portal
DB_USER=portal_user
DB_PASSWORD=portalpassword
DB_HOST=db
DB_PORT=3306
DB_TEST_NAME=service_portal_test
APP_ENV=development
PORT=8080
SESSION_COOKIE_DOMAIN=localhost
FIELD_ENCRYPTION_KEY=0000000000000000000000000000000000000000000000000000000000000000
TLS_CERT_FILE=/app/storage/tls/dev.crt
TLS_KEY_FILE=/app/storage/tls/dev.key
# Empty VITE_API_BASE_URL → SPA uses relative /api paths, which nginx in the
# frontend container proxies to the TLS backend (proxy_ssl_verify off on the
# internal compose network). Avoids CORS and self-signed-cert trust issues
# between the browser and the backend.
VITE_API_BASE_URL=
FRONTEND_PORT=5173
BACKEND_PORT=8080
ENVEOF
  fi
fi

# TLS is mandatory in all environments — generate a self-signed dev cert if
# one is not already present. The resulting paths match the .env defaults and
# the compose volume mount.
"${SCRIPT_DIR}/gen-tls-cert.sh"

cd "${COMPOSE_DIR}"

echo ""
echo "══════════════════════════════════════════"
echo "  Starting Service Portal (TLS-everywhere)"
echo "══════════════════════════════════════════"
echo "  Frontend: http://localhost:5173"
echo "  Backend:  https://localhost:8080 (self-signed — browser will warn)"
echo "  Health:   https://localhost:8080/health"
echo "══════════════════════════════════════════"
echo ""

"${COMPOSE[@]}" \
  -f docker-compose.yml \
  --env-file "${ENV_FILE}" \
  up --build
