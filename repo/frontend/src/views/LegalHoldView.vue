<template>
  <div class="legal-hold-view">
    <button class="btn-back" @click="$router.back()">← Back</button>
    <h1>Legal Holds</h1>
    <p class="subtitle">
      Active legal holds prevent the lakehouse lifecycle job from purging
      ingested data for a source or job. Releasing a hold re-enables purging.
    </p>

    <!-- Place hold form -->
    <section class="card">
      <h2>Place a new hold</h2>
      <div class="form-row">
        <input
          v-model.number="newSourceId"
          type="number"
          min="1"
          placeholder="Source ID"
          data-testid="input-hold-source-id"
        />
        <input
          v-model="newReason"
          type="text"
          placeholder="Reason (e.g. litigation hold #A-123)"
          data-testid="input-hold-reason"
        />
        <button
          class="btn-primary"
          data-testid="btn-place-hold"
          :disabled="!newSourceId || !newReason || busy"
          @click="onPlace"
        >
          Place Hold
        </button>
      </div>
      <div v-if="placeError" class="error">{{ placeError }}</div>
    </section>

    <!-- Active holds -->
    <section class="card">
      <h2>Active holds</h2>
      <div v-if="loading" class="loading">Loading…</div>
      <div v-else-if="store.holds.length === 0" class="empty" data-testid="holds-empty">
        No active holds.
      </div>
      <ul v-else class="hold-list" data-testid="hold-list">
        <li v-for="h in store.holds" :key="h.id" class="hold-item" data-testid="hold-item">
          <div class="hold-row">
            <span class="hold-target">
              <template v-if="h.source_id">Source #{{ h.source_id }}</template>
              <template v-else-if="h.job_id">Job #{{ h.job_id }}</template>
            </span>
            <span class="meta">{{ formatTime(h.placed_at) }}</span>
          </div>
          <div class="hold-reason">{{ h.reason }}</div>
          <div class="hold-actions">
            <button
              class="btn-mini-danger"
              :data-testid="`btn-release-${h.id}`"
              @click="onRelease(h.id)"
            >
              Release
            </button>
          </div>
        </li>
      </ul>
    </section>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useDataOpsStore } from '@/stores/dataops'

const store = useDataOpsStore()
const loading = ref(true)
const newSourceId = ref(null)
const newReason = ref('')
const placeError = ref('')
const busy = ref(false)

onMounted(async () => {
  try {
    await store.fetchHolds()
  } catch (err) {
    console.error('failed to fetch holds', err)
  }
  loading.value = false
})

async function onPlace() {
  busy.value = true
  placeError.value = ''
  try {
    await store.placeHold({ source_id: Number(newSourceId.value), reason: newReason.value })
    newSourceId.value = null
    newReason.value = ''
  } catch (err) {
    placeError.value = err.response?.data?.error?.message ?? 'Could not place hold.'
  } finally {
    busy.value = false
  }
}

async function onRelease(id) {
  if (!window.confirm('Release this legal hold?')) return
  try {
    await store.releaseHold(id)
  } catch (err) {
    console.error('release failed', err)
  }
}

function formatTime(iso) {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}
</script>

<style scoped>
.legal-hold-view { max-width: 720px; }

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
  padding: 1.05rem 1.2rem;
  margin-bottom: 1rem;
}

h2 { margin: 0 0 .55rem; font-size: 1rem; color: #111827; }

.form-row {
  display: grid;
  grid-template-columns: 110px 1fr auto;
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

.hold-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: .5rem; }
.hold-item {
  background: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: .55rem .85rem;
}
.hold-row {
  display: flex; justify-content: space-between; align-items: center;
}
.hold-target { font-weight: 600; color: #111827; }
.meta { font-size: .78rem; color: #6b7280; }
.hold-reason { color: #374151; font-size: .88rem; margin: .15rem 0 .35rem; }
.hold-actions { text-align: right; }

.btn-primary {
  background: #4f46e5; color: #fff; border: none;
  border-radius: 6px; padding: .4rem .9rem;
  font-size: .85rem; cursor: pointer;
}
.btn-primary:disabled { opacity: .5; cursor: not-allowed; }

.btn-mini-danger {
  background: #fee2e2; color: #991b1b; border: none;
  border-radius: 4px; padding: .15rem .55rem;
  font-size: .75rem; cursor: pointer;
}
</style>
