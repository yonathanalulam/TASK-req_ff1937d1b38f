#!/usr/bin/env bash
# run_tests.sh — single-command runner for ALL test layers.
# Runs, in order:
#   1. Backend unit + integration tests (Go)
#   2. Frontend unit tests (Vitest)
#   3. Frontend E2E tests (Playwright)
# Fails the whole pipeline on any layer failure via `set -euo pipefail` plus
# explicit exit-code capture from each `docker compose run --rm`.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}"
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

ENV_FILE="${REPO_ROOT}/.env"
if [[ ! -f "${ENV_FILE}" ]]; then
  if [[ -f "${REPO_ROOT}/.env.example" ]]; then
    echo "[setup] .env not found - copying from .env.example"
    cp "${REPO_ROOT}/.env.example" "${ENV_FILE}"
  else
    echo "error: neither .env nor .env.example exists" >&2
    exit 1
  fi
fi

# TLS certs must exist before the backend starts.
"${SCRIPT_DIR}/scripts/gen-tls-cert.sh"

cd "${COMPOSE_DIR}"

COMPOSE_FILES=(-f docker-compose.yml -f docker-compose.test.yml)
COMPOSE_CMD=("${COMPOSE[@]}" "${COMPOSE_FILES[@]}" --env-file "${ENV_FILE}")

echo ""
echo "=========================================="
echo "  Step 1/4 - Build test images"
echo "=========================================="
"${COMPOSE_CMD[@]}" build --parallel

echo ""
echo "=========================================="
echo "  Step 2/4 - Backend unit + integration tests"
echo "=========================================="
"${COMPOSE_CMD[@]}" run --rm backend-test

echo ""
echo "=========================================="
echo "  Step 3/4 - Frontend unit tests (Vitest)"
echo "=========================================="
"${COMPOSE_CMD[@]}" run --rm frontend-unit-test

echo ""
echo "=========================================="
echo "  Step 4/4 - Frontend E2E tests (Playwright)"
echo "=========================================="
"${COMPOSE_CMD[@]}" run --rm frontend-test

echo ""
echo "All tests passed."
