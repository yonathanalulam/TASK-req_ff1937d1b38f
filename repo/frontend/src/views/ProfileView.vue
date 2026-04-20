<template>
  <div class="profile-view">
    <h1>My Profile</h1>

    <div v-if="loading" class="loading">Loading…</div>

    <form v-else class="profile-form" @submit.prevent="handleSave" novalidate>
      <div class="form-row">
        <label>Username</label>
        <input :value="profileStore.profile?.username" disabled class="input-disabled" />
      </div>

      <div class="form-row">
        <label>Email</label>
        <input :value="profileStore.profile?.email" disabled class="input-disabled" />
      </div>

      <div class="field" :class="{ error: errors.display_name }">
        <label>Display Name <span class="required">*</span></label>
        <input
          v-model="form.display_name"
          data-testid="input-display-name"
          placeholder="Your display name"
          @blur="validateField('display_name')"
        />
        <span v-if="errors.display_name" class="error-msg">{{ errors.display_name }}</span>
      </div>

      <div class="field">
        <label>Phone</label>
        <input
          v-model="form.phone"
          data-testid="input-phone"
          placeholder="e.g. 4155551234"
        />
        <p class="hint">Stored encrypted. Shown masked to non-administrators.</p>
        <p v-if="profileStore.profile?.phone" class="masked-phone" data-testid="masked-phone">
          Current: {{ profileStore.profile.phone }}
        </p>
      </div>

      <div class="field">
        <label>Bio</label>
        <textarea
          v-model="form.bio"
          data-testid="input-bio"
          rows="3"
          placeholder="A short bio…"
        />
      </div>

      <div class="field">
        <label>Avatar URL</label>
        <input
          v-model="form.avatar_url"
          data-testid="input-avatar-url"
          placeholder="https://…"
        />
        <img v-if="form.avatar_url" :src="form.avatar_url" alt="Avatar preview" class="avatar-preview" />
      </div>

      <div v-if="apiError" class="api-error" data-testid="profile-api-error">{{ apiError }}</div>

      <button type="submit" class="btn-primary" data-testid="btn-save-profile" :disabled="saving">
        {{ saving ? 'Saving…' : 'Save Profile' }}
      </button>
    </form>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useProfileStore } from '@/stores/profile'
import { useToast } from '@/composables/useToast'

const profileStore = useProfileStore()
const { success, error: toastError } = useToast()

const loading = ref(true)
const saving = ref(false)
const apiError = ref('')

const form = ref({
  display_name: '',
  phone: '',
  bio: '',
  avatar_url: '',
})

const errors = ref({
  display_name: '',
})

onMounted(async () => {
  try {
    const p = await profileStore.fetchProfile()
    form.value.display_name = p.display_name || ''
    form.value.bio = p.bio || ''
    form.value.avatar_url = p.avatar_url || ''
    // phone is intentionally not pre-filled (show masked version separately)
  } catch {
    toastError('Failed to load profile.')
  } finally {
    loading.value = false
  }
})

function validateField(field) {
  if (field === 'display_name') {
    errors.value.display_name = form.value.display_name.trim()
      ? ''
      : 'Display name is required.'
  }
}

function validateAll() {
  validateField('display_name')
  return !errors.value.display_name
}

async function handleSave() {
  apiError.value = ''
  if (!validateAll()) return

  saving.value = true
  try {
    await profileStore.updateProfile({
      display_name: form.value.display_name.trim(),
      bio: form.value.bio,
      avatar_url: form.value.avatar_url,
      phone: form.value.phone,
    })
    form.value.phone = '' // clear raw phone after save
    success('Profile saved.')
  } catch (err) {
    const msg = err.response?.data?.error?.message || 'Failed to save profile.'
    apiError.value = msg
    toastError(msg)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.profile-view { max-width: 540px; }

h1 { margin-bottom: 1.5rem; font-size: 1.4rem; color: #1a1a2e; }

.profile-form { display: flex; flex-direction: column; gap: 1rem; }

.form-row, .field { display: flex; flex-direction: column; gap: .3rem; }

label { font-size: .85rem; font-weight: 600; color: #374151; }

input, textarea {
  padding: .5rem .75rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .95rem;
  outline: none;
  transition: border-color .15s;
}
input:focus, textarea:focus { border-color: #6366f1; }

.input-disabled { background: #f3f4f6; color: #6b7280; cursor: default; }

.field.error input { border-color: #ef4444; }
.error-msg { font-size: .8rem; color: #ef4444; }

.hint { font-size: .78rem; color: #9ca3af; margin: 0; }

.masked-phone { font-size: .85rem; color: #4b5563; margin: 0; }

.avatar-preview {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  object-fit: cover;
  margin-top: .25rem;
  border: 2px solid #e5e7eb;
}

.api-error {
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #991b1b;
  border-radius: 6px;
  padding: .5rem .75rem;
  font-size: .85rem;
}

.btn-primary {
  background: #6366f1;
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: .6rem 1.25rem;
  font-size: .95rem;
  cursor: pointer;
  align-self: flex-start;
  transition: background .15s;
}
.btn-primary:hover:not(:disabled) { background: #4f46e5; }
.btn-primary:disabled { opacity: .6; cursor: default; }

.required { color: #ef4444; }
</style>
