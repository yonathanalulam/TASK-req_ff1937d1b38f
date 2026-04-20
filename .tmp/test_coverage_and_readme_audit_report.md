# Test Coverage Audit

## Scope / Method / Type
- Static inspection only (no runtime execution).
- Project type declaration found: **fullstack** (`README.md:3`).

## Backend Endpoint Inventory
- Source of truth: `backend/internal/router/router.go:162` through `backend/internal/router/router.go:360`.
- Total endpoints (unique METHOD + fully resolved PATH): **101**.

## API Test Mapping Table
Legend: `TNM` = true no-mock HTTP.

| Endpoint | Covered | Type | Evidence |
|---|---|---|---|
| GET `/health` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:131` (`TestHTTP_Health_RealRouter`) |
| POST `/api/v1/auth/register` | yes | TNM | `backend/internal/auth/integration_test.go:66` |
| POST `/api/v1/auth/login` | yes | TNM | `backend/internal/auth/integration_test.go:66` |
| POST `/api/v1/auth/logout` | yes | TNM | `backend/internal/auth/integration_test.go:66` |
| GET `/api/v1/auth/me` | yes | TNM | `backend/internal/auth/integration_test.go:66` |
| GET `/api/v1/service-categories` | yes | TNM | `backend/internal/catalog/integration_test.go:104` |
| GET `/api/v1/shipping/regions` | yes | TNM | `backend/internal/catalog/integration_test.go:263` |
| GET `/api/v1/shipping/templates` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:144` |
| GET `/api/v1/service-offerings/:id/reviews` | yes | TNM | `backend/internal/review/integration_test.go:230` |
| GET `/api/v1/service-offerings/:id/review-summary` | yes | TNM | `backend/internal/review/integration_test.go:255` |
| GET `/api/v1/internal/data/sources` | yes | TNM | `backend/internal/ingest/integration_test.go:104` |
| POST `/api/v1/internal/data/sources` | yes | TNM | `backend/internal/ingest/integration_test.go:113` |
| PUT `/api/v1/internal/data/sources/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:338` |
| GET `/api/v1/internal/data/jobs` | yes | TNM | `backend/internal/ingest/integration_test.go:135` |
| POST `/api/v1/internal/data/jobs` | yes | TNM | `backend/internal/ingest/integration_test.go:129` |
| GET `/api/v1/internal/data/jobs/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:338` |
| GET `/api/v1/internal/data/schema-versions/:source_id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:369` |
| GET `/api/v1/internal/data/catalog` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:369` |
| GET `/api/v1/internal/data/catalog/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:369` |
| GET `/api/v1/internal/data/lineage/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:369` |
| GET `/api/v1/admin/hmac-keys` | yes | TNM | `backend/internal/hmacadmin/integration_test.go:272` |
| POST `/api/v1/admin/hmac-keys` | yes | TNM | `backend/internal/hmacadmin/integration_test.go:126` |
| POST `/api/v1/admin/hmac-keys/rotate` | yes | TNM | `backend/internal/hmacadmin/integration_test.go:171` |
| DELETE `/api/v1/admin/hmac-keys/:id` | yes | TNM | `backend/internal/hmacadmin/integration_test.go:236` |
| POST `/api/v1/admin/service-categories` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:158` |
| PUT `/api/v1/admin/service-categories/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:158` |
| DELETE `/api/v1/admin/service-categories/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:158` |
| POST `/api/v1/admin/shipping/regions` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:205` |
| POST `/api/v1/admin/shipping/templates` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:217` |
| PUT `/api/v1/admin/shipping/templates/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:217` |
| GET `/api/v1/admin/notification-templates` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:258` |
| PUT `/api/v1/admin/notification-templates/:code` | yes | TNM | `backend/internal/notification/integration_test.go:246` |
| GET `/api/v1/admin/sensitive-terms` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:274` |
| POST `/api/v1/admin/sensitive-terms` | yes | TNM | `backend/internal/moderation/integration_test.go:116` |
| DELETE `/api/v1/admin/sensitive-terms/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:274` |
| GET `/api/v1/admin/users/:user_id/violations` | yes | TNM | `backend/internal/moderation/integration_test.go:340` |
| GET `/api/v1/admin/audit-logs` | yes | TNM | `backend/internal/privacy/integration_test.go:211` |
| DELETE `/api/v1/admin/users/:user_id` | yes | TNM | `backend/internal/privacy/integration_test.go:192` |
| GET `/api/v1/admin/legal-holds` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:300` |
| POST `/api/v1/admin/legal-holds` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:300` |
| DELETE `/api/v1/admin/legal-holds/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:300` |
| POST `/api/v1/admin/lakehouse/lifecycle/run` | yes | TNM | `backend/internal/lakehouse/lifecycle_trigger_test.go:79` |
| GET `/api/v1/dataops/sources` | yes | TNM | `backend/internal/ingest/dataops_auth_test.go:74` |
| POST `/api/v1/dataops/sources` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:409` |
| PUT `/api/v1/dataops/sources/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:409` |
| GET `/api/v1/dataops/jobs` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:425` |
| POST `/api/v1/dataops/jobs` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:425` |
| GET `/api/v1/dataops/jobs/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:425` |
| POST `/api/v1/dataops/jobs/:id/run` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:425` |
| GET `/api/v1/dataops/schema-versions/:source_id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:457` |
| GET `/api/v1/dataops/catalog` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:457` |
| GET `/api/v1/dataops/catalog/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:457` |
| GET `/api/v1/dataops/lineage/:id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:457` |
| GET `/api/v1/users/me/profile` | yes | TNM | `backend/internal/profile/integration_test.go:104` |
| PUT `/api/v1/users/me/profile` | yes | TNM | `backend/internal/profile/integration_test.go:113` |
| GET `/api/v1/users/me/preferences` | yes | TNM | `backend/internal/profile/integration_test.go:150` |
| PUT `/api/v1/users/me/preferences` | yes | TNM | `backend/internal/profile/integration_test.go:150` |
| GET `/api/v1/users/me/favorites` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:484` |
| POST `/api/v1/users/me/favorites` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:484` |
| DELETE `/api/v1/users/me/favorites/:offering_id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:484` |
| GET `/api/v1/users/me/history` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:515` |
| DELETE `/api/v1/users/me/history` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:515` |
| GET `/api/v1/users/me/addresses` | yes | TNM | `backend/internal/profile/integration_test.go:195` |
| POST `/api/v1/users/me/addresses` | yes | TNM | `backend/internal/profile/integration_test.go:195` |
| PUT `/api/v1/users/me/addresses/:id` | yes | TNM | `backend/internal/securitytest/authorization_test.go:83` |
| DELETE `/api/v1/users/me/addresses/:id` | yes | TNM | `backend/internal/profile/integration_test.go:195` |
| PUT `/api/v1/users/me/addresses/:id/default` | yes | TNM | `backend/internal/profile/integration_test.go:195` |
| GET `/api/v1/users/me/notifications` | yes | TNM | `backend/internal/notification/integration_test.go:127` |
| GET `/api/v1/users/me/notifications/unread-count` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:531` |
| GET `/api/v1/users/me/notifications/outbox` | yes | TNM | `backend/internal/notification/integration_test.go:178` |
| PATCH `/api/v1/users/me/notifications/read-all` | yes | TNM | `backend/internal/notification/integration_test.go:127` |
| PATCH `/api/v1/users/me/notifications/:id/read` | yes | TNM | `backend/internal/notification/integration_test.go:127` |
| POST `/api/v1/users/me/export-request` | yes | TNM | `backend/internal/privacy/integration_test.go:98` |
| GET `/api/v1/users/me/export-request/status` | yes | TNM | `backend/internal/privacy/integration_test.go:139` |
| GET `/api/v1/users/me/export-request/download` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:544` |
| POST `/api/v1/users/me/deletion-request` | yes | TNM | `backend/internal/privacy/integration_test.go:159` |
| GET `/api/v1/users/me/deletion-request/status` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:556` |
| GET `/api/v1/service-offerings` | yes | TNM | `backend/internal/catalog/integration_test.go:118` |
| GET `/api/v1/service-offerings/:id` | yes | TNM | `backend/internal/catalog/integration_test.go:251` |
| POST `/api/v1/service-offerings` | yes | TNM | `backend/internal/catalog/integration_test.go:127` |
| PUT `/api/v1/service-offerings/:id` | yes | TNM | `backend/internal/catalog/integration_test.go:199` |
| PATCH `/api/v1/service-offerings/:id/status` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:624` |
| POST `/api/v1/shipping/estimate` | yes | TNM | `backend/internal/catalog/integration_test.go:277` |
| GET `/api/v1/tickets` | yes | TNM | `backend/internal/ticket/integration_test.go:155` |
| POST `/api/v1/tickets` | yes | TNM | `backend/internal/ticket/integration_test.go:124` |
| GET `/api/v1/tickets/:id` | yes | TNM | `backend/internal/securitytest/authorization_test.go:141` |
| PATCH `/api/v1/tickets/:id/status` | yes | TNM | `backend/internal/ticket/integration_test.go:225` |
| GET `/api/v1/tickets/:id/notes` | yes | TNM | `backend/internal/ticket/integration_test.go:321` |
| POST `/api/v1/tickets/:id/notes` | yes | TNM | `backend/internal/ticket/integration_test.go:321` |
| GET `/api/v1/tickets/:id/attachments` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:570` |
| DELETE `/api/v1/tickets/:id/attachments/:file_id` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:570` |
| POST `/api/v1/tickets/:id/reviews` | yes | TNM | `backend/internal/review/integration_test.go:115` |
| PUT `/api/v1/tickets/:id/reviews/:review_id` | yes | TNM | `backend/internal/review/integration_test.go:195` |
| POST `/api/v1/reviews/:id/reports` | yes | TNM | `backend/internal/review/integration_test.go:278` |
| GET `/api/v1/service-offerings/:id/qa` | yes | TNM | `backend/internal/qa/integration_test.go:177` |
| POST `/api/v1/service-offerings/:id/qa` | yes | TNM | `backend/internal/qa/integration_test.go:104` |
| POST `/api/v1/service-offerings/:id/qa/:thread_id/replies` | yes | TNM | `backend/internal/qa/integration_test.go:127` |
| DELETE `/api/v1/qa/:post_id` | yes | TNM | `backend/internal/qa/integration_test.go:202` |
| GET `/api/v1/moderation/queue` | yes | TNM | `backend/internal/moderation/integration_test.go:326` |
| POST `/api/v1/moderation/queue/:id/approve` | yes | TNM | `backend/internal/moderation/integration_test.go:224` |
| POST `/api/v1/moderation/queue/:id/reject` | yes | TNM | `backend/internal/moderation/integration_test.go:271` |
| GET `/api/v1/moderation/actions` | yes | TNM | `backend/internal/securitytest/endpoint_coverage_test.go:602` |

## API Test Classification
- **True No-Mock HTTP:** present broadly via `router.New(...)` + `httptest.NewServer(...)` + real DB fixtures (`backend/internal/securitytest/setup_test.go:66`, `backend/internal/catalog/integration_test.go:25`, `backend/internal/review/integration_test.go:23`).
- **HTTP with mocking/synthetic route:** still present for middleware/handler-focused tests (`backend/internal/auth/handler_test.go:74`, `backend/internal/health/handler_test.go:26`, `backend/internal/middleware/csrf_test.go:17`).
- **Non-HTTP unit/service:** present across modules (`backend/internal/shipping/service_test.go:17`, `backend/internal/config/config_test.go:39`, `backend/internal/crypto/aes_test.go:1`).

## Mock Detection
- No `jest.mock`/`vi.mock`/`sinon.stub` found in backend API tests.
- Synthetic/mocked-style backend tests remain (intentionally unit-level):
  - synthetic handlers: `backend/internal/auth/handler_test.go:74`
  - middleware with injected context: `backend/internal/middleware/rbac_test.go:20`
  - nil DB path: `backend/internal/health/handler_test.go:26`
- Frontend unit tests use `vi.mock` for store/network boundaries (expected for unit layer): `frontend/tests/unit/authStore.spec.js:7`, `frontend/tests/unit/routerGuard.spec.js:19`.

## Coverage Summary
- Total endpoints: **101**
- Endpoints with HTTP tests: **101**
- Endpoints with true no-mock HTTP tests: **101**
- HTTP coverage: **100%**
- True API coverage: **100%**

## Unit Test Summary

### Backend Unit Tests
- Present for controllers/middleware/services/repositories-style logic.
- Examples:
  - auth/middleware: `backend/internal/middleware/rbac_test.go:41`, `backend/internal/middleware/csrf_test.go:39`
  - services: `backend/internal/ticket/service_test.go:412`, `backend/internal/moderation/service_test.go:1`
  - data/security: `backend/internal/hmacadmin/service_test.go:1`, `backend/internal/upload/safe_test.go:1`
- Important backend modules not tested: **no major untested core module identified** in static view; coverage now spans auth, admin, dataops, moderation, privacy, catalog, ticketing, notifications, ingest/lakehouse.

### Frontend Unit Tests (STRICT)
- **Frontend unit tests: PRESENT**
- Files:
  - `frontend/tests/unit/routerGuard.spec.js`
  - `frontend/tests/unit/authStore.spec.js`
  - `frontend/tests/unit/ticketCreateForm.spec.js`
  - `frontend/tests/unit/notificationDrawer.spec.js`
- Framework/tools detected:
  - Vitest + jsdom + Vue Test Utils (`frontend/vitest.config.js:15`, `frontend/package.json:10`, `frontend/package.json:24`)
- Real component/module imports confirmed:
  - `TicketCreateView.vue` import: `frontend/tests/unit/ticketCreateForm.spec.js:39`
  - `NotificationCenterDrawer.vue` import: `frontend/tests/unit/notificationDrawer.spec.js:20`
  - Auth store import: `frontend/tests/unit/authStore.spec.js:23`
- Important frontend modules not unit-tested (remaining):
  - no clear high-risk blind spot; however, moderation/admin views themselves are mostly e2e-covered rather than deep unit-covered.

### Cross-Layer Observation
- Test balance improved significantly: backend HTTP/security depth + frontend unit + frontend e2e all present.

## API Observability Check
- Strong on most new tests: explicit method/path/body/status and route-level expectation.
- Residual weak spots:
  - Some assertions allow multiple statuses (example: dataops run accepts 202 or 422): `backend/internal/securitytest/endpoint_coverage_test.go:453`.
  - A few legacy tests still reference stale paths not present in router (`/api/v1/profile`, `/api/v1/catalog/services`, `/api/v1/catalog/categories`): `backend/internal/profile/integration_test.go:272`, `backend/internal/catalog/integration_test.go:322`, `backend/internal/catalog/integration_test.go:342`.

## Test Quality & Sufficiency
- Happy paths, failures, authz, IDOR, CSRF, HMAC, moderation, throttling: broadly covered.
- `run_tests.sh` root entry now orchestrates all layers in one command and fails on any error:
  - backend: `run_tests.sh:55`
  - frontend unit: `run_tests.sh:61`
  - frontend e2e: `run_tests.sh:67`
  - compose service exists for frontend-unit-test: `deploy/docker/docker-compose.test.yml:40`

## End-to-End Expectations
- Fullstack expectation met: backend integration + frontend e2e + frontend unit.
- Evidence: `frontend/tests/e2e/auth.spec.js:1`, `frontend/tests/unit/routerGuard.spec.js:1`, `backend/internal/securitytest/endpoint_coverage_test.go:1`.

## Tests Check
- Endpoint inventory completeness: **PASS**
- API mapping completeness: **PASS**
- True no-mock API coverage: **PASS**
- Frontend unit requirement: **PASS**

## Test Coverage Score (0–100)
- **94/100**

## Score Rationale
- + Full endpoint HTTP + true-API coverage at 100%
- + Frontend unit gap closed
- + Root all-tests runner added
- - Some permissive assertions and stale legacy-path tests reduce strictness confidence

## Key Gaps
- Clean up stale-route tests (`/api/v1/profile`, `/api/v1/catalog/*`) to avoid maintenance confusion.
- Tighten permissive multi-status assertions where contract should be deterministic.

## Confidence & Assumptions
- Confidence: **high** for route/test mapping and README compliance.
- Assumption: endpoint registration source remains `backend/internal/router/router.go`.

---

# README Audit

## Hard Gate Evaluation
- README exists at required location: **PASS** (`README.md:1`).
- Formatting/readability: **PASS** (`README.md:18`, `README.md:186`).
- Startup instructions for fullstack include explicit `docker-compose up`: **PASS** (`README.md:66`).
- Access method (URL + port): **PASS** (`README.md:81`..`README.md:85`).
- Verification method present with concrete API/browser checks: **PASS** (`README.md:113`..`README.md:159`).
- Environment rules (no runtime local install/manual DB setup): **PASS** (`README.md:49`..`README.md:50`, `README.md:182`).
- Demo credentials for auth roles: **PASS** (`README.md:99`..`README.md:109`).

## High Priority Issues
- None.

## Medium Priority Issues
- None.

## Low Priority Issues
- Minor documentation mismatch in verification comment: health body key says `db="up"` while API model uses `database` field (`README.md:125`, `backend/internal/health/handler.go:13`).

## Hard Gate Failures
- None.

## README Verdict
- **PASS**

---

## Final Verdicts
- **Test Coverage Audit:** **PASS (strong)**
- **README Audit:** **PASS**
