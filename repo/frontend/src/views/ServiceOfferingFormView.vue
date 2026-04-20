<template>
  <div class="offering-form-view">
    <h1>{{ isEdit ? 'Edit Offering' : 'New Offering' }}</h1>

    <form class="offering-form" @submit.prevent="handleSubmit">
      <div class="field" :class="{ error: errors.name }">
        <label for="of-name">Name <span class="required">*</span></label>
        <input
          id="of-name"
          v-model="form.name"
          type="text"
          maxlength="200"
          placeholder="Service name"
          data-testid="input-name"
          @blur="validateName"
        />
        <span v-if="errors.name" class="error-msg">{{ errors.name }}</span>
      </div>

      <div class="field" :class="{ error: errors.category_id }">
        <label for="of-category">Category <span class="required">*</span></label>
        <select
          id="of-category"
          v-model="form.category_id"
          data-testid="select-category"
          @change="errors.category_id = ''"
        >
          <option value="0" disabled>Select a category…</option>
          <option v-for="cat in catalog.categories" :key="cat.id" :value="cat.id">
            {{ cat.name }}
          </option>
        </select>
        <span v-if="errors.category_id" class="error-msg">{{ errors.category_id }}</span>
      </div>

      <div class="field">
        <label for="of-desc">Description</label>
        <textarea
          id="of-desc"
          v-model="form.description"
          rows="3"
          placeholder="Describe the service…"
          data-testid="input-description"
        ></textarea>
      </div>

      <div class="field" :class="{ error: errors.base_price }">
        <label for="of-price">Base Price ($)</label>
        <input
          id="of-price"
          v-model.number="form.base_price"
          type="number"
          min="0"
          step="0.01"
          placeholder="0.00"
          data-testid="input-price"
        />
        <span v-if="errors.base_price" class="error-msg">{{ errors.base_price }}</span>
      </div>

      <div class="field" :class="{ error: errors.duration_minutes }">
        <label for="of-duration">Duration (minutes) <span class="required">*</span></label>
        <input
          id="of-duration"
          v-model.number="form.duration_minutes"
          type="number"
          min="1"
          placeholder="60"
          data-testid="input-duration"
        />
        <span v-if="errors.duration_minutes" class="error-msg">{{ errors.duration_minutes }}</span>
      </div>

      <div v-if="submitError" class="form-error">{{ submitError }}</div>

      <div class="form-actions">
        <button type="button" class="btn-cancel" @click="$router.back()">Cancel</button>
        <button type="submit" class="btn-submit" data-testid="btn-submit-offering" :disabled="submitting">
          {{ submitting ? 'Saving…' : (isEdit ? 'Update Offering' : 'Create Offering') }}
        </button>
      </div>
    </form>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useCatalogStore } from '@/stores/catalog'

const route = useRoute()
const router = useRouter()
const catalog = useCatalogStore()

const isEdit = computed(() => !!route.params.id)

const form = ref({
  name: '',
  category_id: 0,
  description: '',
  base_price: 0,
  duration_minutes: 60,
})

const errors = ref({ name: '', category_id: '', base_price: '', duration_minutes: '' })
const submitError = ref('')
const submitting = ref(false)

onMounted(async () => {
  await catalog.fetchCategories()
  if (isEdit.value) {
    const offering = await catalog.fetchOffering(Number(route.params.id))
    form.value = {
      name: offering.name,
      category_id: offering.category_id,
      description: offering.description ?? '',
      base_price: offering.base_price,
      duration_minutes: offering.duration_minutes,
    }
  }
})

function validateName() {
  errors.value.name = form.value.name.trim() === '' ? 'Name is required.' : ''
}

function validate() {
  errors.value.name = form.value.name.trim() === '' ? 'Name is required.' : ''
  errors.value.category_id = form.value.category_id === 0 ? 'Category is required.' : ''
  errors.value.base_price = form.value.base_price < 0 ? 'Price must be non-negative.' : ''
  errors.value.duration_minutes = form.value.duration_minutes <= 0 ? 'Duration must be at least 1 minute.' : ''
  return Object.values(errors.value).every((e) => e === '')
}

async function handleSubmit() {
  if (!validate()) return
  submitting.value = true
  submitError.value = ''
  try {
    const payload = {
      name: form.value.name.trim(),
      category_id: form.value.category_id,
      description: form.value.description,
      base_price: form.value.base_price,
      duration_minutes: form.value.duration_minutes,
    }
    if (isEdit.value) {
      const updated = await catalog.updateOffering(Number(route.params.id), payload)
      router.push(`/catalog/${updated.id}`)
    } else {
      const created = await catalog.createOffering(payload)
      router.push(`/catalog/${created.id}`)
    }
  } catch (err) {
    submitError.value = err.response?.data?.error?.message ?? 'Failed to save. Please try again.'
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.offering-form-view { max-width: 560px; }

h1 { margin-bottom: 1.5rem; }

.offering-form { display: flex; flex-direction: column; gap: .9rem; }

.field label {
  display: block;
  font-size: .85rem;
  font-weight: 600;
  color: #374151;
  margin-bottom: .25rem;
}

.field input,
.field select,
.field textarea {
  width: 100%;
  padding: .45rem .65rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  background: #fff;
  box-sizing: border-box;
  font-family: inherit;
}

.field textarea { resize: vertical; }

.field.error input,
.field.error select,
.field.error textarea { border-color: #dc2626; }

.error-msg { display: block; color: #dc2626; font-size: .8rem; margin-top: .2rem; }

.required { color: #dc2626; }

.form-error {
  background: #fef2f2;
  border: 1px solid #fca5a5;
  border-radius: 6px;
  padding: .6rem .9rem;
  color: #b91c1c;
  font-size: .85rem;
}

.form-actions { display: flex; justify-content: flex-end; gap: .75rem; margin-top: .5rem; }

.btn-cancel {
  background: #f3f4f6;
  color: #374151;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: .5rem 1rem;
  font-size: .9rem;
  cursor: pointer;
}
.btn-cancel:hover { background: #e5e7eb; }

.btn-submit {
  background: #4f46e5;
  color: #fff;
  border: none;
  border-radius: 8px;
  padding: .5rem 1.25rem;
  font-size: .9rem;
  cursor: pointer;
  transition: background .15s;
}
.btn-submit:hover:not(:disabled) { background: #4338ca; }
.btn-submit:disabled { opacity: .55; cursor: not-allowed; }
</style>
