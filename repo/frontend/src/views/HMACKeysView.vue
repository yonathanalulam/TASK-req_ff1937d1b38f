<template>
  <div class="hmac-keys-view">
    <button class="btn-back" @click="$router.back()">← Back</button>
    <h1>HMAC Signing Keys</h1>
    <p class="subtitle">
      Internal clients (ingestion workers, lakehouse jobs) sign their requests
      to <code>/api/v1/internal/*</code> with an HMAC-SHA256 key managed here.
      Secrets are revealed once at creation or rotation — copy them to the
      client immediately.
    </p>

    <!-- One-shot secret reveal banner -->
    <section
      v-if="store.lastReveal"
      class="reveal-card"
      data-testid="reveal-card"
    >
      <header>
        <strong>
          {{ store.lastReveal.action === 'rotate' ? 'Rotated' : 'New' }} secret for
          <code>{{ store.lastReveal.key_id }}</code>
        </strong>
        <button class="btn-link" @click="dismissReveal" data-testid="btn-dismiss-reveal">
          Dismiss
        </button>
      </header>
      <p class="reveal-warning">
        This is the <strong>only</strong> time the plaintext secret will be shown.
        Copy it now — after dismissing you cannot retrieve it again.
      </p>
      <div class="secret-row">
        <code class="secret" data-testid="reveal-secret">{{ store.lastReveal.secret }}</code>
        <button class="btn-primary" data-testid="btn-copy-secret" @click="copySecret">
          {{ copied ? 'Copied!' : 'Copy' }}
        </button>
      </div>
    </section>

    <!-- Create key -->
    <section class="card">
      <h2>Create a new key</h2>
      <div class="form-row">
        <input
          v-model="newKeyId"
          type="text"
          placeholder="key_id (letters, digits, - _ . only)"
          data-testid="input-new-key-id"
          :disabled="busy"
        />
        <button
          class="btn-primary"
          data-testid="btn-create-key"
          :disabled="!newKeyId || busy"
          @click="onCreate"
        >
          Create
        </button>
      </div>
      <div v-if="createError" class="error" data-testid="create-error">{{ createError }}</div>
    </section>

    <!-- Existing keys -->
    <section class="card">
      <h2>Existing keys</h2>
      <div v-if="loading" class="loading">Loading…</div>
      <div v-else-if="store.keys.length === 0" class="empty" data-testid="keys-empty">
        No HMAC keys. Create one above to enable internal clients.
      </div>
      <table v-else class="keys-table" data-testid="keys-table">
        <thead>
          <tr>
            <th>Key ID</th>
            <th>Status</th>
            <th>Created</th>
            <th>Last rotated</th>
            <th class="actions-col">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="k in store.keys"
            :key="k.id"
            :data-testid="`key-row-${k.key_id}`"
          >
            <td><code>{{ k.key_id }}</code></td>
            <td>
              <span :class="['badge', k.is_active ? 'badge-ok' : 'badge-revoked']">
                {{ k.is_active ? 'active' : 'revoked' }}
              </span>
            </td>
            <td>{{ formatTime(k.created_at) }}</td>
            <td>{{ k.rotated_at ? formatTime(k.rotated_at) : '—' }}</td>
            <td class="actions">
              <button
                class="btn-mini"
                :data-testid="`btn-rotate-${k.key_id}`"
                :disabled="busy"
                @click="onRotate(k.key_id)"
              >
                Rotate
              </button>
              <button
                v-if="k.is_active"
                class="btn-mini-danger"
                :data-testid="`btn-revoke-${k.id}`"
                :disabled="busy"
                @click="onRevoke(k.id, k.key_id)"
              >
                Revoke
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useHmacKeysStore } from '@/stores/hmacKeys'

const store = useHmacKeysStore()
const loading = ref(true)
const busy = ref(false)
const newKeyId = ref('')
const createError = ref('')
const copied = ref(false)

onMounted(async () => {
  try {
    await store.fetchKeys()
  } catch (err) {
    console.error('failed to fetch hmac keys', err)
  }
  loading.value = false
})

async function onCreate() {
  busy.value = true
  createError.value = ''
  copied.value = false
  try {
    await store.createKey(newKeyId.value.trim())
    newKeyId.value = ''
  } catch (err) {
    createError.value =
      err.response?.data?.error?.message ?? 'Could not create key.'
  } finally {
    busy.value = false
  }
}

async function onRotate(keyId) {
  // Rotation is a hard cut-over — existing clients break until they pick up
  // the new secret. Confirm before proceeding so this doesn't happen by a
  // stray click.
  if (
    !window.confirm(
      `Rotate HMAC secret for "${keyId}"?\n\n` +
        'This immediately invalidates the current secret. ' +
        'Any client still using it will receive 401 until updated.'
    )
  ) {
    return
  }
  busy.value = true
  copied.value = false
  try {
    await store.rotateKey(keyId)
  } catch (err) {
    console.error('rotate failed', err)
  } finally {
    busy.value = false
  }
}

async function onRevoke(id, keyId) {
  if (
    !window.confirm(
      `Revoke HMAC key "${keyId}"?\n\n` +
        'The key row is retained for audit but the verifier will reject it ' +
        'immediately. Rotate to bring it back.'
    )
  ) {
    return
  }
  busy.value = true
  try {
    await store.revokeKey(id)
  } catch (err) {
    console.error('revoke failed', err)
  } finally {
    busy.value = false
  }
}

async function copySecret() {
  const secret = store.lastReveal?.secret
  if (!secret) return
  try {
    await navigator.clipboard.writeText(secret)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch (err) {
    // Clipboard API can fail in insecure contexts; the secret is still
    // selectable manually from the displayed code block.
    console.error('clipboard write failed', err)
  }
}

function dismissReveal() {
  store.clearReveal()
  copied.value = false
}

function formatTime(iso) {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}
</script>

<style scoped>
.hmac-keys-view { max-width: 900px; }

.btn-back {
  background: none; border: none; color: #4f46e5;
  font-size: .9rem; cursor: pointer; padding: 0; margin-bottom: .75rem;
}
.btn-back:hover { text-decoration: underline; }

h1 { margin: 0 0 .35rem; }
.subtitle { color: #6b7280; font-size: .9rem; margin: 0 0 1.25rem; }
.subtitle code { background: #f3f4f6; padding: 0 .25rem; border-radius: 3px; }

.card {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 1.05rem 1.2rem;
  margin-bottom: 1rem;
}

h2 { margin: 0 0 .55rem; font-size: 1rem; color: #111827; }

.reveal-card {
  background: #fef3c7;
  border: 1px solid #f59e0b;
  border-radius: 12px;
  padding: 1rem 1.2rem;
  margin-bottom: 1rem;
}
.reveal-card header {
  display: flex; justify-content: space-between; align-items: center;
  margin-bottom: .4rem;
}
.reveal-warning { font-size: .85rem; color: #92400e; margin: 0 0 .6rem; }
.secret-row {
  display: flex; gap: .5rem; align-items: center;
}
.secret {
  flex: 1; font-family: ui-monospace, Menlo, monospace;
  font-size: .8rem; padding: .4rem .55rem;
  background: #fff; border: 1px solid #f59e0b; border-radius: 6px;
  word-break: break-all; user-select: all;
}
.btn-link {
  background: none; border: none; color: #6b7280;
  font-size: .8rem; cursor: pointer;
}
.btn-link:hover { text-decoration: underline; color: #374151; }

.form-row {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: .5rem;
}
.form-row input {
  padding: .4rem .6rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  font-family: inherit;
}

.error { color: #b91c1c; font-size: .85rem; margin-top: .4rem; }

.loading, .empty { color: #6b7280; padding: 1rem 0; text-align: center; }

.keys-table { width: 100%; border-collapse: collapse; font-size: .9rem; }
.keys-table th, .keys-table td {
  text-align: left; padding: .5rem .6rem;
  border-bottom: 1px solid #f3f4f6;
}
.keys-table th { color: #6b7280; font-weight: 600; font-size: .8rem; }
.keys-table code {
  background: #f3f4f6; padding: .1rem .35rem;
  border-radius: 4px; font-size: .82rem;
}
.actions-col { width: 160px; }
.actions { display: flex; gap: .4rem; }

.badge {
  display: inline-block; padding: .1rem .5rem;
  border-radius: 999px; font-size: .72rem; font-weight: 600;
  text-transform: uppercase; letter-spacing: .02em;
}
.badge-ok { background: #dcfce7; color: #166534; }
.badge-revoked { background: #fee2e2; color: #991b1b; }

.btn-primary {
  background: #4f46e5; color: #fff; border: none;
  border-radius: 6px; padding: .4rem .9rem;
  font-size: .85rem; cursor: pointer;
}
.btn-primary:disabled { opacity: .5; cursor: not-allowed; }

.btn-mini {
  background: #eef2ff; color: #4338ca; border: none;
  border-radius: 4px; padding: .2rem .6rem;
  font-size: .75rem; cursor: pointer;
}
.btn-mini:disabled { opacity: .5; cursor: not-allowed; }

.btn-mini-danger {
  background: #fee2e2; color: #991b1b; border: none;
  border-radius: 4px; padding: .2rem .6rem;
  font-size: .75rem; cursor: pointer;
}
.btn-mini-danger:disabled { opacity: .5; cursor: not-allowed; }
</style>
