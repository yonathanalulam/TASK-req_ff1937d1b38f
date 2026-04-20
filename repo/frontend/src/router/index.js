import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import HealthView from '@/views/HealthView.vue'
import LoginView from '@/views/LoginView.vue'

const routes = [
  {
    path: '/',
    redirect: '/dashboard',
  },
  {
    path: '/health',
    name: 'health',
    component: HealthView,
    meta: { public: true },
  },
  {
    path: '/login',
    name: 'login',
    component: LoginView,
    meta: { public: true },
  },
  {
    path: '/dashboard',
    name: 'dashboard',
    component: () => import('@/views/DashboardView.vue'),
    meta: { requiresAuth: true },
  },

  // ── Phase 3: User profile, preferences, address book ────────────────────────
  {
    path: '/profile',
    name: 'profile',
    component: () => import('@/views/ProfileView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/preferences',
    name: 'preferences',
    component: () => import('@/views/PreferencesView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/addresses',
    name: 'addresses',
    component: () => import('@/views/AddressBookView.vue'),
    meta: { requiresAuth: true },
  },

  // ── Phase 4: Service catalog & offerings ────────────────────────────────────
  {
    path: '/catalog',
    name: 'catalog',
    component: () => import('@/views/ServiceCatalogView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/catalog/new',
    name: 'catalog-new',
    component: () => import('@/views/ServiceOfferingFormView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/catalog/:id',
    name: 'catalog-detail',
    component: () => import('@/views/ServiceOfferingDetailView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/catalog/:id/edit',
    name: 'catalog-edit',
    component: () => import('@/views/ServiceOfferingFormView.vue'),
    meta: { requiresAuth: true },
  },

  // ── Phase 5: Tickets ────────────────────────────────────────────────────────
  {
    path: '/tickets',
    name: 'tickets',
    component: () => import('@/views/TicketListView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/tickets/new',
    name: 'ticket-new',
    component: () => import('@/views/TicketCreateView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/tickets/:id',
    name: 'ticket-detail',
    component: () => import('@/views/TicketDetailView.vue'),
    meta: { requiresAuth: true },
  },

  // ── Phase 6: Reviews & Q&A (embedded under catalog offerings) ───────────────
  {
    path: '/catalog/:id/reviews',
    name: 'offering-reviews',
    component: () => import('@/views/ReviewListView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/catalog/:id/qa',
    name: 'offering-qa',
    component: () => import('@/views/QAThreadView.vue'),
    meta: { requiresAuth: true },
  },

  // ── Phase 7: Notifications ──────────────────────────────────────────────────
  {
    path: '/notifications/outbox',
    name: 'notification-outbox',
    component: () => import('@/views/NotificationOutboxView.vue'),
    meta: { requiresAuth: true },
  },

  // ── Phase 8: Moderation ─────────────────────────────────────────────────────
  // Backend RBAC blocks /moderation/* at the API; the meta.roles here mirrors
  // that constraint so unauthorised users never land on the SPA shell of a
  // page they can't use. Backend remains the source of truth.
  {
    path: '/moderation/queue',
    name: 'moderation-queue',
    component: () => import('@/views/ModerationQueueView.vue'),
    meta: { requiresAuth: true, roles: ['moderator', 'administrator'] },
  },
  {
    path: '/moderation/violations',
    name: 'moderation-violations',
    component: () => import('@/views/ViolationHistoryView.vue'),
    meta: { requiresAuth: true, roles: ['moderator', 'administrator'] },
  },

  // ── Phase 9: Privacy Center ─────────────────────────────────────────────────
  {
    path: '/privacy',
    name: 'privacy',
    component: () => import('@/views/PrivacyCenterView.vue'),
    meta: { requiresAuth: true },
  },

  // ── Phase 10: Data Operations ───────────────────────────────────────────────
  // Dataops console — sources, jobs, and lakehouse catalog. Visible to
  // data_operator and administrator roles.
  {
    path: '/dataops',
    name: 'dataops',
    component: () => import('@/views/DataOpsView.vue'),
    meta: { requiresAuth: true, roles: ['data_operator', 'administrator'] },
  },
  // Legal holds — admin-only.
  {
    path: '/admin/legal-holds',
    name: 'legal-holds',
    component: () => import('@/views/LegalHoldView.vue'),
    meta: { requiresAuth: true, roles: ['administrator'] },
  },

  // ── Security: HMAC key lifecycle (admin-only) ───────────────────────────────
  {
    path: '/admin/hmac-keys',
    name: 'hmac-keys',
    component: () => import('@/views/HMACKeysView.vue'),
    meta: { requiresAuth: true, roles: ['administrator'] },
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

// Auth guard
router.beforeEach(async (to) => {
  const auth = useAuthStore()

  // Hydrate session on first navigation
  if (!auth.initialized) {
    await auth.fetchMe()
  }

  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }

  // Route-level role guard: if the destination route declares an allowed
  // roles list, the user must hold at least one of them. Keeps the backend
  // RBAC and frontend visibility in sync without duplicating role logic.
  if (to.meta.roles && to.meta.roles.length > 0 && auth.isAuthenticated) {
    const userRoles = auth.user?.roles ?? []
    const allowed = to.meta.roles.some((r) => userRoles.includes(r))
    if (!allowed) {
      return { name: 'dashboard' }
    }
  }

  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'dashboard' }
  }
})

export default router
