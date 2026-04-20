#!/usr/bin/env bash
# gen-key.sh — print a freshly generated 32-byte hex-encoded AES-256 key.
#
# Intended for generating FIELD_ENCRYPTION_KEY values for new deployments.
# Output is suitable for pasting directly into a .env file:
#
#     FIELD_ENCRYPTION_KEY=$(./scripts/gen-key.sh)
#
# The script tries a few sources of cryptographic randomness in order of
# preference so it works on systems that may not have OpenSSL installed.

set -euo pipefail

if command -v openssl >/dev/null 2>&1; then
  openssl rand -hex 32
  exit 0
fi

if [[ -r /dev/urandom ]] && command -v xxd >/dev/null 2>&1; then
  head -c 32 /dev/urandom | xxd -p -c 64
  exit 0
fi

if [[ -r /dev/urandom ]] && command -v od >/dev/null 2>&1; then
  head -c 32 /dev/urandom | od -An -v -tx1 | tr -d ' \n'
  echo
  exit 0
fi

if command -v python3 >/dev/null 2>&1; then
  python3 -c 'import secrets; print(secrets.token_hex(32))'
  exit 0
fi

echo "gen-key.sh: no usable random source found (tried openssl, /dev/urandom+xxd/od, python3)" >&2
exit 1
