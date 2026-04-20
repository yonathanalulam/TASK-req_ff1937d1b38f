<template>
  <div class="app-shell">
    <header class="app-header">
      <nav>
        <router-link to="/" class="brand">Service Portal</router-link>

        <template v-if="auth.isAuthenticated">
          <router-link to="/dashboard">Dashboard</router-link>
          <router-link to="/catalog">Catalog</router-link>
          <router-link to="/tickets">Tickets</router-link>
          <router-link
            v-if="hasModRole"
            to="/moderation/queue"
            data-testid="link-moderation"
          >Moderation</router-link>
          <router-link
            v-if="hasDataOpsRole"
            to="/dataops"
            data-testid="link-dataops"
          >Data Ops</router-link>
          <router-link
            v-if="hasAdminRole"
            to="/admin/legal-holds"
            data-testid="link-legal-holds"
          >Legal Holds</router-link>
          <router-link
            v-if="hasAdminRole"
            to="/admin/hmac-keys"
            data-testid="link-hmac-keys"
          >HMAC Keys</router-link>
          <router-link to="/profile">Profile</router-link>
          <router-link to="/preferences">Preferences</router-link>
          <router-link to="/addresses">Addresses</router-link>
          <router-link to="/privacy" data-testid="link-privacy">Privacy</router-link>
          <span class="nav-user">{{ auth.user?.display_name }}</span>
          <button
            class="btn-bell"
            data-testid="btn-notifications"
            aria-label="Open notifications"
            @click="showDrawer = true"
          >
            <span class="bell-icon">&#x1f514;</span>
            <span
              v-if="auth.unreadCount > 0"
              class="bell-badge"
              data-testid="notification-badge"
            >
              {{ auth.unreadCount > 9 ? '9+' : auth.unreadCount }}
            </span>
          </button>
          <button class="btn-logout" @click="handleLogout">Logout</button>
        </template>
        <template v-else>
          <router-link to="/login">Sign In</router-link>
        </template>
      </nav>
    </header>

    <main class="app-main">
      <RouterView />
    </main>

    <NotificationCenterDrawer v-if="showDrawer" @close="closeDrawer" />

    <ToastMessage ref="toastRef" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useNotificationStore } from '@/stores/notification'
import { useToast } from '@/composables/useToast'
import { useRouter } from 'vue-router'
import ToastMessage from '@/components/ToastMessage.vue'
import NotificationCenterDrawer from '@/components/NotificationCenterDrawer.vue'

const auth = useAuthStore()
const notif = useNotificationStore()
const router = useRouter()
const { toastRef, success, error } = useToast()

const showDrawer = ref(false)

// Roles that can access the moderation dashboard
const hasModRole = computed(() => {
  const roles = auth.user?.roles ?? []
  return roles.includes('moderator') || roles.includes('administrator')
})

// Roles that can access admin-only views (legal holds, etc.)
const hasAdminRole = computed(() => {
  const roles = auth.user?.roles ?? []
  return roles.includes('administrator')
})

// Roles that can access the Data Operations console (sources, jobs, catalog).
// Administrator has superset access.
const hasDataOpsRole = computed(() => {
  const roles = auth.user?.roles ?? []
  return roles.includes('data_operator') || roles.includes('administrator')
})

// Keep auth.unreadCount and notif.unreadCount in sync
watch(() => auth.unreadCount, (n) => notif.setUnreadFromAuthMe(n))

// Poll for fresh unread count every 60 seconds while authenticated.
let pollInterval = null

function startPolling() {
  if (pollInterval) return
  pollInterval = setInterval(async () => {
    if (!auth.isAuthenticated) return
    try {
      const n = await notif.fetchUnreadCount()
      auth.unreadCount = n
    } catch {
      /* swallow polling errors */
    }
  }, 60_000)
}

function stopPolling() {
  if (pollInterval) {
    clearInterval(pollInterval)
    pollInterval = null
  }
}

onMounted(() => {
  if (auth.isAuthenticated) startPolling()
})

watch(() => auth.isAuthenticated, (now) => {
  if (now) startPolling()
  else stopPolling()
})

onUnmounted(stopPolling)

async function closeDrawer() {
  showDrawer.value = false
  // Refresh badge after the drawer closes (likely user marked items read)
  try {
    const n = await notif.fetchUnreadCount()
    auth.unreadCount = n
  } catch { /* noop */ }
}

async function handleLogout() {
  try {
    await auth.logout()
    success('Signed out successfully.')
    router.push('/login')
  } catch {
    error('Logout failed. Please try again.')
  }
}
</script>

<style scoped>
.app-shell {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  font-family: system-ui, sans-serif;
  background: #f8fafc;
}

.app-header {
  background: #1a1a2e;
  color: #fff;
  padding: 0 1.5rem;
  position: sticky;
  top: 0;
  z-index: 100;
}

.app-header nav {
  display: flex;
  align-items: center;
  gap: 1.25rem;
  height: 3.5rem;
}

.brand {
  font-weight: 700;
  font-size: 1.1rem;
  margin-right: auto;
  color: #fff;
  text-decoration: none;
}

.app-header a {
  color: #c7d2fe;
  text-decoration: none;
  font-size: .9rem;
  transition: color .15s;
}

.app-header a:hover,
.app-header a.router-link-active { color: #fff; }

.nav-user {
  font-size: .85rem;
  color: #a5b4fc;
}

.btn-logout {
  background: rgba(255,255,255,.1);
  color: #fff;
  border: 1px solid rgba(255,255,255,.2);
  border-radius: 6px;
  padding: .3rem .75rem;
  font-size: .85rem;
  cursor: pointer;
  transition: background .15s;
}

.btn-logout:hover { background: rgba(255,255,255,.2); }

.btn-bell {
  position: relative;
  background: transparent;
  border: none;
  cursor: pointer;
  padding: .15rem .35rem;
  color: #fff;
  font-size: 1.05rem;
  line-height: 1;
}
.bell-icon { display: inline-block; }
.bell-badge {
  position: absolute;
  top: -.15rem;
  right: -.25rem;
  background: #ef4444;
  color: #fff;
  font-size: .65rem;
  font-weight: 700;
  padding: .05rem .35rem;
  border-radius: 999px;
  min-width: 1rem;
  text-align: center;
  line-height: 1.1;
}

.app-main {
  flex: 1;
  padding: 2rem 1.5rem;
  max-width: 1200px;
  margin: 0 auto;
  width: 100%;
  box-sizing: border-box;
}
</style>
