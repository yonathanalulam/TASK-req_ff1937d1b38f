# Local Service Commerce & Content Operations Portal - Clarification Questions

## 1. Field-Level Encryption: Key Derivation, Nonce Strategy, and Column Layout

**Question:** The prompt mandates AES-256 encryption at rest for sensitive fields such as phone numbers and address lines, but it does not specify whether a KDF should be applied to the configured secret, how per-record IVs/nonces are produced, whether the authentication tag is stored alongside the ciphertext, or how the ciphertext is physically laid out in MySQL. A naive implementation (ECB mode, static IV, no authentication tag, or a hex-encoded value stuffed into a `VARCHAR`) would satisfy "AES-256" on paper while being trivially broken in practice. What is the contract?

**My Understanding:** The stored value must be authenticated (GCM), each record must use a fresh random nonce to avoid catastrophic nonce reuse, and the nonce must travel with the ciphertext so a single `VARBINARY` column is self-contained. The configured secret should be treated as the raw 256-bit key (hex-decoded on load) rather than being passed through a KDF on every request — that way the cost is paid once at startup and a downstream operator can rotate by replacing the environment value.

**Solution:** `internal/crypto/aes.go` exposes `Encrypt` / `Decrypt` built on AES-256-GCM. The key is hex-decoded from the `FIELD_ENCRYPTION_KEY` environment variable and validated to be exactly 32 bytes before the first call. For each encrypt, a 12-byte nonce is drawn from `crypto/rand` and prepended to the GCM output, so the on-disk format is `nonce || ciphertext || tag` stored as `VARBINARY`. Both `users.phone_encrypted` and `addresses.address_line1_encrypted` / `address_line2_encrypted` follow this layout; the profile and address services decrypt lazily on read (see `internal/address/service.go`) and re-encrypt on every write so rotation is a matter of re-running an ingest pass with a new key rather than an in-place column rewrite.

---

## 2. Contact Masking: Where in the Response Pipeline Does Masking Apply, and Who Bypasses It?

**Question:** The prompt shows masked phone numbers in the form `(415) ***-1234` but does not say whether masking is a view-layer concern (applied by the Vue.js frontend) or a server-side concern, nor who is allowed to see unmasked values. If the API returns the plaintext and trusts the UI, any CSRF-protected API consumer or a curious user with DevTools open will pull the raw string. What is the trust boundary?

**My Understanding:** Masking must happen server-side before serialization so the wire format is the only format a regular user's client ever receives. The owning user should see their own plaintext when editing the profile (otherwise round-tripping the value would destroy it), and Administrators should see plaintext for support workflows. Everyone else — including Service Agents reading a ticket — sees the masked form.

**Solution:** `internal/profile/phone.go` provides `MaskPhone()`, which extracts the decimal digits and returns `(NXX) ***-XXXX`, falling back to `***-****` when fewer than ten digits are available. The masking decision lives in the service (`profile.Service.GetProfile`), not the handler: an `isAdmin` boolean gate controls whether the decrypted plaintext or the masked string is emitted. Update flows accept and persist plaintext under the owning user's session, so editing a profile is not lossy. The controller never receives plaintext for a non-admin read — masking is applied before the DTO leaves the service, which keeps the JSON payload safe to log.

---

## 3. Session, CSRF, and Lockout: How the Three Cooperate Without Locking Legitimate Users Out

**Question:** The prompt stacks four session-protection requirements: HttpOnly cookies, per-request CSRF tokens, a 30-minute inactivity timeout, and a lockout that fires after 5 failed logins in 10 minutes for 15 minutes. Several details are underspecified — is the CSRF token rotated on every request or bound to the session, does "inactivity" mean "last authenticated request" or "last user interaction," and does the lockout counter consider the IP or the username? Each answer changes the attack surface.

**My Understanding:** The CSRF token should be bound to the session row (rotating it per-request breaks legitimate tabs that race), inactivity should be measured from the last authenticated request the server observed (the only clock the server controls), and the lockout must key on the username so a shared-IP attacker cannot deny service to an unrelated account, while still firing a single audit event when the threshold is crossed so log volume is bounded.

**Solution:** `internal/session/store.go` persists a `sessions` row with a UUID id, a 32-byte hex CSRF token generated once per session, and `last_active_at` / `expires_at` columns; the cookie is set HttpOnly with SameSite and a 24-hour absolute cap. The CSRF middleware (`internal/middleware/csrf.go`) validates `X-CSRF-Token` against the stored value on every state-changing request; `Touch()` updates `last_active_at` on each authenticated call and the inactivity limit is the package-level `InactivityTimeout = 30 * time.Minute`. Lockout logic in `internal/auth/service.go` (`checkLockout` / `recordAttempt`) keys on the user, counts failures inside a 10-minute sliding window, and when the 5th failure crosses the threshold it sets a 15-minute lock, fires the `LockoutNotifier` callback exactly once (wired in `router.go` to dispatch an `NotifAccountLockout` notification and write an audit entry), and then stops double-notifying on subsequent attempts during the same lock.

---

## 4. Rate Limiting: Per-User vs Per-IP, and How Review Quotas Resist Splitting

**Question:** "60 requests per minute per user" is unambiguous for authenticated traffic, but login, register, and other public endpoints have no user to key on. "10 review/report submissions per hour" raises a different question: if `create review`, `update review`, and `report abuse` each had their own 10/hour bucket, an abuser could submit 30 user-visible actions an hour while claiming compliance. Are the buckets shared or separate?

**My Understanding:** Unauthenticated endpoints must key on the client IP so login brute-force is still bounded. For reviews, the intent of the 10/hour cap is "user-generated review content," so create and update should share a bucket (they produce the same artifact), while `report abuse` is a different action against a different resource and deserves its own 10/hour bucket so one does not starve the other. The ceiling is applied synchronously on each request — a fixed window would let bursts straddle the boundary — so a sliding window of recent timestamps is the right primitive.

**Solution:** `internal/middleware/ratelimit.go` implements a per-key sliding window: timestamps live in `buckets[key][]time.Time`, are trimmed on each call to drop entries older than the window, and a full bucket returns 429 with a `Retry-After` header. The key is `"u:" + userID` for authenticated requests and `"ip:" + clientIP` otherwise, so public auth endpoints are still bounded. `router.go` constructs two limiter instances from `NewReviewReportLimiter()` (10/hour): one (`reviewRL`) wraps both `POST /tickets/:id/reviews` and `PUT /tickets/:id/reviews/:review_id` so a reviewer cannot split quota by alternating create and update, while a separate instance (`reportRL`) backs `POST /reviews/:id/reports`. `NewGeneralLimiter()` applies 60/min globally on the protected API group.

---

## 5. HMAC Signing for Internal Clients: Key Rotation, Canonical Message, and Rollover Semantics

**Question:** The prompt says internal clients (ingestion workers) sign requests with HMAC and that keys rotate server-side, but it does not prescribe the canonical message format, how the key id is conveyed, or what happens to in-flight requests when a key rotates. A naive rotation that immediately invalidates the old secret will drop requests already in transit; a rotation that keeps both secrets valid indefinitely defeats the point of rotating.

**My Understanding:** The canonical message must bind the method, path, and a hash of the body so an attacker cannot swap payloads under a captured signature. The `key_id` belongs in a request header so the server can look up the correct secret without trying every active key. Rotation should be a hard swap — the previous secret becomes invalid the moment the new one is written — and the plaintext secret must only ever leave the server once, at creation time. Callers that need a grace window should be issued a new `key_id` instead.

**Solution:** `internal/crypto/hmac_sign.go` canonicalizes each request as `METHOD + "\n" + PATH + "\n" + hex(sha256(body))` and the middleware expects `X-Key-ID` (for lookup) plus `X-Signature` in the form `hmac-sha256 <hex>`. `Verify()` uses `hmac.Equal` for constant-time comparison. Keys live in `hmac_keys` (`key_id` unique 1–64 chars of `[A-Za-z0-9._-]`, `secret_encrypted` as AES-GCM ciphertext, `is_active`, `rotated_at`); `internal/hmacadmin/service.go` generates a 32-byte random secret on create and returns plaintext exactly once via a `SecretReveal` struct — subsequent `List` calls never expose it. `Rotate` overwrites the secret in place, so the previous value becomes invalid immediately; callers who need staggered rollover create a second `key_id` and revoke the first once traffic has cut over. `EnsureDevKey` (dev-only, gated on `cfg.AppEnv`) auto-provisions a key so the CORS-exposed `X-Key-ID, X-Signature` headers are exercisable out of the box.

---

## 6. Content Moderation: What Counts as "Borderline," and How the Freeze Clock Escalates

**Question:** The prompt distinguishes "prohibited" terms (block outright) from "borderline" terms (route to a moderator), and it specifies escalating enforcement — 24-hour posting freeze, then 7-day — without saying what resets the counter, which author surfaces are subject to the freeze, or whether approval of a moderator-reviewed item clears the violation.

**My Understanding:** Violation escalation is a monotone count, not a rolling window — otherwise an abuser could wait out the first freeze and reset to a fresh 24 hours forever. Every surface that accepts free text (tickets notes, reviews, Q&A) must consult the dictionary, and the freeze must be checked before the write path starts so a frozen user's request is rejected cleanly with a lockout message rather than after a partial insert. A moderator-approved item should not create a violation — the violation record is specifically the rejection signal.

**Solution:** `internal/moderation/service.go` caches `sensitive_terms` in memory (`cacheTerm map[string]string` keyed by lowercased term, values `"prohibited"` or `"borderline"`), reloading on every add/delete and once at startup (`modSvc.ReloadTerms` in `router.go`). `Screen()` does whole-word tokenization and returns `ScreenResult{Class, FlaggedTerms}`. Prohibited hits reject at the middleware (`ScreenContent`); borderline hits call `OnBorderlineFlagged`, demote content to `pending_moderation`, and enqueue a `moderation_queue` row. `RejectItem` (not `ApproveItem`) writes a `violation_records` row and updates `users.posting_freeze_until`: the first violation adds `FirstFreezeHours = 24`, any subsequent one adds `SecondFreezeHours = 24 * 7`. `IsUserFrozen` backs the `FreezeCheck()` middleware wired in front of ticket-note, review, and Q&A write routes in `router.go`, so a frozen user is rejected before any screening or persistence work happens.

---

## 7. Lakehouse Checkpoints and Lifecycle: Resumability, Storage Layout, and Legal Hold Precedence

**Question:** Ingestion must track incremental offsets or `updated_at` checkpoints, resume cleanly mid-transfer, archive bronze data after 90 days, purge after 18 months, and honor legal holds. Several edge cases aren't specified: does "updated_at" and "offset" coexist on one source, where do bronze/silver/gold files physically live, and does a legal hold on a source block only purge (the destructive step) or also archive (the relocation step)? Archiving a held source would silently move the evidence to a backup path and break the implicit audit trail.

**My Understanding:** Sources must be able to pick either checkpoint type per job — some upstream tables expose a monotonic offset, others only `updated_at` — so the checkpoint row carries a discriminator. Storage should be date-partitioned so a lifecycle sweep can operate on whole directories rather than scanning every file. Legal hold must block both archive and purge: the goal is to freeze the record in place, untouched, until released.

**Solution:** `internal/ingest/service.go` upserts into `ingest_checkpoints` with `(source_id, job_id, checkpoint_type, checkpoint_value, updated_at)` — `checkpoint_type` is `CheckpointUpdatedAt` or `CheckpointOffset`, and `LatestCheckpointForSource` picks the most recent by `updated_at` so a restarted job resumes from the last committed point. `internal/lakehouse/service.go` writes each layer to `storage/lakehouse/<layer>/<source_id>/<YYYY-MM-DD>/<nanos>.dat` and records a row in `lakehouse_metadata` with a lineage entry in `lakehouse_lineage`. `RunLifecycle` (with `DefaultArchiveDays = 90`, `DefaultPurgeDays = 18*30`-ish expressed in days) moves bronze rows past `archiveDays` into `storage/backups/lakehouse/` and stamps `archived_at`, then deletes archived rows past `purgeDays` and stamps `purged_at`. `activeHoldFilter` reads `legal_holds` (scoped by `source_id` and/or `job_id`, active when `released_at IS NULL`) and excludes held records from both operations — a held source is counted in the `Held` result field and neither moved nor deleted. `StartLifecycleWorker` runs the sweep on a daily cadence from `router.go`; admins can trigger it on demand via `POST /api/v1/admin/lakehouse/lifecycle/run`.

---

## 8. Ticket Lifecycle: Which Transitions Are Reachable by Which Role

**Question:** The ticket states — Accepted, Dispatched, In Service, Completed, Closed — plus user cancellation before Dispatch, imply a state machine but the prompt does not enumerate who can drive which transition. If a Regular User can move a ticket to `Dispatched` the lifecycle is meaningless; if only an Administrator can close one, routine closures jam up.

**My Understanding:** Regular Users can create a ticket and cancel their own ticket while it is still in `Accepted`. Service Agents own the operational transitions (`Dispatched`, `In Service`, `Completed`). `Closed` is a terminal state reached after `Completed` and should be reachable by the owning user (acknowledging completion) or an Administrator. SLA timers run alongside the state, firing notifications on transition, and cancellation after Dispatch is blocked at the handler rather than silently allowed.

**Solution:** Ticket routes in `router.go` sit under the authenticated `/api/v1` group, with `PATCH /tickets/:id/status` driving transitions and the service enforcing the allowed-from/allowed-to matrix. Ticket create runs through `modSvc.FreezeCheck()` so a frozen user cannot bypass enforcement by filing a new ticket, and note creation layers `FreezeCheck` + `ScreenContent("content")` for the same reason. SLA evaluation is a background goroutine (`ticket_pkg.StartSLAEngine` in `router.go`) wired to the notification dispatcher so a transition or SLA breach emits a templated notification (`ticketSvc.SetNotifier` → `notifSvc.Dispatch`). Attachments are handled out-of-band via `/tickets/:id/attachments`, with per-file delete limited to the owning user and enforcement bounded by the 5 MB / 5-file caps applied in the ticket service.
