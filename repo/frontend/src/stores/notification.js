import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const useNotificationStore = defineStore('notification', () => {
  const items = ref([])
  const nextCursor = ref(0)
  const unreadCount = ref(0)
  const outbox = ref([])

  async function fetchPage(cursor = 0, limit = 20, readState = '') {
    const params = { cursor, limit }
    if (readState === 'read' || readState === 'unread') params.read = readState
    const { data } = await axios.get('/api/v1/users/me/notifications', { params })
    items.value = cursor === 0 ? data.items : [...items.value, ...data.items]
    nextCursor.value = data.next_cursor ?? 0
    return data
  }

  async function fetchUnreadCount() {
    const { data } = await axios.get('/api/v1/users/me/notifications/unread-count')
    unreadCount.value = data.unread_count ?? 0
    return unreadCount.value
  }

  async function markRead(id) {
    await axios.patch(`/api/v1/users/me/notifications/${id}/read`)
    items.value = items.value.map((n) => (n.id === id ? { ...n, is_read: true } : n))
    if (unreadCount.value > 0) unreadCount.value -= 1
  }

  async function markAllRead() {
    await axios.patch('/api/v1/users/me/notifications/read-all')
    items.value = items.value.map((n) => ({ ...n, is_read: true }))
    unreadCount.value = 0
  }

  async function fetchOutbox() {
    const { data } = await axios.get('/api/v1/users/me/notifications/outbox')
    outbox.value = data.items
    return data.items
  }

  function setUnreadFromAuthMe(n) {
    if (typeof n === 'number') unreadCount.value = n
  }

  return {
    items,
    nextCursor,
    unreadCount,
    outbox,
    fetchPage,
    fetchUnreadCount,
    markRead,
    markAllRead,
    fetchOutbox,
    setUnreadFromAuthMe,
  }
})
