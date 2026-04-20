<template>
  <div class="violation-history-view">
    <button class="btn-back" @click="$router.back()">← Back</button>
    <h1>Violation History</h1>

    <div class="lookup">
      <label for="vh-user">User ID</label>
      <input
        id="vh-user"
        v-model.number="userIdInput"
        type="number"
        min="1"
        placeholder="123"
        data-testid="input-user-id"
      />
      <button
        class="btn-primary"
        data-testid="btn-load-violations"
        :disabled="!userIdInput"
        @click="load"
      >
        Load
      </button>
    </div>

    <div v-if="loading" class="loading">Loading…</div>
    <div v-else-if="data && data.violations.length === 0" class="empty">
      No violations on record for this user.
    </div>

    <div v-else-if="data" class="result">
      <div v-if="data.freeze_until" class="freeze-banner" data-testid="freeze-banner">
        Currently frozen until {{ formatTime(data.freeze_until) }}
      </div>

      <ul class="vh-list" data-testid="violation-list">
        <li v-for="v in data.violations" :key="v.id" class="vh-item" data-testid="violation-item">
          <div class="vh-head">
            <span class="content-type">{{ v.content_type }} #{{ v.content_id }}</span>
            <span class="meta">{{ formatTime(v.violation_at) }}</span>
          </div>
          <div v-if="v.freeze_applied" class="freeze-line">
            Freeze applied: {{ v.freeze_duration_hours }}h
          </div>
        </li>
      </ul>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useModerationStore } from '@/stores/moderation'

const store = useModerationStore()
const userIdInput = ref(null)
const loading = ref(false)

const data = computed(() => store.violationsByUser[userIdInput.value] ?? null)

async function load() {
  if (!userIdInput.value) return
  loading.value = true
  try {
    await store.fetchUserViolations(userIdInput.value)
  } finally {
    loading.value = false
  }
}

function formatTime(iso) {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}
</script>

<style scoped>
.violation-history-view { max-width: 720px; }

.btn-back {
  background: none; border: none; color: #4f46e5;
  font-size: .9rem; cursor: pointer; padding: 0; margin-bottom: .75rem;
}
.btn-back:hover { text-decoration: underline; }

h1 { margin: 0 0 1rem; }

.lookup {
  display: flex; gap: .5rem; align-items: center;
  margin-bottom: 1.25rem;
}
.lookup label { font-weight: 600; font-size: .85rem; color: #374151; }
.lookup input {
  width: 110px;
  padding: .35rem .55rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
}

.btn-primary {
  background: #4f46e5; color: #fff; border: none;
  border-radius: 6px; padding: .4rem 1rem; font-size: .85rem; cursor: pointer;
}
.btn-primary:disabled { opacity: .5; cursor: not-allowed; }

.loading, .empty { color: #6b7280; text-align: center; padding: 1.5rem 0; }

.freeze-banner {
  background: #fef2f2;
  border: 1px solid #fca5a5;
  border-radius: 8px;
  padding: .55rem .8rem;
  color: #991b1b;
  font-size: .9rem;
  margin-bottom: 1rem;
}

.vh-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: .5rem; }
.vh-item { background: #fff; border: 1px solid #e5e7eb; border-radius: 8px; padding: .6rem .85rem; }
.vh-head { display: flex; justify-content: space-between; align-items: center; }
.content-type { font-weight: 600; color: #111827; font-size: .9rem; }
.meta { font-size: .8rem; color: #6b7280; }
.freeze-line { color: #b91c1c; font-size: .82rem; margin-top: .25rem; }
</style>
