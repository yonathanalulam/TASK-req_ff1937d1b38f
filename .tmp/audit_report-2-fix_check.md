# audit_report-2 Fix Check (Static Review)

Reviewed against issues listed in `.tmp/audit_report-2.md`.

## Overall Result
- **5 / 5 issues appear fixed in code (static evidence).**
- No runtime execution was performed; conclusions are static-only.

## Issue-by-Issue Verification

### 1) High — Session cookie `Secure` flag disabled outside production
- **Status:** **Fixed**
- **What changed:** Session cookie is now always set with `Secure=true` and `HttpOnly=true`, independent of `APP_ENV`.
- **Evidence:**
  - `backend/internal/auth/handler.go:172`
  - `backend/internal/auth/handler.go:184`
  - `backend/internal/auth/integration_test.go:99`
  - `backend/internal/auth/integration_test.go:119`
  - `backend/internal/auth/integration_test.go:359`

### 2) High — Security test fixture used nonexistent `notifications.code`
- **Status:** **Fixed**
- **What changed:** Fixture now inserts into `template_code`, matching schema.
- **Evidence:**
  - `backend/internal/securitytest/authorization_test.go:64`
  - `backend/internal/db/migrations/000010_notifications.up.sql:15`

### 3) Medium — Frontend route-level role gating incomplete on privileged pages
- **Status:** **Fixed**
- **What changed:** Privileged routes now declare `meta.roles`; e2e tests added for direct URL redirection.
- **Evidence:**
  - `frontend/src/router/index.js:126`
  - `frontend/src/router/index.js:132`
  - `frontend/src/router/index.js:165`
  - `frontend/tests/e2e/moderation.spec.js:44`
  - `frontend/tests/e2e/hmac-keys.spec.js:33`

### 4) Medium — Notification requirement gap (missing upcoming end-time semantics)
- **Status:** **Fixed**
- **What changed:** Added `upcoming_end` template code, seeded default template, lifecycle dispatch when ticket enters `In Service`, and test coverage.
- **Evidence:**
  - `backend/internal/models/notification.go:13`
  - `backend/internal/notification/service.go:386`
  - `backend/internal/ticket/service.go:242`
  - `backend/internal/notification/service_test.go:245`
  - `backend/internal/ticket/service_test.go:466`

### 5) Medium — HMAC integration tests were overly permissive
- **Status:** **Fixed**
- **What changed:** Integration tests now target a real internal route with strict expected statuses (400/401/200) and seeded valid key/signature flow.
- **Evidence:**
  - `backend/internal/auth/integration_test.go:220`
  - `backend/internal/auth/integration_test.go:225`
  - `backend/internal/auth/integration_test.go:275`
  - `backend/internal/auth/integration_test.go:303`
  - `backend/internal/auth/integration_test.go:323`

## Notes / Boundary
- This check is **static-only** (no app/test execution).
- Runtime correctness and full regression safety remain **manual verification required** by running the relevant backend/frontend test suites.
