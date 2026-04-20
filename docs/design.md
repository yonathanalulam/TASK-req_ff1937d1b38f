# Local Service Commerce & Content Operations Portal - Design

## 1. Purpose & Scope

The portal gives field-service providers an offline-first way to publish service offerings, take customer requests, and govern user-generated content across five roles: Regular User, Service Agent, Moderator, Administrator, and Data Operator. The UI is English-only with inline validation, toast confirmations, and status badges. The system runs on a local network with TLS, persists everything in a MySQL instance it controls, and does not reach external services — ingestion pulls from local DB tables, log files, and filesystem drops only.

## 2. High-Level Architecture

```
┌────────────────────────────┐        HTTPS (local TLS)       ┌──────────────────────────┐
│  Vue.js 3 SPA              │ ─────── cookies + CSRF ──────▶ │  Go/Gin API              │
│  Pinia stores per domain   │                                │  RBAC + rate limit +     │
│  Vue Router + AppShell     │                                │  CSRF + HMAC middleware  │
└────────────────────────────┘                                └──────────┬───────────────┘
                                                                         │
                               ┌─────────────────────────────────────────┼───────────────────────────┐
                               ▼                                         ▼                           ▼
                   ┌────────────────────┐                   ┌───────────────────────┐   ┌───────────────────────────┐
                   │  MySQL             │                   │ Local filesystem       │   │ Background goroutines     │
                   │  users, sessions,  │                   │ storage/uploads        │   │ SLA engine                │
                   │  addresses,        │                   │ storage/exports        │   │ Privacy export/deletion   │
                   │  tickets, reviews, │                   │ storage/lakehouse/     │   │ Lakehouse lifecycle       │
                   │  moderation,       │                   │   {bronze,silver,gold} │   │   (archive 90d,           │
                   │  notifications,    │                   │ storage/backups/       │   │    purge 18mo,            │
                   │  ingest + lineage  │                   │   lakehouse/           │   │    legal-hold aware)      │
                   └────────────────────┘                   └───────────────────────┘   └───────────────────────────┘
```

Two API surfaces coexist: the session-authenticated `/api/v1/...` group consumed by the SPA, and an HMAC-protected `/api/v1/internal/...` group for machine clients (ingestion workers). The `/api/v1/dataops/...` surface mirrors a subset of the internal endpoints for human Data Operators so the same primitives are reachable via either transport.

## 3. Backend Structure (`repo/backend`)

Layered by domain package under `internal/`. Each package owns its models, service, handler, and SQL.

| Package             | Responsibility                                                                 |
|---------------------|---------------------------------------------------------------------------------|
| `auth`              | Registration, login, logout, session issuance, failed-attempt lockout tracking |
| `session`           | Session row lifecycle, CSRF token minting, inactivity touch (30 min)           |
| `middleware`        | `RequireAuth`, `CSRF.Validate`, `NewGeneralLimiter` (60/min), `NewReviewReportLimiter` (10/hr), `NewHMACVerifier`, `RequireRole` |
| `crypto`            | AES-256-GCM `Encrypt`/`Decrypt` and HMAC-SHA256 sign/verify                    |
| `profile`           | User profile (encrypted phone), preferences (muted tags/authors), favorites, browsing history, `MaskPhone` |
| `address`           | US address CRUD, ZIP validation, single-default enforcement, encrypted lines 1/2 |
| `catalog`           | Service categories (admin-managed) and service offerings (agents/admins)      |
| `shipping`          | Regions, weight/quantity templates, estimate calculation with cutoff-based ETA |
| `ticket`            | Ticket lifecycle (Accepted → Dispatched → In Service → Completed → Closed), notes, attachments (JPG/PNG/PDF, ≤5 MB, ≤5 files), background SLA engine |
| `review`            | 1–5 star reviews with text + images, summary aggregation, abuse reports       |
| `qa`                | Pre-service Q&A threads, moderator delete                                      |
| `moderation`        | Local sensitive-term dictionary, `Screen`, `OnBorderlineFlagged`, `FreezeCheck`, escalating freeze (24 h → 7 d) |
| `notification`      | Templated notifications, outbox view for non-in-app channels, seeded defaults  |
| `privacy`           | Export requests (downloadable local file), deletion workflow (30-day anonymization, 7-year audit retention), admin hard-delete |
| `audit`             | Append-only audit log, admin listing                                           |
| `hmacadmin`         | HMAC key issuance, rotation (hard swap), revocation, one-time plaintext reveal |
| `ingest`            | Data sources, jobs, schema versions, resumable checkpoints (updated_at or offset) |
| `lakehouse`         | Bronze/silver/gold writes on local disk, metadata catalog, lineage, lifecycle (90-day archive, 18-month purge, legal hold) |
| `router`            | Wires middleware, services, background workers, and routes into a single Gin engine |
| `config`, `db`, `health`, `testutil`, `securitytest`, `apierr`, `upload`, `bgjob`, `models` | Shared plumbing |

### 3.1 Request Pipeline

1. `gin.Recovery` + request logger + CORS (exposes `X-CSRF-Token`, `X-Key-ID`, `X-Signature`).
2. For public auth: `generalRL.Limit()` keyed on IP so login brute-force is still bounded.
3. For protected routes: `RequireAuth` → `CSRF.Validate` → `generalRL.Limit()` (keyed on user) → optional `RequireRole` / `FreezeCheck` / `ScreenContent` / per-action limiter.
4. For `/internal/*`: `HMACVerifier.ValidateHMAC` replaces the session/CSRF stack; each request must carry `X-Key-ID` and `X-Signature: hmac-sha256 <hex>` over `METHOD\nPATH\nhex(sha256(body))`.

### 3.2 Data at Rest

Field-level encryption is AES-256-GCM with a per-record 12-byte random nonce, stored as `nonce || ciphertext || tag` in `VARBINARY` columns (`users.phone_encrypted`, `addresses.address_line1_encrypted`, `addresses.address_line2_encrypted`, `hmac_keys.secret_encrypted`). The master key is loaded once at startup from `FIELD_ENCRYPTION_KEY` (hex-decoded to 32 bytes). Decryption is lazy on read at the service boundary; responses apply masking (`MaskPhone`) before serialization for non-admin consumers.

### 3.3 Background Workers

Started from `router.New` as goroutines with a shared `context.Background()`:

- `ticket.StartSLAEngine` — scans open tickets, fires notifications on transitions and SLA breaches.
- `privacy.StartExportWorker` / `StartDeletionWorker` — async fulfillment of user-initiated export/deletion.
- `lakehouse.StartLifecycleWorker` — daily archive (bronze ≥ 90 days) and purge (archived ≥ 18 months), skipping anything under an active legal hold.

## 4. Frontend Structure (`repo/frontend`)

Vue 3 SPA built with Vite. State per domain lives in Pinia stores; views are routed under an `AppShell` that hosts the `NotificationCenterDrawer` and `ToastMessage` outlet.

```
src/
  App.vue, main.js
  router/index.js        ── guards auth + role per route
  components/            ── AppShell, modals (Address, Review), widgets (ShippingEstimate, ReviewSummary), ToastMessage, NotificationCenterDrawer
  composables/useToast.js
  stores/                ── auth, profile, address, catalog, shipping, ticket, review, qa, notification, moderation, privacy, dataops, hmacKeys
  views/                 ── Login, Dashboard, Profile, Preferences, AddressBook, ServiceCatalog, ServiceOfferingDetail, ServiceOfferingForm,
                            TicketList, TicketCreate, TicketDetail, ReviewList, QAThread, NotificationOutbox, ModerationQueue,
                            ViolationHistory, PrivacyCenter, DataOps, LegalHold, HMACKeys, Health
```

The `auth` store holds the session CSRF token returned on login; the HTTP client attaches it as `X-CSRF-Token` on every state-changing request. Masked values arrive pre-masked from the server — the UI never un-masks.

## 5. Role-Based Access

| Role              | Capability surface                                                                       |
|-------------------|-------------------------------------------------------------------------------------------|
| Regular User      | Profile, preferences, addresses, catalog browse, tickets (create/cancel-before-dispatch), reviews, Q&A threads, notifications, privacy center |
| Service Agent     | Offering create/update, ticket operational transitions, Q&A replies                       |
| Moderator         | Moderation queue (approve/reject), Q&A post delete, user violation lookup                 |
| Administrator     | Everything above, category management, shipping regions/templates, notification templates, sensitive terms, HMAC keys, legal holds, audit log, hard delete |
| Data Operator     | `/dataops/*` — sources, jobs (with run), schema versions, lakehouse catalog + lineage     |

`RequireRole` is applied as a route middleware; Administrator is accepted as a superset wherever a narrower role is required.

## 6. Security Posture

- TLS on all transport (including local network).
- HttpOnly, SameSite session cookie with a 24-hour absolute cap and 30-minute inactivity timeout (`session.InactivityTimeout`).
- CSRF token bound to the session row, validated per state-changing request.
- Login lockout: 5 failures in a 10-minute sliding window → 15-minute lock (`auth.Service.checkLockout`), one notification + audit entry per lock.
- Rate limits: 60/min general, 10/hr shared across review create/update, 10/hr for abuse reports (sliding window in-memory).
- HMAC signing for internal clients with one-time plaintext reveal on creation, hard-swap rotation, and constant-time verification.
- Content governance: prohibited terms blocked pre-write, borderline terms queued for moderators, violations drive escalating posting freezes (24 h then 7 d).
- Privacy: user-initiated export (downloadable local file) and deletion (purge/anonymize after 30 days, audit retained 7 years).

## 7. Lakehouse & Ingestion

Ingest sources declare a checkpoint strategy (`updated_at` or monotonic `offset`) stored in `ingest_checkpoints`. Jobs resume from the latest checkpoint and emit row-count and schema checks against `schema_versions`. Writes land in `storage/lakehouse/<layer>/<source_id>/<YYYY-MM-DD>/<nanos>.dat` with metadata in `lakehouse_metadata` and lineage edges in `lakehouse_lineage`. The lifecycle worker archives bronze older than 90 days to `storage/backups/lakehouse/` and purges archived data older than 18 months; `legal_holds` rows short-circuit both operations until released.

## 8. Deployment

`docker-compose.yml` at the repo root composes MySQL plus the Go server; `deploy/` and `scripts/` contain local-network provisioning helpers. The frontend is served as a static build (`vite build`) pointed at the Go server's TLS listener. All persisted artifacts (uploads, exports, lakehouse files, backups) live under `storage/` on the server's local disk.
