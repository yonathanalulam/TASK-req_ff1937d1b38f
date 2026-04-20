# Local Service Commerce & Content Operations Portal

> **Project type: `fullstack`** — Vue 3 SPA + Go REST API + MySQL, all orchestrated by Docker Compose.

An offline-first web platform that lets field-service providers publish service
offerings, manage customer requests end-to-end, and govern user-generated
content — all without external network dependencies.

The system covers profile and address management, a service catalog with
region-based shipping fee calculation, a full ticket lifecycle with SLA timers
and file attachments, post-completion reviews and Q&A, an in-app notification
center, content moderation with escalating freezes, user-initiated data export
and deletion (GDPR-style), and a Bronze/Silver/Gold lakehouse with HMAC-protected
ingestion APIs.

---

## Stack

| Layer    | Technology                                            |
|----------|-------------------------------------------------------|
| Frontend | Vue 3 (JavaScript), Pinia, Vue Router, Vite, nginx    |
| Backend  | Go 1.22, Gin                                          |
| Database | MySQL 8.0                                             |
| Auth     | HttpOnly session cookies, CSRF tokens, bcrypt cost 12 |
| Crypto   | AES-256-GCM (field-level), HMAC-SHA256 (internal API) |
| Tests    | Go `testing` (unit + integration), Playwright (E2E)   |
| Runtime  | Docker + docker-compose                               |

---

## Features

- **Five-role RBAC** — Regular User, Service Agent, Moderator, Administrator, Data Operator
- **Encrypted PII** — phone numbers and address lines stored with AES-256-GCM
- **Account hardening** — 30-min inactivity timeout, 24-hr absolute timeout, 5-fail/10-min lockout, 60 req/min general rate limit
- **Service catalog** — categories, offerings, favorites, browsing history, region-aware shipping with cutoff-based ETA
- **Ticket lifecycle** — Accepted → Dispatched → In Service → Completed → Closed (or Cancelled), SLA tracking, multipart attachments (JPG/PNG/PDF, max 5 files × 5 MB)
- **Reviews & Q&A** — 1–5 star reviews with image uploads, aggregated metrics, moderator-only post deletion
- **Notifications** — templated in-app messages with outbox routing for users who disable in-app delivery
- **Content moderation** — sensitive-term dictionary, automatic prohibited-content blocking, borderline review queue, escalating posting freeze (24h → 7d)
- **Privacy center** — user-initiated ZIP data export and GDPR-style deletion with 30-day grace period and irreversible-action confirmation
- **Lakehouse** — Bronze/Silver/Gold layers on local disk, lineage tracking, schema evolution detection, lifecycle archival/purge, legal-hold protection

---

## Prerequisites

- **Docker 20.10+** and **docker-compose** (v1 or v2 plugin)
- That's it. No local Go, Node, or MySQL installation required.

---

## Start the application

### Option A — wrapper script (recommended)

```bash
./scripts/start.sh
```

### Option B — direct docker-compose

```bash
cp .env.example .env                            # first run only
docker-compose -f deploy/docker/docker-compose.yml --env-file .env up --build
```

Either path performs the same workflow:

1. Copies `.env.example` to `.env` if you don't have one yet
2. Pulls `mysql:8.0` from Docker Hub on first run
3. Builds the backend and frontend images locally
4. Brings up MySQL and waits for the healthcheck
5. Runs database migrations (16 schema files)
6. Seeds reference data (users, categories, offerings, shipping templates)
7. Starts the backend API and the frontend

When startup finishes you can access:

| URL                             | What                                       |
|---------------------------------|--------------------------------------------|
| http://localhost:5173           | Frontend web app (SPA proxies to backend)  |
| https://localhost:8080          | Backend REST API (self-signed in dev)      |
| https://localhost:8080/health   | Health probe (DB + status)                 |

The backend terminates TLS in all environments. In dev the cert is
self-signed, so a browser hitting `https://localhost:8080` directly will
show a warning — routine SPA traffic goes through the frontend's nginx
proxy and does not require the user to trust the cert.

Stop everything with `Ctrl+C`, or fully tear down with:

```bash
cd deploy/docker && docker-compose down       # keep volumes (data persists)
cd deploy/docker && docker-compose down -v    # also delete volumes (fresh start)
```

### Seed accounts

The seed script creates one user per role. Password for all of them is `password`.

| Username        | Role           |
|-----------------|----------------|
| `regular_user`  | regular_user   |
| `service_agent` | service_agent  |
| `moderator`     | moderator      |
| `admin`         | administrator  |
| `data_operator` | data_operator  |

---

## Verify the running stack

### Backend API (curl)

The backend terminates TLS with a self-signed certificate in dev, so `-k` skips
trust validation. Every successful request is expected to return the documented
status; any other status indicates a broken deployment.

```bash
# 1. Liveness — DB connectivity + app status
curl -sk -o /dev/null -w "%{http_code}\n" https://localhost:8080/health
# Expected: 200
# Body keys: status="ok", db="up"

# 2. Login a seeded user (cookie jar captures sp_session + CSRF)
curl -sk -c cookies.txt -H 'Content-Type: application/json' \
  -d '{"username":"regular_user","password":"password"}' \
  https://localhost:8080/api/v1/auth/login
# Expected: 200
# Body keys: user{id,username,roles[]}, csrf_token

# 3. Authenticated session probe
curl -sk -b cookies.txt https://localhost:8080/api/v1/auth/me
# Expected: 200
# Body keys: user{id,username}, csrf_token, unread_count

# 4. Public catalog browse
curl -sk https://localhost:8080/api/v1/service-categories
# Expected: 200; array of {id,name,slug,response_time_minutes}

# 5. RBAC denial (seeded regular_user is not admin)
curl -sk -b cookies.txt -o /dev/null -w "%{http_code}\n" \
  https://localhost:8080/api/v1/admin/hmac-keys
# Expected: 403
```

### Frontend (browser)

1. Navigate to <http://localhost:5173>. The login form loads without console errors.
2. Sign in as `regular_user` / `password`. You are redirected to `/dashboard`.
3. Open the notification drawer from the top bar. It renders empty-state or seeded items without a spinner stall.
4. Navigate to `/tickets/new`. The submit button remains disabled until every required field is filled; validation errors appear inline as you edit.
5. Sign out. You return to `/login` and `/dashboard` is no longer reachable.
6. Sign in as `admin` / `password`. The **Moderation** and **HMAC Keys** navigation links are visible; sign back in as `regular_user` and confirm those links are hidden (route-level role guard also redirects direct URL visits to `/dashboard`).

Any deviation (non-200 API response, visible stack trace, missing redirect, or
visible admin link for `regular_user`) is a verification failure.

---

## Run the test suite

```bash
./run_tests.sh            # root entrypoint — backend + frontend unit + Playwright e2e
```

Equivalent shortcuts exist for individual layers:

```bash
./scripts/run-tests.sh    # backend + e2e (no frontend unit tests)
```

This single command:

1. Builds the test images in parallel — `Dockerfile.backend-test` (golang:1.22-alpine with the full backend source tree) and `Dockerfile.frontend-test` (the official Playwright image with browser binaries preinstalled).
2. Runs **backend tests** — `go test -count=1 ./...` inside the backend test image, against an isolated `service_portal_test` database. Covers unit tests, package integration tests, HTTP-level integration tests, and dedicated security tests. `-count=1` disables the Go test cache so results reflect the current code every run.
3. Runs **Playwright E2E tests** — `npx playwright test` against the freshly built frontend (nginx) and backend (Gin) services on the compose network. Real Chromium, not a placeholder.
4. Exits non-zero on **any** test failure — orchestrated via `set -euo pipefail` in `scripts/run-tests.sh` and the actual exit codes from the test containers. A failing backend test or failing Playwright spec stops the pipeline.

No setup, no extra installs — Docker handles everything.

---

## Project layout

```
repo/
├── frontend/                    Vue 3 application
│   ├── src/
│   │   ├── views/               Page-level components (one per route)
│   │   ├── components/          Reusable UI (modals, drawers, widgets)
│   │   ├── stores/              Pinia stores (one per domain)
│   │   ├── router/              Vue Router config + auth guard
│   │   └── composables/         Shared composables (toast, etc.)
│   └── tests/e2e/               Playwright spec files
├── backend/
│   ├── cmd/server/              Single binary entry point (server | migrate | seed | test)
│   └── internal/
│       ├── apierr/              Standard JSON error envelope
│       ├── auth/                Session login, register, lockout
│       ├── audit/               Append-only audit log writer
│       ├── address/             Address book CRUD with AES-encrypted lines
│       ├── catalog/             Service categories + offerings
│       ├── config/              Env-loaded configuration
│       ├── crypto/              AES-256-GCM and HMAC-SHA256 helpers
│       ├── db/                  Migrations, seed runner, embedded SQL
│       ├── health/              Health check endpoint
│       ├── ingest/              Source registry + jobs + checkpoints + schema evolution
│       ├── lakehouse/           Bronze/Silver/Gold writers + lineage + lifecycle
│       ├── middleware/          Auth, CSRF, rate-limit, HMAC, RBAC
│       ├── models/              Shared struct definitions
│       ├── moderation/          Term dictionary, screening middleware, queue, freeze engine
│       ├── notification/        Template rendering, dispatch, list, outbox
│       ├── privacy/             Data export ZIP + deletion/anonymization workers
│       ├── profile/             Profile, preferences, favorites, history
│       ├── qa/                  Q&A threads + replies
│       ├── review/              Reviews, images, reports, summary
│       ├── router/              Route registration + cross-package wiring
│       ├── session/             Session store + cookie management
│       ├── shipping/            Region/template CRUD + ETA calculation
│       ├── testutil/            Integration test helpers (DBOrSkip, TruncateTables)
│       └── ticket/              Ticket CRUD + SLA engine + transition matrix
├── deploy/
│   ├── docker/                  Dockerfiles + docker-compose.yml + docker-compose.test.yml
│   └── seed/                    SQL files run by the seed step
├── storage/                     Persistent runtime data (uploads, exports, lakehouse, backups)
├── scripts/
│   ├── start.sh                 ← Start the application
│   └── run-tests.sh             ← Run all tests
└── README.md
```

---

## Configuration

All configuration is read from `.env` at the repo root (auto-created from
`.env.example` on first start). Key variables:

| Variable                         | Purpose                                             | Default                                              |
|----------------------------------|-----------------------------------------------------|------------------------------------------------------|
| `DB_NAME`                        | Application database name                           | `service_portal`                                     |
| `DB_TEST_NAME`                   | Isolated database used by the test runner           | `service_portal_test`                                |
| `DB_USER` / `DB_PASSWORD`        | Application DB credentials                          | `portal_user` / `portalpassword`                     |
| `FIELD_ENCRYPTION_KEY`           | 64-char hex (32-byte) AES-256 key for encrypted PII | dev placeholder; **must be rotated for production**  |
| `APP_ENV`                        | `development` / `production` / `test`               | `development`                                        |
| `FRONTEND_PORT`                  | Host port for the Vite-built frontend               | `5173`                                               |
| `BACKEND_PORT`                   | Host port for the Gin API                           | `8080`                                               |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | TLS certificate + key paths (**required**, see TLS section below) | `/app/storage/tls/dev.{crt,key}`                    |
| `AUTH_LOGIN_DEBUG`               | Emit per-attempt login diagnostic logs (dev only)   | unset                                                |

---

## TLS transport (mandatory everywhere)

The backend **requires TLS in every environment** — development, test, and
production alike. There is no HTTP fallback. Both `TLS_CERT_FILE` and
`TLS_KEY_FILE` must be set and point at readable PEM files on disk; startup
fails fast at two points if they aren't:

- `config.Load` rejects empty `TLS_CERT_FILE` / `TLS_KEY_FILE` as a config
  error. No code path constructs a valid `*config.Config` without TLS paths.
- `runServer` verifies both files exist and are readable, then calls
  `ListenAndServeTLS`. A missing file produces an actionable error that names
  the path and points at the generator script.

The `./scripts/start.sh` and `./scripts/run-tests.sh` helpers invoke
`./scripts/gen-tls-cert.sh` before bringing up docker-compose so that a
self-signed pair always exists at the default paths:

```
storage/tls/dev.crt
storage/tls/dev.key
```

The cert is valid for `CN=localhost` with SANs for `localhost`, `backend`
(docker service name), and `127.0.0.1`. It is suitable for local dev and
automated tests only — **do not use it in production**. For production,
replace the files with a cert issued by your CA and update `TLS_CERT_FILE` /
`TLS_KEY_FILE` to point at them.

### Inside docker-compose

- The backend container reads the cert from `/app/storage/tls/` (the repo's
  `storage/tls/` directory is bind-mounted read-only).
- nginx in the frontend container proxies `/api/*` to the backend over HTTPS
  with `proxy_ssl_verify off` — both ends of the hop live on the internal
  compose network and terminate the ephemeral self-signed cert.
- The browser-facing frontend is served from nginx on port 5173; the SPA uses
  relative `/api/*` paths (default `VITE_API_BASE_URL=`), so the browser
  never has to trust the self-signed cert directly.

### Regenerate or replace the cert

```bash
./scripts/gen-tls-cert.sh           # no-op if cert already exists
./scripts/gen-tls-cert.sh --force   # regenerate (e.g. expired dev cert)
```

---

## Data Operator role

The `data_operator` role owns the ingestion + lakehouse operational surface.
It is a peer of `administrator` for that surface; `administrator` retains a
superset of permissions so ops can be delegated without weakening admin.

Backend routes (all session-auth + CSRF + RBAC):

| Method | Path                                     | Allowed roles                 |
|--------|------------------------------------------|-------------------------------|
| GET    | `/api/v1/dataops/sources`                | data_operator, administrator  |
| POST   | `/api/v1/dataops/sources`                | data_operator, administrator  |
| PUT    | `/api/v1/dataops/sources/:id`            | data_operator, administrator  |
| GET    | `/api/v1/dataops/jobs`                   | data_operator, administrator  |
| POST   | `/api/v1/dataops/jobs`                   | data_operator, administrator  |
| GET    | `/api/v1/dataops/jobs/:id`               | data_operator, administrator  |
| POST   | `/api/v1/dataops/jobs/:id/run`           | data_operator, administrator  |
| GET    | `/api/v1/dataops/schema-versions/:id`    | data_operator, administrator  |
| GET    | `/api/v1/dataops/catalog[?source_id=]`   | data_operator, administrator  |
| GET    | `/api/v1/dataops/catalog/:id`            | data_operator, administrator  |
| GET    | `/api/v1/dataops/lineage/:id`            | data_operator, administrator  |
| POST   | `/api/v1/admin/lakehouse/lifecycle/run`  | administrator                 |
| *      | `/api/v1/admin/legal-holds*`             | administrator                 |

Frontend: the `/dataops` route and `Data Ops` nav link are visible to
`data_operator` and `administrator`. The route guard enforces the same RBAC
declared in each route's `meta.roles` array, keeping frontend visibility and
backend authorization in lockstep.

The HMAC-protected `/api/v1/internal/data/*` surface is preserved for machine
clients. It is not replaced by `/dataops` — they serve different consumers
(browser vs. signed HTTP client).

---

## Ingestion + lifecycle behavior

- `POST /api/v1/dataops/jobs/:id/run` executes the job end-to-end. The runner
  reads the source config, pulls records (db_table / log_file /
  filesystem_drop), records a schema version, writes Bronze + Silver + Gold
  files under `storage/lakehouse/`, captures a resumable checkpoint
  (`updated_at` or `offset`), and persists status/row counts/error on the job
  row. Row-count tolerance is 0.1%.
- The lakehouse lifecycle worker runs on a 24-hour cadence (daily sweep):
  archive Bronze files older than 90 days, purge archived files older than
  ~18 months. Any file whose `source_id` or `job_id` has an active legal hold
  is counted in `Held` and skipped — legal holds are honored by both the
  scheduled worker and the on-demand admin trigger endpoint.
- Administrators can trigger a sweep on demand via
  `POST /api/v1/admin/lakehouse/lifecycle/run` (optional `archive_days` and
  `purge_days` query params override the defaults for ad-hoc sweeps).

---

## API surface

All routes live under `/api/v1/`. State-changing requests require both a
session cookie and the `X-CSRF-Token` header obtained from `GET /auth/me`.
Internal data-pipeline routes under `/api/v1/internal/` require an HMAC-signed
request (`X-Key-ID` + `X-Signature: hmac-sha256 <hex>`).

Standard error envelope:

```json
{ "error": { "code": "validation_error", "message": "...", "details": {} } }
```

Cursor pagination shape on list endpoints:

```json
{ "items": [...], "next_cursor": 123 }
```

---

## Security highlights

- **Sessions** — opaque random IDs in HttpOnly cookies; CSRF tokens validated on every mutating request
- **Account lockout** — 5 failed logins in 10 minutes triggers a 15-minute lock; lockout event also fires a notification + audit log entry
- **AES-256-GCM** — phone number and address lines encrypted at rest; nonces generated per-record
- **HMAC-SHA256** — internal ingestion endpoints require signed `METHOD\nPATH\nsha256(body)` payloads with rotatable `hmac_keys`
- **Content moderation** — case-insensitive whole-word matching, prohibited terms blocked at the middleware layer (422 `content_blocked`), borderline content queued and demoted to `pending_moderation`
- **Posting freeze** — first violation = 24h freeze, second = 7-day freeze, attempts during a freeze return 403 `posting_frozen` with `freeze_until`
- **Audit log** — append-only `audit_logs` table; no UPDATE or DELETE endpoints; retention sweep removes rows older than 7 years

---

## Offline-first guarantees

- No outbound network calls anywhere in the codebase
- All MySQL, file storage, and lakehouse layers run on the local Docker network
- Embedded `time/tzdata` ensures timezone math works in minimal containers without system tz files
- Migrations and seed data are bundled into the backend binary via `embed`
