// Unit tests for stores/auth.js — hit the real store code via Pinia and
// stub axios at the module boundary so no network calls happen.

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('axios', () => {
  const get = vi.fn()
  const post = vi.fn()
  return {
    default: {
      get,
      post,
      interceptors: {
        request: { use: vi.fn() },
        response: { use: vi.fn() },
      },
    },
  }
})

import axios from 'axios'
import { useAuthStore } from '@/stores/auth'

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    axios.get.mockReset()
    axios.post.mockReset()
  })

  it('starts unauthenticated and uninitialized', () => {
    const store = useAuthStore()
    expect(store.isAuthenticated).toBe(false)
    expect(store.initialized).toBe(false)
    expect(store.user).toBeNull()
  })

  it('fetchMe populates user + csrf + unread_count on 200', async () => {
    axios.get.mockResolvedValueOnce({
      data: {
        user: { id: 1, username: 'alice', roles: ['regular_user'] },
        csrf_token: 'csrf-xyz',
        unread_count: 7,
      },
    })
    const store = useAuthStore()
    await store.fetchMe()
    expect(store.isAuthenticated).toBe(true)
    expect(store.user.username).toBe('alice')
    expect(store.csrfToken).toBe('csrf-xyz')
    expect(store.unreadCount).toBe(7)
    expect(store.initialized).toBe(true)
  })

  it('fetchMe clears state on error and still marks initialized', async () => {
    axios.get.mockRejectedValueOnce(new Error('401'))
    const store = useAuthStore()
    await store.fetchMe()
    expect(store.user).toBeNull()
    expect(store.csrfToken).toBe('')
    expect(store.unreadCount).toBe(0)
    expect(store.initialized).toBe(true)
  })

  it('login stores the returned user and csrf token', async () => {
    axios.post.mockResolvedValueOnce({
      data: {
        user: { id: 2, username: 'bob', roles: ['administrator'] },
        csrf_token: 'csrf-bob',
      },
    })
    const store = useAuthStore()
    await store.login('bob', 'password')
    expect(store.user.id).toBe(2)
    expect(store.csrfToken).toBe('csrf-bob')
  })

  it('logout clears state even if the request fails', async () => {
    const store = useAuthStore()
    // Seed a session
    axios.get.mockResolvedValueOnce({
      data: { user: { id: 3 }, csrf_token: 'seed' },
    })
    await store.fetchMe()
    expect(store.isAuthenticated).toBe(true)

    axios.post.mockRejectedValueOnce(new Error('network'))
    await store.logout()
    expect(store.user).toBeNull()
    expect(store.csrfToken).toBe('')
  })
})
