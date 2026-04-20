# Fix-Check Report (Static Re-Audit)

Source baseline: `.tmp/static-audit-report.md`  
Method: static-only code/doc inspection (no runtime execution)

## Overall Fix-Check Verdict

- **Overall: All Fixed (9 Fixed, 0 Partial)**
- The previously reported Blocker/High/Medium issues are now addressed in static code/doc evidence.

## Issue-by-Issue Status

### 1) Blocker — TLS requirement not implemented; server always starts HTTP
- **Status:** **Fixed**
- **What changed:** TLS is now mandatory in all environments; startup fails if TLS paths are missing or unreadable, and server uses `ListenAndServeTLS` with no HTTP fallback path.
- **Evidence:** `backend/internal/config/config.go:95`, `backend/internal/config/config.go:105`, `backend/cmd/server/main.go:175`, `backend/cmd/server/main.go:183`, `backend/cmd/server/main.go:188`, `backend/cmd/server/main.go:202`, `README.md:189`, `README.md:202`, `scripts/start.sh:56`, `scripts/run-tests.sh:54`, `scripts/gen-tls-cert.sh:4`

### 2) Blocker — Test runner claims full test execution but does not run stated suites
- **Status:** **Fixed**
- **What changed:** test flow now runs real backend and Playwright commands via `scripts/run-tests.sh`.
- **Evidence:** `scripts/run-tests.sh:66`, `scripts/run-tests.sh:72`, `scripts/run-tests.sh:76`, `scripts/run-tests.sh:82`, `deploy/docker/docker-compose.test.yml:21`, `deploy/docker/docker-compose.test.yml:24`, `deploy/docker/docker-compose.test.yml:44`, `README.md:105`, `README.md:106`, `README.md:107`
- **Note:** `deploy/docker/docker-compose.test.yml:57` still defines a placeholder `test` service, but the documented/primary runner now invokes test containers directly.

### 3) High — Missing review throttling (10/hour) for review submissions
- **Status:** **Fixed**
- **What changed:** review rate limiter is applied to review create/update routes (report throttle remains present).
- **Evidence:** `backend/internal/router/router.go:316`, `backend/internal/router/router.go:319`, `backend/internal/router/router.go:325`, `backend/internal/router/router.go:331`, `backend/internal/review/throttle_test.go:3`, `backend/internal/review/throttle_test.go:49`

### 4) High — Ticket creation lacks address ownership validation
- **Status:** **Fixed**
- **What changed:** ticket creation now verifies `addresses.user_id` matches caller `UserID`; foreign address returns forbidden.
- **Evidence:** `backend/internal/ticket/service.go:86`, `backend/internal/ticket/service.go:90`, `backend/internal/ticket/service.go:99`, `backend/internal/ticket/service.go:100`, `backend/internal/ticket/address_idor_test.go:3`, `backend/internal/ticket/address_idor_test.go:43`

### 5) High — Data Operator role defined but not functionally realized
- **Status:** **Fixed**
- **What changed:** explicit backend `/api/v1/dataops/*` RBAC group for `data_operator` + `administrator`; frontend route/nav guards include `data_operator`; seed user role present.
- **Evidence:** `backend/internal/router/router.go:236`, `backend/internal/router/router.go:242`, `backend/internal/router/router.go:243`, `frontend/src/router/index.js:140`, `frontend/src/router/index.js:147`, `frontend/src/components/AppShell.vue:17`, `frontend/src/components/AppShell.vue:101`, `deploy/seed/001_users.sql:11`, `backend/internal/ingest/dataops_auth_test.go:5`, `backend/internal/ingest/dataops_auth_test.go:85`

### 6) High — Ingestion/lakehouse orchestration incomplete vs execution requirements
- **Status:** **Fixed (static evidence)**
- **What changed:** job runner now executes end-to-end source pull, schema versioning, Bronze/Silver/Gold writes, checkpoint persistence, and job progress updates; route wiring exposes run endpoint in DataOps surface.
- **Evidence:** `backend/internal/ingest/runner.go:3`, `backend/internal/ingest/runner.go:59`, `backend/internal/ingest/runner.go:95`, `backend/internal/ingest/runner.go:117`, `backend/internal/ingest/runner.go:121`, `backend/internal/ingest/runner.go:132`, `backend/internal/ingest/runner.go:137`, `backend/internal/ingest/runner.go:145`, `backend/internal/router/router.go:251`, `README.md:246`
- **Boundary:** runtime correctness/performance of all source types still requires execution-based validation.

### 7) Medium — Session inactivity extension unreliable (async Touch using request context)
- **Status:** **Fixed**
- **What changed:** `Touch` now runs under bounded background context instead of request context.
- **Evidence:** `backend/internal/middleware/auth.go:82`, `backend/internal/middleware/auth.go:90`, `backend/internal/middleware/auth.go:92`

### 8) Medium — Lakehouse lifecycle policy execution not wired into runtime workers
- **Status:** **Fixed**
- **What changed:** lifecycle worker exists and is started from router; admin on-demand lifecycle trigger endpoint is also wired.
- **Evidence:** `backend/internal/lakehouse/worker.go:16`, `backend/internal/lakehouse/worker.go:21`, `backend/internal/lakehouse/worker.go:43`, `backend/internal/router/router.go:156`, `backend/internal/router/router.go:158`, `backend/internal/router/router.go:234`, `backend/internal/lakehouse/handler.go:83`, `backend/internal/lakehouse/lifecycle_trigger_test.go:79`

### 9) Medium — Forged-cookie security test used wrong cookie name
- **Status:** **Fixed**
- **What changed:** forged-cookie test uses `sp_session`, matching runtime cookie name.
- **Evidence:** `backend/internal/securitytest/web_attack_test.go:98`, `backend/internal/securitytest/web_attack_test.go:101`, `backend/internal/session/store.go:22`

## Static Verification Boundaries

- No commands/tests were executed in this pass; conclusions are static-only.
