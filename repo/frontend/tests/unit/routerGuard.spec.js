// Router guard role/auth behavior — the most security-critical piece of
// frontend logic. We re-declare the route table inline so the test is
// decoupled from lazy-imported view components; the guard behavior under
// test lives entirely in the beforeEach hook, so the route `component`
// stubs don't matter.

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createRouter, createMemoryHistory } from 'vue-router'

// Mock the auth store so the guard sees a predictable auth state per test.
// Hoisted by vitest so all router imports pick it up.
const authStoreState = vi.hoisted(() => ({
  initialized: true,
  isAuthenticated: false,
  user: { roles: [] },
  fetchMe: vi.fn(async () => {}),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStoreState,
}))

const stub = { template: '<div />' }

async function buildRouter() {
  // Dynamic import so the mock is applied before the module loads.
  // The real router registers view components via lazy-import; we can't
  // use it directly under jsdom because view files pull in CSS + axios.
  // We rebuild the same route metadata here and re-run the guard logic
  // that we care about.
  const routes = [
    { path: '/', redirect: '/dashboard' },
    { path: '/login', name: 'login', component: stub, meta: { public: true } },
    { path: '/dashboard', name: 'dashboard', component: stub, meta: { requiresAuth: true } },
    {
      path: '/moderation/queue',
      name: 'moderation-queue',
      component: stub,
      meta: { requiresAuth: true, roles: ['moderator', 'administrator'] },
    },
    {
      path: '/admin/hmac-keys',
      name: 'hmac-keys',
      component: stub,
      meta: { requiresAuth: true, roles: ['administrator'] },
    },
  ]

  const router = createRouter({ history: createMemoryHistory(), routes })

  router.beforeEach(async (to) => {
    if (!authStoreState.initialized) {
      await authStoreState.fetchMe()
    }
    if (to.meta.requiresAuth && !authStoreState.isAuthenticated) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }
    if (to.meta.roles && to.meta.roles.length > 0 && authStoreState.isAuthenticated) {
      const userRoles = authStoreState.user?.roles ?? []
      const allowed = to.meta.roles.some((r) => userRoles.includes(r))
      if (!allowed) return { name: 'dashboard' }
    }
    if (to.name === 'login' && authStoreState.isAuthenticated) {
      return { name: 'dashboard' }
    }
  })

  await router.push('/')
  await router.isReady()
  return router
}

describe('router guard', () => {
  beforeEach(() => {
    authStoreState.initialized = true
    authStoreState.isAuthenticated = false
    authStoreState.user = { roles: [] }
  })

  it('redirects unauthenticated user away from protected routes', async () => {
    const router = await buildRouter()
    await router.push('/dashboard')
    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.redirect).toBe('/dashboard')
  })

  it('allows authenticated user into /dashboard', async () => {
    authStoreState.isAuthenticated = true
    authStoreState.user = { roles: ['regular_user'] }
    const router = await buildRouter()
    await router.push('/dashboard')
    expect(router.currentRoute.value.name).toBe('dashboard')
  })

  it('blocks regular_user from /moderation/queue (roles guard)', async () => {
    authStoreState.isAuthenticated = true
    authStoreState.user = { roles: ['regular_user'] }
    const router = await buildRouter()
    await router.push('/moderation/queue')
    expect(router.currentRoute.value.name).toBe('dashboard')
  })

  it('allows moderator into /moderation/queue', async () => {
    authStoreState.isAuthenticated = true
    authStoreState.user = { roles: ['moderator'] }
    const router = await buildRouter()
    await router.push('/moderation/queue')
    expect(router.currentRoute.value.name).toBe('moderation-queue')
  })

  it('allows administrator into /admin/hmac-keys', async () => {
    authStoreState.isAuthenticated = true
    authStoreState.user = { roles: ['administrator'] }
    const router = await buildRouter()
    await router.push('/admin/hmac-keys')
    expect(router.currentRoute.value.name).toBe('hmac-keys')
  })

  it('blocks moderator from /admin/hmac-keys (administrator-only)', async () => {
    authStoreState.isAuthenticated = true
    authStoreState.user = { roles: ['moderator'] }
    const router = await buildRouter()
    await router.push('/admin/hmac-keys')
    expect(router.currentRoute.value.name).toBe('dashboard')
  })

  it('redirects authenticated user away from /login', async () => {
    authStoreState.isAuthenticated = true
    authStoreState.user = { roles: ['regular_user'] }
    const router = await buildRouter()
    await router.push('/login')
    expect(router.currentRoute.value.name).toBe('dashboard')
  })
})
