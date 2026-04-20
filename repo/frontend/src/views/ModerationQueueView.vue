<template>
  <div class="moderation-queue-view">
    <h1>Moderation Queue</h1>

    <div v-if="loading" class="loading">Loading…</div>
    <div v-else-if="store.queue.length === 0" class="empty" data-testid="queue-empty">
      Queue is empty. Nothing to review.
    </div>
    <ul v-else class="queue-list" data-testid="queue-list">
      <li
        v-for="item in store.queue"
        :key="item.id"
        class="queue-item"
        data-testid="queue-item"
      >
        <div class="item-head">
          <span class="content-type" data-testid="queue-content-type">{{ item.content_type }}</span>
          <span class="meta">#{{ item.content_id }} · {{ formatTime(item.created_at) }}</span>
        </div>
        <p class="content" v-html="highlight(item.content_text, item.flagged_terms || [])"></p>
        <div v-if="item.flagged_terms?.length" class="flagged-row">
          <span class="flagged-label">Flagged:</span>
          <span v-for="term in item.flagged_terms" :key="term" class="flagged-term">{{ term }}</span>
        </div>
        <div class="actions">
          <input
            v-model="reasons[item.id]"
            placeholder="Reason (optional)"
            class="reason-input"
            :data-testid="`input-reason-${item.id}`"
          />
          <button
            class="btn-approve"
            :data-testid="`btn-approve-${item.id}`"
            @click="onApprove(item.id)"
          >
            Approve
          </button>
          <button
            class="btn-reject"
            :data-testid="`btn-reject-${item.id}`"
            @click="onReject(item.id)"
          >
            Reject
          </button>
        </div>
      </li>
    </ul>

    <div v-if="lastFreeze" class="freeze-banner" data-testid="freeze-banner">
      Freeze applied until {{ formatTime(lastFreeze) }}
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useModerationStore } from '@/stores/moderation'

const store = useModerationStore()
const loading = ref(true)
const reasons = ref({})
const lastFreeze = ref(null)

onMounted(async () => {
  await store.fetchQueue()
  loading.value = false
})

function highlight(text, terms) {
  if (!terms || terms.length === 0) return escape(text)
  let safe = escape(text)
  for (const term of terms) {
    const re = new RegExp(`\\b${escapeReg(term)}\\b`, 'gi')
    safe = safe.replace(re, (m) => `<mark>${m}</mark>`)
  }
  return safe
}

function escape(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function escapeReg(s) { return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') }

async function onApprove(id) {
  try {
    await store.approve(id, reasons.value[id] ?? '')
  } catch (err) {
    console.error('approve failed', err)
  }
}

async function onReject(id) {
  try {
    const result = await store.reject(id, reasons.value[id] ?? '')
    if (result?.freeze_until) lastFreeze.value = result.freeze_until
  } catch (err) {
    console.error('reject failed', err)
  }
}

function formatTime(iso) {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}
</script>

<style scoped>
.moderation-queue-view { max-width: 760px; }

h1 { margin: 0 0 1rem; }

.loading, .empty { color: #6b7280; text-align: center; padding: 2.5rem 0; }

.queue-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: .65rem; }

.queue-item {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: .85rem 1rem;
}

.item-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: .35rem;
}

.content-type {
  text-transform: uppercase;
  font-size: .7rem;
  font-weight: 700;
  letter-spacing: .03em;
  background: #eef2ff;
  color: #4338ca;
  padding: .15rem .5rem;
  border-radius: 4px;
}

.meta { font-size: .8rem; color: #6b7280; }

.content {
  white-space: pre-wrap;
  color: #111827;
  margin: .35rem 0;
  font-size: .9rem;
  line-height: 1.5;
}

:deep(mark) { background: #fde68a; color: #78350f; padding: 0 .15em; border-radius: 2px; }

.flagged-row { display: flex; align-items: center; gap: .35rem; flex-wrap: wrap; margin-bottom: .55rem; font-size: .8rem; }
.flagged-label { color: #6b7280; }
.flagged-term {
  background: #fee2e2;
  color: #991b1b;
  padding: .1rem .45rem;
  border-radius: 4px;
  font-weight: 600;
}

.actions { display: flex; gap: .5rem; align-items: center; }

.reason-input {
  flex: 1;
  padding: .35rem .55rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .85rem;
}

.btn-approve, .btn-reject {
  border: none;
  border-radius: 6px;
  padding: .35rem .85rem;
  font-size: .85rem;
  cursor: pointer;
}
.btn-approve { background: #10b981; color: #fff; }
.btn-approve:hover { background: #059669; }
.btn-reject  { background: #dc2626; color: #fff; }
.btn-reject:hover  { background: #b91c1c; }

.freeze-banner {
  margin-top: 1rem;
  background: #fef2f2;
  border: 1px solid #fca5a5;
  border-radius: 8px;
  padding: .55rem .8rem;
  color: #991b1b;
  font-size: .85rem;
}
</style>
