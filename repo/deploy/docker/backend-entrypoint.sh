#!/bin/sh
# backend-entrypoint.sh - generates a self-signed TLS pair on first start if
# none exists, then hands off to the Go server binary. TLS is mandatory for
# the backend at config-load time, so we must have readable cert/key files on
# disk before the binary is exec'd.

set -eu

: "${TLS_CERT_FILE:=/app/storage/tls/dev.crt}"
: "${TLS_KEY_FILE:=/app/storage/tls/dev.key}"
export TLS_CERT_FILE TLS_KEY_FILE

CERT_DIR=$(dirname "${TLS_CERT_FILE}")
mkdir -p "${CERT_DIR}"

if [ ! -s "${TLS_CERT_FILE}" ] || [ ! -s "${TLS_KEY_FILE}" ]; then
  echo "[backend-entrypoint] generating self-signed TLS cert at ${TLS_CERT_FILE}"
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
    -keyout "${TLS_KEY_FILE}" \
    -out "${TLS_CERT_FILE}" \
    -subj "/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,DNS:backend,IP:127.0.0.1" \
    >/dev/null 2>&1
  chmod 644 "${TLS_CERT_FILE}"
  chmod 600 "${TLS_KEY_FILE}"
fi

exec ./server "$@"
