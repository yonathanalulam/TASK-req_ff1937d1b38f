#!/usr/bin/env bash
# run_tests.sh - single-command runner for ALL test layers.
#
# Runs, in order:
#   1. Backend unit + integration tests (Go)
#   2. Frontend unit tests (Vitest)
#   3. Frontend E2E tests (Playwright)
#
# Fails the pipeline on any non-zero exit via `set -euo pipefail` plus
# explicit checks of each `docker compose run --rm` result.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}"
cd "${REPO_ROOT}"

# Resolve docker compose invocation: prefer v2 plugin, fall back to v1 binary.
if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE=(docker-compose)
else
  echo "error: neither 'docker compose' nor 'docker-compose' is available" >&2
  exit 1
fi

# Compose files: base (docker-compose.yml has defaults for every env var)
# + test overlay (adds backend-test, frontend-unit-test, frontend-test).
COMPOSE_CMD=("${COMPOSE[@]}" -f docker-compose.yml -f docker-compose.test.yml)

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

echo ""
echo "=========================================="
echo "  Step 4/4 - Frontend E2E tests (Playwright)"
echo "=========================================="
"${COMPOSE_CMD[@]}" run --rm frontend-test

echo ""
echo "All tests passed."
