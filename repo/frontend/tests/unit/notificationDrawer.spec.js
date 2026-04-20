// Unit tests for NotificationCenterDrawer — verifies empty vs populated
// rendering, the mark-read affordances, and mark-all state gating.

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

const notifStore = vi.hoisted(() => ({
  items: [],
  unreadCount: 0,
  nextCursor: 0,
  fetchPage: vi.fn(async () => {}),
  markRead: vi.fn(async () => {}),
  markAllRead: vi.fn(async () => {}),
}))

vi.mock('@/stores/notification', () => ({
  useNotificationStore: () => notifStore,
}))

import NotificationCenterDrawer from '@/components/NotificationCenterDrawer.vue'

function mountDrawer() {
  return mount(NotificationCenterDrawer, {
    global: { stubs: { 'router-link': true } },
  })
}

describe('NotificationCenterDrawer', () => {
  beforeEach(() => {
    notifStore.items = []
    notifStore.unreadCount = 0
    notifStore.nextCursor = 0
    notifStore.fetchPage.mockReset().mockResolvedValue()
    notifStore.markRead.mockReset().mockResolvedValue()
    notifStore.markAllRead.mockReset().mockResolvedValue()
  })

  it('renders the empty state when there are no items', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    expect(wrapper.find('[data-testid="notification-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="btn-mark-all-read"]').attributes('disabled')).toBeDefined()
  })

  it('renders items with unread styling + mark-read button when any are unread', async () => {
    notifStore.items = [
      { id: 1, title: 'Hello', body: 'body', is_read: false, created_at: '2026-04-20T10:00:00Z' },
      { id: 2, title: 'Read one', body: 'body', is_read: true, created_at: '2026-04-20T11:00:00Z' },
    ]
    notifStore.unreadCount = 1
    const wrapper = mountDrawer()
    await flushPromises()
    const titles = wrapper.findAll('[data-testid="notification-title"]').map((w) => w.text())
    expect(titles).toContain('Hello')
    expect(titles).toContain('Read one')
    // Only the unread item gets a mark-read button.
    const marks = wrapper.findAll('[data-testid="btn-mark-read"]')
    expect(marks).toHaveLength(1)
    await marks[0].trigger('click')
    expect(notifStore.markRead).toHaveBeenCalledWith(1)
  })

  it('enables mark-all-read when unreadCount > 0 and calls the store', async () => {
    notifStore.items = [
      { id: 3, title: 'T', body: 'B', is_read: false, created_at: '2026-04-20T12:00:00Z' },
    ]
    notifStore.unreadCount = 1
    const wrapper = mountDrawer()
    await flushPromises()
    const btn = wrapper.find('[data-testid="btn-mark-all-read"]')
    expect(btn.attributes('disabled')).toBeUndefined()
    await btn.trigger('click')
    expect(notifStore.markAllRead).toHaveBeenCalledTimes(1)
  })

  it('shows "Load More" only when nextCursor > 0', async () => {
    notifStore.items = [
      { id: 4, title: 'n', body: 'b', is_read: true, created_at: '2026-04-20T09:00:00Z' },
    ]
    notifStore.nextCursor = 42
    const wrapper = mountDrawer()
    await flushPromises()
    expect(wrapper.find('[data-testid="btn-load-more-notifs"]').exists()).toBe(true)
  })
})
