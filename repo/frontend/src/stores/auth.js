import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import axios from 'axios'

export const useAuthStore = defineStore('auth', () => {
  const user = ref(null)
  const csrfToken = ref('')
  const unreadCount = ref(0)
  const initialized = ref(false)

  const isAuthenticated = computed(() => user.value !== null)

  // Attach CSRF token to all mutating requests
  axios.interceptors.request.use((config) => {
    const mutating = ['post', 'put', 'patch', 'delete']
    if (mutating.includes(config.method?.toLowerCase()) && csrfToken.value) {
      config.headers['X-CSRF-Token'] = csrfToken.value
    }
    return config
  })

  // Handle 401 globally — redirect to login
  axios.interceptors.response.use(
    (res) => res,
    (err) => {
      if (err.response?.status === 401) {
        user.value = null
        csrfToken.value = ''
        if (window.location.pathname !== '/login') {
          window.location.href = '/login'
        }
      }
      return Promise.reject(err)
    }
  )

  async function fetchMe() {
    try {
      const { data } = await axios.get('/api/v1/auth/me')
      user.value = data.user
      csrfToken.value = data.csrf_token
      if (typeof data.unread_count === 'number') unreadCount.value = data.unread_count
    } catch {
      user.value = null
      csrfToken.value = ''
      unreadCount.value = 0
    } finally {
      initialized.value = true
    }
  }

  async function login(username, password) {
    const { data } = await axios.post('/api/v1/auth/login', { username, password })
    user.value = data.user
    csrfToken.value = data.csrf_token
    return data
  }

  async function register(username, email, password, displayName) {
    const { data } = await axios.post('/api/v1/auth/register', {
      username,
      email,
      password,
      display_name: displayName,
    })
    return data
  }

  async function logout() {
    try {
      await axios.post('/api/v1/auth/logout')
    } finally {
      user.value = null
      csrfToken.value = ''
    }
  }

  return {
    user,
    csrfToken,
    unreadCount,
    initialized,
    isAuthenticated,
    fetchMe,
    login,
    register,
    logout,
  }
})
