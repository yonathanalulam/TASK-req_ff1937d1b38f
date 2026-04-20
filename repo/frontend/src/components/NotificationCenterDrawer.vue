<template>
  <div class="drawer-backdrop" @click.self="$emit('close')">
    <aside class="drawer" data-testid="notification-drawer">
      <div class="drawer-header">
        <h2>Notifications</h2>
        <button class="btn-close" @click="$emit('close')">×</button>
      </div>

      <div class="drawer-actions">
        <button
          class="btn-link"
          data-testid="btn-mark-all-read"
          :disabled="store.items.length === 0 || store.unreadCount === 0"
          @click="onMarkAll"
        >
          Mark all read
        </button>
        <router-link
          to="/notifications/outbox"
          class="link"
          data-testid="link-outbox"
          @click="$emit('close')"
        >
          Outbox
        </router-link>
      </div>

      <div v-if="loading" class="loading">Loading…</div>
      <div v-else-if="store.items.length === 0" class="empty" data-testid="notification-empty">
        Nothing here yet.
      </div>
      <ul v-else class="notification-list">
        <li
          v-for="n in store.items"
          :key="n.id"
          class="notification-item"
          :class="{ unread: !n.is_read }"
          data-testid="notification-item"
        >
          <div class="item-body">
            <div class="item-title" data-testid="notification-title">{{ n.title }}</div>
            <div class="item-text">{{ n.body }}</div>
            <div class="item-meta">{{ formatTime(n.created_at) }}</div>
          </div>
          <button
            v-if="!n.is_read"
            class="btn-mark"
            data-testid="btn-mark-read"
            @click="onMarkRead(n.id)"
          >
            Mark read
          </button>
        </li>
      </ul>

      <div v-if="store.nextCursor > 0" class="load-more">
        <button class="btn-secondary" data-testid="btn-load-more-notifs" @click="loadMore">
          Load More
        </button>
      </div>
    </aside>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useNotificationStore } from '@/stores/notification'

defineEmits(['close'])

const store = useNotificationStore()
const loading = ref(true)

onMounted(async () => {
  await store.fetchPage(0)
  loading.value = false
})

async function loadMore() {
  await store.fetchPage(store.nextCursor)
}

async function onMarkRead(id) {
  await store.markRead(id)
}

async function onMarkAll() {
  await store.markAllRead()
}

function formatTime(iso) {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}
</script>

<style scoped>
.drawer-backdrop {
  position: fixed; inset: 0;
  background: rgba(17, 24, 39, .35);
  display: flex;
  justify-content: flex-end;
  z-index: 600;
}

.drawer {
  background: #fff;
  width: 380px;
  max-width: 95vw;
  height: 100%;
  overflow-y: auto;
  padding: 1rem 1.1rem 1.5rem;
  box-shadow: -4px 0 20px rgba(0,0,0,.15);
  display: flex;
  flex-direction: column;
}

.drawer-header { display: flex; align-items: center; justify-content: space-between; }
h2 { margin: 0; font-size: 1.05rem; color: #111827; }

.btn-close {
  background: none; border: none;
  font-size: 1.4rem; cursor: pointer; color: #6b7280;
}
.btn-close:hover { color: #111827; }

.drawer-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: .65rem 0 .85rem;
  font-size: .85rem;
}

.btn-link {
  background: none; border: none; cursor: pointer;
  color: #4f46e5; padding: 0;
}
.btn-link:disabled { color: #c7d2fe; cursor: not-allowed; }
.btn-link:hover:not(:disabled) { text-decoration: underline; }

.link { color: #4f46e5; text-decoration: none; }
.link:hover { text-decoration: underline; }

.loading, .empty { color: #6b7280; padding: 2rem 0; text-align: center; }

.notification-list {
  list-style: none; padding: 0; margin: 0;
  display: flex; flex-direction: column; gap: .5rem;
}

.notification-item {
  display: flex;
  gap: .5rem;
  padding: .65rem .8rem;
  border-radius: 8px;
  background: #f9fafb;
}
.notification-item.unread { background: #eef2ff; }

.item-body { flex: 1; min-width: 0; }
.item-title { font-weight: 600; color: #111827; font-size: .9rem; }
.item-text { color: #374151; font-size: .85rem; margin-top: .15rem; word-wrap: break-word; }
.item-meta { color: #9ca3af; font-size: .75rem; margin-top: .3rem; }

.btn-mark {
  align-self: flex-start;
  background: #fff;
  border: 1px solid #c7d2fe;
  border-radius: 4px;
  padding: .15rem .5rem;
  font-size: .72rem;
  color: #4f46e5;
  cursor: pointer;
}
.btn-mark:hover { background: #eef2ff; }

.load-more { margin-top: .8rem; text-align: center; }
.btn-secondary {
  background: #f3f4f6; border: 1px solid #d1d5db; color: #374151;
  border-radius: 6px; padding: .35rem .8rem; font-size: .82rem; cursor: pointer;
}
</style>
