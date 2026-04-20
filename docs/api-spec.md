# Local Service Commerce & Content Operations Portal - API Specification

All endpoints are served by the Go/Gin server under `/api/v1/*` over TLS.

- **Session auth:** HttpOnly cookie issued at login. State-changing requests must also carry `X-CSRF-Token` (minted per session, returned on login).
- **HMAC auth** (internal routes): `X-Key-ID: <key_id>` plus `X-Signature: hmac-sha256 <hex>` over `METHOD + "\n" + PATH + "\n" + hex(sha256(body))`.
- **Rate limits:** 60 requests/minute per authenticated user (or per IP when unauthenticated); 10 requests/hour per user for review create+update (shared bucket); 10/hour for abuse reports.
- **Errors:** JSON envelope `{ "error": { "code": "...", "message": "..." } }` with standard HTTP status.

## Public / Unauthenticated

| Method | Path                                          | Description                                    |
|--------|-----------------------------------------------|------------------------------------------------|
| GET    | `/health`                                     | Liveness + DB check                            |
| POST   | `/api/v1/auth/register`                       | Create account                                 |
| POST   | `/api/v1/auth/login`                          | Issue session cookie + CSRF token              |
| GET    | `/api/v1/service-categories`                  | List active service categories                 |
| GET    | `/api/v1/shipping/regions`                    | List shipping regions                          |
| GET    | `/api/v1/shipping/templates`                  | List shipping templates                        |
| GET    | `/api/v1/service-offerings/:id/reviews`       | List reviews for an offering                   |
| GET    | `/api/v1/service-offerings/:id/review-summary`| Aggregated review metrics (positive-rate, star histogram) |

## Authenticated Session

All routes below require a valid session, a matching `X-CSRF-Token` on non-GET requests, and pass through the 60/min general rate limiter.

### Auth & session

| Method | Path                        | Description                                    |
|--------|-----------------------------|------------------------------------------------|
| POST   | `/api/v1/auth/logout`       | Invalidate the current session                 |
| GET    | `/api/v1/auth/me`           | Current user + unread notification count       |

### Profile, preferences, favorites, history

| Method | Path                                             | Description                              |
|--------|--------------------------------------------------|------------------------------------------|
| GET    | `/api/v1/users/me/profile`                       | Profile (masked phone unless admin)      |
| PUT    | `/api/v1/users/me/profile`                       | Update profile (accepts plaintext phone) |
| GET    | `/api/v1/users/me/preferences`                   | Notification + muted tags/authors        |
| PUT    | `/api/v1/users/me/preferences`                   | Update preferences                       |
| GET    | `/api/v1/users/me/favorites`                     | Favorited offerings                      |
| POST   | `/api/v1/users/me/favorites`                     | Add favorite                             |
| DELETE | `/api/v1/users/me/favorites/:offering_id`        | Remove favorite                          |
| GET    | `/api/v1/users/me/history`                       | Browsing history                         |
| DELETE | `/api/v1/users/me/history`                       | Clear history                            |

### Address book (US-style)

| Method | Path                                     | Description                                   |
|--------|------------------------------------------|-----------------------------------------------|
| GET    | `/api/v1/users/me/addresses`             | List addresses (lines 1/2 decrypted on read)  |
| POST   | `/api/v1/users/me/addresses`             | Create; ZIP format + required city/state      |
| PUT    | `/api/v1/users/me/addresses/:id`         | Update                                        |
| DELETE | `/api/v1/users/me/addresses/:id`         | Delete                                        |
| PUT    | `/api/v1/users/me/addresses/:id/default` | Enforce single-default invariant              |

### Service offerings & shipping estimate

| Method | Path                                           | Roles                  | Description                                  |
|--------|------------------------------------------------|------------------------|----------------------------------------------|
| GET    | `/api/v1/service-offerings`                    | any authenticated      | Paginated list                               |
| GET    | `/api/v1/service-offerings/:id`                | any authenticated      | Offering detail                              |
| POST   | `/api/v1/service-offerings`                    | ServiceAgent, Admin    | Create offering                              |
| PUT    | `/api/v1/service-offerings/:id`                | ServiceAgent, Admin    | Update offering                              |
| PATCH  | `/api/v1/service-offerings/:id/status`         | ServiceAgent, Admin    | Enable/disable offering                      |
| POST   | `/api/v1/shipping/estimate`                    | any authenticated      | Fee + arrival window by region/weight/quantity |

### Tickets

| Method | Path                                                | Description                                                   |
|--------|-----------------------------------------------------|---------------------------------------------------------------|
| GET    | `/api/v1/tickets`                                   | List tickets scoped to the caller                             |
| POST   | `/api/v1/tickets`                                   | Create (blocked by `FreezeCheck`); attachments ≤5, ≤5 MB each |
| GET    | `/api/v1/tickets/:id`                               | Ticket detail with status + SLA timer                         |
| PATCH  | `/api/v1/tickets/:id/status`                        | Advance state (Accepted → Dispatched → In Service → Completed → Closed) or user cancel before Dispatch |
| GET    | `/api/v1/tickets/:id/notes`                         | List notes                                                    |
| POST   | `/api/v1/tickets/:id/notes`                         | Add note (`FreezeCheck` + `ScreenContent("content")`)         |
| GET    | `/api/v1/tickets/:id/attachments`                   | List attachments                                              |
| DELETE | `/api/v1/tickets/:id/attachments/:file_id`          | Delete attachment                                             |

### Reviews

Review create/update share a 10/hour bucket; reports use a separate 10/hour bucket.

| Method | Path                                              | Description                                              |
|--------|---------------------------------------------------|----------------------------------------------------------|
| POST   | `/api/v1/tickets/:id/reviews`                     | Create 1–5 star review with text + images                |
| PUT    | `/api/v1/tickets/:id/reviews/:review_id`          | Update review                                            |
| POST   | `/api/v1/reviews/:id/reports`                     | Report abusive review (throttled separately)             |

### Q&A

| Method | Path                                                           | Roles                      |
|--------|----------------------------------------------------------------|----------------------------|
| GET    | `/api/v1/service-offerings/:id/qa`                             | any authenticated          |
| POST   | `/api/v1/service-offerings/:id/qa`                             | RegularUser, Admin         |
| POST   | `/api/v1/service-offerings/:id/qa/:thread_id/replies`          | ServiceAgent, Admin        |
| DELETE | `/api/v1/qa/:post_id`                                          | Moderator, Admin           |

### Notifications

| Method | Path                                                 | Description                             |
|--------|------------------------------------------------------|-----------------------------------------|
| GET    | `/api/v1/users/me/notifications`                     | Notification list                       |
| GET    | `/api/v1/users/me/notifications/unread-count`        | Unread count badge                      |
| GET    | `/api/v1/users/me/notifications/outbox`              | Outbox for non-in-app channels          |
| PATCH  | `/api/v1/users/me/notifications/read-all`            | Mark all read                           |
| PATCH  | `/api/v1/users/me/notifications/:id/read`            | Mark one read                           |

### Privacy center

| Method | Path                                           | Description                                 |
|--------|------------------------------------------------|---------------------------------------------|
| POST   | `/api/v1/users/me/export-request`              | Queue export job                            |
| GET    | `/api/v1/users/me/export-request/status`       | Poll status                                 |
| GET    | `/api/v1/users/me/export-request/download`     | Download the generated export file          |
| POST   | `/api/v1/users/me/deletion-request`            | Request deletion (30-day grace)             |
| GET    | `/api/v1/users/me/deletion-request/status`     | Poll deletion status                        |

### Moderation (Moderator or Admin)

| Method | Path                                      | Description                                |
|--------|-------------------------------------------|--------------------------------------------|
| GET    | `/api/v1/moderation/queue`                | List pending items                         |
| POST   | `/api/v1/moderation/queue/:id/approve`    | Approve (no violation recorded)            |
| POST   | `/api/v1/moderation/queue/:id/reject`     | Reject (records violation, extends freeze) |
| GET    | `/api/v1/moderation/actions`              | Moderator action history                   |

### Data operator (Data Operator or Admin)

| Method | Path                                                | Description                                |
|--------|-----------------------------------------------------|--------------------------------------------|
| GET    | `/api/v1/dataops/sources`                           | List ingest sources                        |
| POST   | `/api/v1/dataops/sources`                           | Create source                              |
| PUT    | `/api/v1/dataops/sources/:id`                       | Update source                              |
| GET    | `/api/v1/dataops/jobs`                              | List ingest jobs                           |
| POST   | `/api/v1/dataops/jobs`                              | Create job                                 |
| GET    | `/api/v1/dataops/jobs/:id`                          | Job detail                                 |
| POST   | `/api/v1/dataops/jobs/:id/run`                      | Trigger a run (resumes from checkpoint)    |
| GET    | `/api/v1/dataops/schema-versions/:source_id`        | Schema evolution history                   |
| GET    | `/api/v1/dataops/catalog`                           | Lakehouse catalog listing                  |
| GET    | `/api/v1/dataops/catalog/:id`                       | Catalog entry detail                       |
| GET    | `/api/v1/dataops/lineage/:id`                       | Lineage edges                              |

### Administrator

| Method | Path                                                   | Description                                    |
|--------|--------------------------------------------------------|------------------------------------------------|
| GET    | `/api/v1/admin/hmac-keys`                              | List HMAC keys (metadata only)                 |
| POST   | `/api/v1/admin/hmac-keys`                              | Create key; plaintext returned once            |
| POST   | `/api/v1/admin/hmac-keys/rotate`                       | Hard-swap existing key's secret                |
| DELETE | `/api/v1/admin/hmac-keys/:id`                          | Revoke                                         |
| POST   | `/api/v1/admin/service-categories`                     | Create category                                |
| PUT    | `/api/v1/admin/service-categories/:id`                 | Update category                                |
| DELETE | `/api/v1/admin/service-categories/:id`                 | Delete category                                |
| POST   | `/api/v1/admin/shipping/regions`                       | Create region                                  |
| POST   | `/api/v1/admin/shipping/templates`                     | Create template                                |
| PUT    | `/api/v1/admin/shipping/templates/:id`                 | Update template                                |
| GET    | `/api/v1/admin/notification-templates`                 | List templates                                 |
| PUT    | `/api/v1/admin/notification-templates/:code`           | Upsert template                                |
| GET    | `/api/v1/admin/sensitive-terms`                        | List dictionary                                |
| POST   | `/api/v1/admin/sensitive-terms`                        | Add term (prohibited or borderline)            |
| DELETE | `/api/v1/admin/sensitive-terms/:id`                    | Remove term                                    |
| GET    | `/api/v1/admin/users/:user_id/violations`              | Per-user violation history                     |
| GET    | `/api/v1/admin/audit-logs`                             | Append-only audit log                          |
| DELETE | `/api/v1/admin/users/:user_id`                         | Hard-delete user (privacy compliance)          |
| GET    | `/api/v1/admin/legal-holds`                            | List legal holds                               |
| POST   | `/api/v1/admin/legal-holds`                            | Place hold on source/job                       |
| DELETE | `/api/v1/admin/legal-holds/:id`                        | Release hold                                   |
| POST   | `/api/v1/admin/lakehouse/lifecycle/run`                | Trigger on-demand archive + purge sweep        |

## HMAC-Protected Internal Routes

For ingestion workers. Every request must carry `X-Key-ID` and `X-Signature: hmac-sha256 <hex>` over `METHOD\nPATH\nhex(sha256(body))`. No session/CSRF is required.

| Method | Path                                                   | Description                              |
|--------|--------------------------------------------------------|------------------------------------------|
| GET    | `/api/v1/internal/data/sources`                        | List sources                             |
| POST   | `/api/v1/internal/data/sources`                        | Register source                          |
| PUT    | `/api/v1/internal/data/sources/:id`                    | Update source                            |
| GET    | `/api/v1/internal/data/jobs`                           | List jobs                                |
| POST   | `/api/v1/internal/data/jobs`                           | Create job                               |
| GET    | `/api/v1/internal/data/jobs/:id`                       | Job detail                               |
| GET    | `/api/v1/internal/data/schema-versions/:source_id`     | Schema evolution                         |
| GET    | `/api/v1/internal/data/catalog`                        | Lakehouse catalog                        |
| GET    | `/api/v1/internal/data/catalog/:id`                    | Catalog entry                            |
| GET    | `/api/v1/internal/data/lineage/:id`                    | Lineage edges                            |

## Conventions

- **Timestamps:** RFC3339 UTC.
- **IDs:** unsigned 64-bit integers serialized as JSON numbers.
- **Pagination:** `?page=` + `?page_size=`; responses include `items` and `total`.
- **Masking:** phone numbers appear as `(NXX) ***-XXXX` for non-admin readers; owning user receives plaintext on their own `GET /users/me/profile` only when editing flows explicitly require it.
- **Attachments:** uploads stored under `storage/uploads/`; JPG/PNG/PDF, ≤5 MB each, max 5 per ticket.
- **Lockout response:** 423 with a body describing the remaining lock duration.
- **Rate-limit response:** 429 with `Retry-After` header.
