<template>
  <div class="preferences-view">
    <h1>Preferences</h1>

    <div v-if="loading" class="loading">Loading…</div>

    <form v-else class="pref-form" @submit.prevent="handleSave" novalidate>
      <section class="pref-section">
        <h2>Notifications</h2>

        <label class="toggle-row">
          <input
            type="checkbox"
            v-model="form.notify_in_app"
            data-testid="toggle-notify-in-app"
          />
          <span class="toggle-label">
            <strong>In-app notifications</strong>
            <small>Show alerts inside the portal</small>
          </span>
        </label>
      </section>

      <section class="pref-section">
        <h2>Content Filters</h2>

        <div class="field">
          <label>Muted tag IDs (comma-separated)</label>
          <input
            v-model="mutedTagsRaw"
            data-testid="input-muted-tags"
            placeholder="e.g. 1, 2, 3"
          />
          <span class="hint">Content tagged with these IDs will be hidden.</span>
        </div>

        <div class="field">
          <label>Muted author IDs (comma-separated)</label>
          <input
            v-model="mutedAuthorsRaw"
            data-testid="input-muted-authors"
            placeholder="e.g. 42, 99"
          />
          <span class="hint">Posts from these users will be hidden.</span>
        </div>
      </section>

      <div v-if="apiError" class="api-error" data-testid="pref-api-error">{{ apiError }}</div>

      <button type="submit" class="btn-primary" data-testid="btn-save-prefs" :disabled="saving">
        {{ saving ? 'Saving…' : 'Save Preferences' }}
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

const form = ref({ notify_in_app: true })
const mutedTagsRaw = ref('')
const mutedAuthorsRaw = ref('')

onMounted(async () => {
  try {
    const prefs = await profileStore.fetchPreferences()
    form.value.notify_in_app = prefs.notify_in_app ?? true
    mutedTagsRaw.value = (prefs.muted_tags ?? []).join(', ')
    mutedAuthorsRaw.value = (prefs.muted_authors ?? []).join(', ')
  } catch {
    toastError('Failed to load preferences.')
  } finally {
    loading.value = false
  }
})

function parseIds(raw) {
  return raw
    .split(',')
    .map((s) => parseInt(s.trim(), 10))
    .filter((n) => !isNaN(n) && n > 0)
}

async function handleSave() {
  apiError.value = ''
  saving.value = true
  try {
    await profileStore.updatePreferences({
      notify_in_app: form.value.notify_in_app,
      muted_tags: parseIds(mutedTagsRaw.value),
      muted_authors: parseIds(mutedAuthorsRaw.value),
    })
    success('Preferences saved.')
  } catch (err) {
    const msg = err.response?.data?.error?.message || 'Failed to save preferences.'
    apiError.value = msg
    toastError(msg)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.preferences-view { max-width: 540px; }

h1 { margin-bottom: 1.5rem; font-size: 1.4rem; color: #1a1a2e; }
h2 { font-size: 1rem; color: #374151; margin: 0 0 .75rem; }

.pref-form { display: flex; flex-direction: column; gap: 1.5rem; }

.pref-section {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1.25rem;
}

.toggle-row {
  display: flex;
  align-items: flex-start;
  gap: .75rem;
  cursor: pointer;
}

.toggle-row input[type="checkbox"] {
  width: 18px;
  height: 18px;
  margin-top: 3px;
  cursor: pointer;
  accent-color: #6366f1;
}

.toggle-label { display: flex; flex-direction: column; gap: 2px; }
.toggle-label strong { font-size: .9rem; color: #111827; }
.toggle-label small { font-size: .8rem; color: #6b7280; }

.field { display: flex; flex-direction: column; gap: .3rem; }

label { font-size: .85rem; font-weight: 600; color: #374151; }

input {
  padding: .5rem .75rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .95rem;
  outline: none;
  transition: border-color .15s;
}
input:focus { border-color: #6366f1; }

.hint { font-size: .78rem; color: #9ca3af; }

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
</style>
