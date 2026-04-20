#!/usr/bin/env bash
# run-tests.sh — single-command test runner
# Usage: ./scripts/run-tests.sh
# Runs: backend unit+integration tests, then Playwright e2e tests
# Requires: Docker + docker-compose

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}"
COMPOSE_DIR="${REPO_ROOT}/deploy/docker"
ENV_FILE="${REPO_ROOT}/.env"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "[setup] .env not found — copying from .env.example"
  cp "${REPO_ROOT}/.env.example" "${ENV_FILE}"
fi

cd "${COMPOSE_DIR}"

echo ""
echo "══════════════════════════════════════════"
echo "  Step 1/3 — Build images"
echo "══════════════════════════════════════════"
docker-compose \
  -f docker-compose.yml \
  -f docker-compose.test.yml \
  --env-file "${ENV_FILE}" \
  build --parallel

echo ""
echo "══════════════════════════════════════════"
echo "  Step 2/3 — Backend unit + integration tests"
echo "══════════════════════════════════════════"
docker-compose \
  -f docker-compose.yml \
  -f docker-compose.test.yml \
  --env-file "${ENV_FILE}" \
  run --rm backend-test

echo ""
echo "══════════════════════════════════════════"
echo "  Step 3/3 — Playwright e2e tests"
echo "══════════════════════════════════════════"
docker-compose \
  -f docker-compose.yml \
  -f docker-compose.test.yml \
  --env-file "${ENV_FILE}" \
  run --rm frontend-test

echo ""
echo "✓ All tests passed."
