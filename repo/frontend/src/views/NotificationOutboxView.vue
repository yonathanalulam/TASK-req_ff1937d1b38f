<template>
  <div class="outbox-view">
    <button class="btn-back" @click="$router.back()">← Back</button>
    <h1>Notification Outbox</h1>
    <p class="subtitle">
      Messages we couldn't deliver in-app (because in-app notifications are disabled
      in your preferences) appear here for review.
    </p>

    <div v-if="loading" class="loading">Loading…</div>
    <div v-else-if="store.outbox.length === 0" class="empty" data-testid="outbox-empty">
      Outbox is empty.
    </div>
    <ul v-else class="outbox-list" data-testid="outbox-list">
      <li
        v-for="o in store.outbox"
        :key="o.id"
        class="outbox-item"
        data-testid="outbox-item"
      >
        <div class="status-row">
          <span class="status-badge" :class="o.status">{{ o.status }}</span>
          <span class="meta">{{ formatTime(o.created_at) }}</span>
        </div>
        <div class="title">{{ o.notification?.title }}</div>
        <div class="body">{{ o.notification?.body }}</div>
        <div v-if="o.attempts > 0" class="attempts">Attempts: {{ o.attempts }}</div>
      </li>
    </ul>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useNotificationStore } from '@/stores/notification'

const store = useNotificationStore()
const loading = ref(true)

onMounted(async () => {
  await store.fetchOutbox()
  loading.value = false
})

function formatTime(iso) {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}
</script>

<style scoped>
.outbox-view { max-width: 720px; }

.btn-back {
  background: none; border: none; color: #4f46e5;
  font-size: .9rem; cursor: pointer; padding: 0; margin-bottom: .75rem;
}
.btn-back:hover { text-decoration: underline; }

h1 { margin: 0 0 .35rem; }
.subtitle { color: #6b7280; font-size: .9rem; margin: 0 0 1.25rem; }

.loading, .empty { color: #6b7280; text-align: center; padding: 2rem 0; }

.outbox-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: .55rem; }

.outbox-item {
  background: #fff; border: 1px solid #e5e7eb; border-radius: 10px; padding: .8rem 1rem;
}

.status-row { display: flex; align-items: center; justify-content: space-between; margin-bottom: .25rem; }

.status-badge {
  font-size: .72rem; padding: .1rem .5rem; border-radius: 4px; font-weight: 600;
  background: #e5e7eb; color: #374151;
}
.status-badge.pending { background: #fef3c7; color: #92400e; }
.status-badge.sent    { background: #d1fae5; color: #065f46; }
.status-badge.failed  { background: #fee2e2; color: #991b1b; }

.meta { font-size: .75rem; color: #9ca3af; }
.title { font-weight: 600; color: #111827; }
.body { color: #374151; font-size: .9rem; margin-top: .2rem; }
.attempts { font-size: .75rem; color: #6b7280; margin-top: .3rem; }
</style>
