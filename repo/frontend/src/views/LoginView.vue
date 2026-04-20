<template>
  <div class="login-page">
    <div class="login-card">
      <h1>Sign In</h1>

      <form @submit.prevent="handleLogin" novalidate>
        <!-- Username -->
        <div class="field" :class="{ error: errors.username }">
          <label for="username">Username</label>
          <input
            id="username"
            v-model="form.username"
            type="text"
            autocomplete="username"
            placeholder="Enter your username"
            @blur="validateField('username')"
            data-testid="input-username"
          />
          <span class="field-error" v-if="errors.username">{{ errors.username }}</span>
        </div>

        <!-- Password -->
        <div class="field" :class="{ error: errors.password }">
          <label for="password">Password</label>
          <input
            id="password"
            v-model="form.password"
            type="password"
            autocomplete="current-password"
            placeholder="Enter your password"
            @blur="validateField('password')"
            data-testid="input-password"
          />
          <span class="field-error" v-if="errors.password">{{ errors.password }}</span>
        </div>

        <!-- Lockout message -->
        <div class="lockout-notice" v-if="lockoutSeconds > 0" data-testid="lockout-notice">
          Account locked. Try again in {{ lockoutSeconds }}s.
        </div>

        <!-- API error -->
        <div class="api-error" v-if="apiError" data-testid="api-error">{{ apiError }}</div>

        <button
          type="submit"
          :disabled="submitting || lockoutSeconds > 0"
          data-testid="btn-login"
        >
          {{ submitting ? 'Signing in…' : 'Sign In' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const form = ref({ username: '', password: '' })
const errors = ref({ username: '', password: '' })
const apiError = ref('')
const submitting = ref(false)
const lockoutSeconds = ref(0)

let lockoutTimer = null

function validateField(field) {
  if (field === 'username') {
    errors.value.username = form.value.username.trim() ? '' : 'Username is required'
  }
  if (field === 'password') {
    errors.value.password = form.value.password ? '' : 'Password is required'
  }
}

function validateAll() {
  validateField('username')
  validateField('password')
  return !errors.value.username && !errors.value.password
}

async function handleLogin() {
  if (!validateAll()) return

  submitting.value = true
  apiError.value = ''

  try {
    await auth.login(form.value.username, form.value.password)
    const redirect = route.query.redirect || '/dashboard'
    router.push(redirect)
  } catch (err) {
    const data = err.response?.data?.error
    if (data?.code === 'account_locked') {
      lockoutSeconds.value = data.details?.remaining_seconds || 900
      startLockoutCountdown()
    } else {
      apiError.value = data?.message || 'Login failed. Please try again.'
    }
  } finally {
    submitting.value = false
  }
}

function startLockoutCountdown() {
  clearInterval(lockoutTimer)
  lockoutTimer = setInterval(() => {
    if (lockoutSeconds.value > 0) {
      lockoutSeconds.value--
    } else {
      clearInterval(lockoutTimer)
    }
  }, 1000)
}

onUnmounted(() => clearInterval(lockoutTimer))
</script>

<style scoped>
.login-page {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 60vh;
}

.login-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 2rem 2.5rem;
  width: 100%;
  max-width: 400px;
  box-shadow: 0 2px 12px rgba(0,0,0,.06);
}

h1 {
  margin: 0 0 1.5rem;
  font-size: 1.5rem;
  color: #1a1a2e;
}

.field {
  margin-bottom: 1.25rem;
}

label {
  display: block;
  font-size: .875rem;
  font-weight: 500;
  margin-bottom: .4rem;
  color: #374151;
}

input {
  width: 100%;
  padding: .6rem .75rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  box-sizing: border-box;
  transition: border-color .15s;
}

input:focus {
  outline: none;
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99,102,241,.15);
}

.field.error input {
  border-color: #ef4444;
}

.field-error {
  display: block;
  font-size: .78rem;
  color: #ef4444;
  margin-top: .3rem;
}

.api-error {
  background: #fef2f2;
  border: 1px solid #fca5a5;
  border-radius: 6px;
  padding: .6rem .75rem;
  font-size: .875rem;
  color: #b91c1c;
  margin-bottom: 1rem;
}

.lockout-notice {
  background: #fefce8;
  border: 1px solid #fde047;
  border-radius: 6px;
  padding: .6rem .75rem;
  font-size: .875rem;
  color: #854d0e;
  margin-bottom: 1rem;
}

button[type="submit"] {
  width: 100%;
  padding: .7rem;
  background: #4f46e5;
  color: #fff;
  border: none;
  border-radius: 6px;
  font-size: .95rem;
  font-weight: 600;
  cursor: pointer;
  transition: background .15s;
}

button[type="submit"]:hover:not(:disabled) {
  background: #4338ca;
}

button[type="submit"]:disabled {
  opacity: .6;
  cursor: not-allowed;
}
</style>
