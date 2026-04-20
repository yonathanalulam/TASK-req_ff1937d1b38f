<template>
  <div class="ticket-create-view">
    <h1>New Service Request</h1>

    <form class="ticket-form" @submit.prevent="handleSubmit">
      <div class="field" :class="{ error: errors.offering_id }">
        <label for="tc-offering">Offering <span class="required">*</span></label>
        <select
          id="tc-offering"
          v-model="form.offering_id"
          data-testid="select-offering"
          @change="onOfferingChange"
        >
          <option value="0" disabled>Select an offering…</option>
          <option v-for="o in catalog.offerings" :key="o.id" :value="o.id">
            {{ o.name }}
          </option>
        </select>
        <span v-if="errors.offering_id" class="error-msg">{{ errors.offering_id }}</span>
      </div>

      <div class="field" :class="{ error: errors.category_id }">
        <label for="tc-category">Category <span class="required">*</span></label>
        <select id="tc-category" v-model="form.category_id" data-testid="select-ticket-category">
          <option value="0" disabled>Select a category…</option>
          <option v-for="c in catalog.categories" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
        <span v-if="errors.category_id" class="error-msg">{{ errors.category_id }}</span>
      </div>

      <div class="field" :class="{ error: errors.address_id }">
        <label for="tc-address">Address <span class="required">*</span></label>
        <select id="tc-address" v-model="form.address_id" data-testid="select-address">
          <option value="0" disabled>Select an address…</option>
          <option v-for="a in addressStore.addresses" :key="a.id" :value="a.id">
            {{ a.label }} — {{ a.address_line1 }}, {{ a.city }} {{ a.state }}
          </option>
        </select>
        <span v-if="errors.address_id" class="error-msg">{{ errors.address_id }}</span>
      </div>

      <div class="two-col">
        <div class="field" :class="{ error: errors.preferred_start }">
          <label for="tc-start">Preferred Start <span class="required">*</span></label>
          <input
            id="tc-start"
            v-model="form.preferred_start"
            type="datetime-local"
            data-testid="input-preferred-start"
          />
          <span v-if="errors.preferred_start" class="error-msg">{{ errors.preferred_start }}</span>
        </div>
        <div class="field" :class="{ error: errors.preferred_end }">
          <label for="tc-end">Preferred End <span class="required">*</span></label>
          <input
            id="tc-end"
            v-model="form.preferred_end"
            type="datetime-local"
            data-testid="input-preferred-end"
          />
          <span v-if="errors.preferred_end" class="error-msg">{{ errors.preferred_end }}</span>
        </div>
      </div>

      <div class="field">
        <label>Delivery Method</label>
        <div class="radio-row">
          <label><input type="radio" value="pickup" v-model="form.delivery_method" /> Pickup</label>
          <label><input type="radio" value="courier" v-model="form.delivery_method" /> Courier</label>
        </div>
      </div>

      <div class="field">
        <label for="tc-files">Attachments (JPG/PNG/PDF, max 5 files, 5 MB each)</label>
        <input
          id="tc-files"
          type="file"
          accept=".jpg,.jpeg,.png,.pdf,image/jpeg,image/png,application/pdf"
          multiple
          data-testid="input-attachments"
          @change="onFiles"
        />
        <div v-if="fileErrors.length > 0" class="error-msg">
          <div v-for="(e, i) in fileErrors" :key="i">{{ e }}</div>
        </div>
        <div v-if="selectedFiles.length > 0" class="file-list">
          <div v-for="(f, i) in selectedFiles" :key="i" class="file-chip">
            {{ f.name }} ({{ (f.size / 1024).toFixed(0) }} KB)
          </div>
        </div>
      </div>

      <div v-if="submitError" class="form-error" data-testid="ticket-error">{{ submitError }}</div>

      <div class="form-actions">
        <button type="button" class="btn-cancel" @click="$router.back()">Cancel</button>
        <button
          type="submit"
          class="btn-submit"
          data-testid="btn-submit-ticket"
          :disabled="submitting"
        >
          {{ submitting ? 'Submitting…' : 'Create Ticket' }}
        </button>
      </div>
    </form>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useCatalogStore } from '@/stores/catalog'
import { useAddressStore } from '@/stores/address'
import { useTicketStore } from '@/stores/ticket'

const router = useRouter()
const catalog = useCatalogStore()
const addressStore = useAddressStore()
const ticket = useTicketStore()

const form = ref({
  offering_id: 0,
  category_id: 0,
  address_id: 0,
  preferred_start: '',
  preferred_end: '',
  delivery_method: 'pickup',
})
const errors = ref({ offering_id: '', category_id: '', address_id: '', preferred_start: '', preferred_end: '' })
const fileErrors = ref([])
const selectedFiles = ref([])
const submitting = ref(false)
const submitError = ref('')

const MAX_FILES = 5
const MAX_BYTES = 5 * 1024 * 1024
const ALLOWED_MIME = new Set(['image/jpeg', 'image/png', 'application/pdf'])

onMounted(async () => {
  await Promise.all([
    catalog.fetchCategories(),
    catalog.fetchOfferings(),
    addressStore.fetchAddresses(),
  ])
})

function onOfferingChange() {
  const sel = catalog.offerings.find((o) => o.id === Number(form.value.offering_id))
  if (sel && form.value.category_id === 0) {
    form.value.category_id = sel.category_id
  }
}

function onFiles(e) {
  const files = Array.from(e.target.files ?? [])
  fileErrors.value = []
  if (files.length > MAX_FILES) {
    fileErrors.value.push(`At most ${MAX_FILES} files allowed`)
    selectedFiles.value = []
    return
  }
  for (const f of files) {
    if (f.size > MAX_BYTES) {
      fileErrors.value.push(`${f.name} exceeds 5 MB`)
    }
    if (!ALLOWED_MIME.has(f.type)) {
      fileErrors.value.push(`${f.name} is not JPG, PNG, or PDF`)
    }
  }
  if (fileErrors.value.length === 0) {
    selectedFiles.value = files
  }
}

function validate() {
  errors.value = { offering_id: '', category_id: '', address_id: '', preferred_start: '', preferred_end: '' }
  let ok = true
  if (!form.value.offering_id) { errors.value.offering_id = 'Required.'; ok = false }
  if (!form.value.category_id) { errors.value.category_id = 'Required.'; ok = false }
  if (!form.value.address_id) { errors.value.address_id = 'Required.'; ok = false }
  if (!form.value.preferred_start) { errors.value.preferred_start = 'Required.'; ok = false }
  if (!form.value.preferred_end) { errors.value.preferred_end = 'Required.'; ok = false }
  if (form.value.preferred_start && form.value.preferred_end &&
      form.value.preferred_end < form.value.preferred_start) {
    errors.value.preferred_end = 'End must be after start.'
    ok = false
  }
  if (fileErrors.value.length > 0) ok = false
  return ok
}

async function handleSubmit() {
  if (!validate()) return
  submitting.value = true
  submitError.value = ''
  try {
    const created = await ticket.createTicket({
      offering_id: Number(form.value.offering_id),
      category_id: Number(form.value.category_id),
      address_id: Number(form.value.address_id),
      preferred_start: form.value.preferred_start,
      preferred_end: form.value.preferred_end,
      delivery_method: form.value.delivery_method,
    }, selectedFiles.value)
    router.push(`/tickets/${created.id}`)
  } catch (err) {
    submitError.value = err.response?.data?.error?.message ?? 'Could not create ticket. Please try again.'
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.ticket-create-view { max-width: 620px; }
h1 { margin-bottom: 1.4rem; }

.ticket-form { display: flex; flex-direction: column; gap: .9rem; }

.field label {
  display: block;
  font-size: .85rem;
  font-weight: 600;
  color: #374151;
  margin-bottom: .25rem;
}

.field input,
.field select {
  width: 100%;
  padding: .45rem .65rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  background: #fff;
  box-sizing: border-box;
}

.field.error input, .field.error select { border-color: #dc2626; }
.error-msg { display: block; color: #dc2626; font-size: .8rem; margin-top: .2rem; }
.required { color: #dc2626; }

.two-col { display: grid; grid-template-columns: 1fr 1fr; gap: .75rem; }

.radio-row { display: flex; gap: 1rem; padding: .35rem 0; }
.radio-row label { font-weight: 400; font-size: .9rem; cursor: pointer; }

.file-list { display: flex; flex-wrap: wrap; gap: .35rem; margin-top: .4rem; }
.file-chip {
  background: #e5e7eb;
  padding: .15rem .5rem;
  border-radius: 4px;
  font-size: .78rem;
  color: #374151;
}

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
}
.btn-submit:hover:not(:disabled) { background: #4338ca; }
.btn-submit:disabled { opacity: .55; cursor: not-allowed; }
</style>
