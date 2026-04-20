<template>
  <div class="privacy-view">
    <button class="btn-back" @click="$router.back()">← Back</button>
    <h1>Privacy Center</h1>
    <p class="subtitle">Manage exports of your personal data and account deletion.</p>

    <!-- Export -->
    <section class="card" data-testid="export-section">
      <h2>Download my data</h2>
      <p class="muted">
        Get a ZIP archive containing your profile, addresses, tickets, reviews,
        Q&amp;A posts, and notifications. Audit logs are excluded.
      </p>

      <div class="status-row">
        <span class="label">Status</span>
        <span class="badge" :class="exportBadgeClass" data-testid="export-status">
          {{ exportStatusText }}
        </span>
      </div>

      <div class="actions">
        <button
          v-if="canRequestExport"
          class="btn-primary"
          data-testid="btn-request-export"
          :disabled="busy"
          @click="onRequestExport"
        >
          Request Export
        </button>
        <a
          v-if="canDownload"
          class="btn-primary"
          data-testid="btn-download-export"
          :href="downloadHref"
        >
          Download ZIP
        </a>
      </div>

      <div v-if="exportError" class="error" data-testid="export-error">{{ exportError }}</div>
    </section>

    <!-- Deletion -->
    <section class="card danger" data-testid="deletion-section">
      <h2>Delete my account</h2>
      <p class="muted">
        Account deletion is irreversible after the 30-day grace period.
        Your account will be deactivated immediately and personal data will be
        anonymized after 30 days. Audit log entries are retained.
      </p>

      <div v-if="deletionPending" class="pending-banner" data-testid="deletion-pending">
        Deletion scheduled for {{ formatTime(deletion.scheduled_for) }}.
      </div>

      <button
        v-else
        class="btn-danger"
        data-testid="btn-open-delete"
        @click="showDeleteModal = true"
      >
        Delete my account
      </button>
    </section>

    <!-- Confirmation modal -->
    <div v-if="showDeleteModal" class="modal-backdrop" @click.self="showDeleteModal = false">
      <div class="modal" data-testid="modal-delete">
        <h3>Confirm Deletion</h3>
        <p class="muted">
          Type <code>DELETE</code> below to confirm. This cannot be undone.
        </p>
        <input
          v-model="confirmText"
          type="text"
          placeholder="DELETE"
          data-testid="input-delete-confirm"
        />
        <div v-if="deletionError" class="error">{{ deletionError }}</div>
        <div class="modal-actions">
          <button class="btn-secondary" @click="showDeleteModal = false">Cancel</button>
          <button
            class="btn-danger"
            data-testid="btn-confirm-delete"
            :disabled="confirmText !== 'DELETE' || busy"
            @click="onConfirmDelete"
          >
            Delete my account
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { usePrivacyStore } from '@/stores/privacy'

const privacy = usePrivacyStore()
const busy = ref(false)
const exportError = ref('')
const deletionError = ref('')
const showDeleteModal = ref(false)
const confirmText = ref('')

const exportReq = computed(() => privacy.exportRequest)
const deletion = computed(() => privacy.deletionRequest)

const exportStatusText = computed(() => {
  const r = exportReq.value
  if (!r) return 'No active export'
  return ({
    pending: 'Preparing…',
    processing: 'Generating archive…',
    ready: 'Ready to download',
    downloaded: 'Downloaded',
    expired: 'Expired',
  })[r.status] ?? r.status
})

const exportBadgeClass = computed(() => {
  const s = exportReq.value?.status ?? ''
  return {
    pending: s === 'pending' || s === 'processing',
    ready: s === 'ready',
    done: s === 'downloaded',
    expired: s === 'expired',
  }
})

const canRequestExport = computed(() => {
  const r = exportReq.value
  return !r || r.status === 'downloaded' || r.status === 'expired'
})

const canDownload = computed(() => exportReq.value?.status === 'ready')
const downloadHref = computed(() => privacy.downloadUrl())

const deletionPending = computed(() => deletion.value?.status === 'pending')

onMounted(async () => {
  await Promise.all([
    privacy.fetchExportStatus(),
    privacy.fetchDeletionStatus(),
  ])
})

async function onRequestExport() {
  busy.value = true
  exportError.value = ''
  try {
    await privacy.requestExport()
  } catch (err) {
    if (err.response?.status === 409) {
      exportError.value = 'An export is already in progress.'
    } else {
      exportError.value = err.response?.data?.error?.message ?? 'Could not start export.'
    }
  } finally {
    busy.value = false
  }
}

async function onConfirmDelete() {
  busy.value = true
  deletionError.value = ''
  try {
    await privacy.requestDeletion()
    showDeleteModal.value = false
  } catch (err) {
    if (err.response?.status === 409) {
      deletionError.value = 'A deletion request is already pending.'
    } else {
      deletionError.value = err.response?.data?.error?.message ?? 'Could not submit request.'
    }
  } finally {
    busy.value = false
  }
}

function formatTime(iso) {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}
</script>

<style scoped>
.privacy-view { max-width: 720px; }

.btn-back {
  background: none; border: none; color: #4f46e5;
  font-size: .9rem; cursor: pointer; padding: 0; margin-bottom: .75rem;
}
.btn-back:hover { text-decoration: underline; }

h1 { margin: 0 0 .35rem; }
.subtitle { color: #6b7280; font-size: .9rem; margin: 0 0 1.25rem; }

.card {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 1.1rem 1.25rem;
  margin-bottom: 1rem;
}
.card.danger { border-color: #fca5a5; background: #fff7f7; }

h2 { margin: 0 0 .4rem; font-size: 1.05rem; color: #111827; }
.muted { color: #6b7280; font-size: .9rem; line-height: 1.5; margin: 0 0 .8rem; }

.status-row { display: flex; gap: .55rem; align-items: center; margin-bottom: .75rem; }
.label { color: #6b7280; font-size: .85rem; }
.badge {
  background: #e5e7eb; color: #374151;
  padding: .15rem .55rem; border-radius: 4px;
  font-size: .8rem; font-weight: 600;
}
.badge.pending { background: #fef3c7; color: #92400e; }
.badge.ready { background: #d1fae5; color: #065f46; }
.badge.done { background: #e5e7eb; color: #6b7280; }
.badge.expired { background: #fee2e2; color: #991b1b; }

.actions { display: flex; gap: .5rem; align-items: center; }

.pending-banner {
  background: #fef3c7;
  border: 1px solid #fde68a;
  color: #78350f;
  padding: .55rem .8rem;
  border-radius: 8px;
  font-size: .9rem;
}

.btn-primary {
  background: #4f46e5; color: #fff; border: none;
  border-radius: 6px; padding: .45rem 1rem;
  font-size: .9rem; cursor: pointer; text-decoration: none;
  display: inline-block;
}
.btn-primary:hover:not(:disabled) { background: #4338ca; }
.btn-primary:disabled { opacity: .5; cursor: not-allowed; }

.btn-secondary {
  background: #f3f4f6; color: #374151; border: 1px solid #d1d5db;
  border-radius: 6px; padding: .45rem 1rem;
  font-size: .9rem; cursor: pointer;
}

.btn-danger {
  background: #dc2626; color: #fff; border: none;
  border-radius: 6px; padding: .45rem 1rem;
  font-size: .9rem; cursor: pointer;
}
.btn-danger:hover:not(:disabled) { background: #b91c1c; }
.btn-danger:disabled { opacity: .5; cursor: not-allowed; }

.error { color: #b91c1c; font-size: .85rem; margin-top: .5rem; }

.modal-backdrop {
  position: fixed; inset: 0;
  background: rgba(17, 24, 39, .5);
  display: flex; align-items: center; justify-content: center;
  z-index: 600;
}
.modal {
  background: #fff;
  border-radius: 12px;
  padding: 1.25rem 1.5rem;
  width: 420px;
  max-width: 90vw;
}
h3 { margin: 0 0 .5rem; }
.modal input {
  width: 100%;
  padding: .5rem .65rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  box-sizing: border-box;
  margin: .5rem 0;
  font-family: inherit;
}
.modal-actions { display: flex; justify-content: flex-end; gap: .5rem; margin-top: .6rem; }
code { background: #e5e7eb; padding: 0 .3em; border-radius: 3px; }
</style>
