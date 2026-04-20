#!/usr/bin/env bash
# run-tests.sh — single-command test runner
# Usage: ./scripts/run-tests.sh
# Runs: backend unit+integration tests, then Playwright e2e tests
# Requires: Docker + either `docker-compose` (v1) or `docker compose` (v2 plugin)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_DIR="${REPO_ROOT}/deploy/docker"
PROJECT_NAME="service-portal-test"

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
# Empty VITE_API_BASE_URL → SPA uses relative /api paths, proxied by nginx
# to the TLS backend inside the docker network.
VITE_API_BASE_URL=
FRONTEND_PORT=5173
BACKEND_PORT=8080
ENVEOF
  fi
fi

# TLS is mandatory — the `backend` compose service won't start without a
# cert/key pair. Generate a deterministic self-signed pair so the Playwright
# tests have a real server to talk to.
bash "${SCRIPT_DIR}/gen-tls-cert.sh"

cd "${COMPOSE_DIR}"

COMPOSE_ARGS=(-p "${PROJECT_NAME}" -f docker-compose.yml -f docker-compose.test.yml --env-file "${ENV_FILE}")

# Avoid collisions with an already-running dev stack and keep retries clean.
"${COMPOSE[@]}" "${COMPOSE_ARGS[@]}" down --remove-orphans >/dev/null 2>&1 || true
cleanup() {
  "${COMPOSE[@]}" "${COMPOSE_ARGS[@]}" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo ""
echo "══════════════════════════════════════════"
echo "  Step 1/3 — Build images"
echo "══════════════════════════════════════════"
"${COMPOSE[@]}" "${COMPOSE_ARGS[@]}" build --parallel

echo ""
echo "══════════════════════════════════════════"
echo "  Step 2/3 — Backend unit + integration tests"
echo "══════════════════════════════════════════"
"${COMPOSE[@]}" "${COMPOSE_ARGS[@]}" run --rm backend-test

echo ""
echo "══════════════════════════════════════════"
echo "  Step 3/3 — Playwright e2e tests"
echo "══════════════════════════════════════════"
"${COMPOSE[@]}" "${COMPOSE_ARGS[@]}" run --rm frontend-test

echo ""
echo "All tests passed."
