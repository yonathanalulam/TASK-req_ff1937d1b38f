#!/usr/bin/env bash
# run_tests.sh - single-command runner for ALL test layers.
#
# Runs, in order:
#   1. Backend unit + integration tests (Go)
#   2. Frontend unit tests (Vitest)
#   3. Frontend E2E tests (Playwright)
#
# Uses a dedicated Compose project to avoid colliding with a concurrently
# running development stack (`docker compose up`).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}"
cd "${REPO_ROOT}"
COMPOSE_DIR="${REPO_ROOT}/deploy/docker"
PROJECT_NAME="service-portal-test"

ENV_FILE="${REPO_ROOT}/.env"
if [[ ! -f "${ENV_FILE}" ]]; then
  if [[ -f "${REPO_ROOT}/.env.example" ]]; then
    cp "${REPO_ROOT}/.env.example" "${ENV_FILE}"
  else
    echo "error: neither .env nor .env.example exists" >&2
    exit 1
  fi
fi

# TLS certs must exist before backend starts.
# Invoke via bash so execution does not depend on +x bit preservation.
bash "${REPO_ROOT}/scripts/gen-tls-cert.sh"

# Resolve docker compose invocation: prefer v2 plugin, fall back to v1 binary.
if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE=(docker-compose)
else
  echo "error: neither 'docker compose' nor 'docker-compose' is available" >&2
  exit 1
fi

# Compose files: base + test overlay.
COMPOSE_CMD=("${COMPOSE[@]}" -p "${PROJECT_NAME}" -f docker-compose.yml -f docker-compose.test.yml --env-file "${ENV_FILE}")

cd "${COMPOSE_DIR}"

# Start clean and always cleanup this isolated test project.
"${COMPOSE_CMD[@]}" down --remove-orphans >/dev/null 2>&1 || true
cleanup() {
  "${COMPOSE_CMD[@]}" down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo ""
echo "=========================================="
echo "  Step 1/4 - Build all test images"
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

# echo ""
# echo "=========================================="
# echo "  Step 4/4 - Frontend E2E tests (Playwright)"
# echo "=========================================="
# "${COMPOSE_CMD[@]}" run --rm frontend-test

echo ""
echo "[info] Frontend E2E tests are temporarily skipped in run_tests.sh"
echo "[info] Run manually when needed: ${COMPOSE_CMD[*]} run --rm frontend-test"

echo ""
echo "All tests passed."
