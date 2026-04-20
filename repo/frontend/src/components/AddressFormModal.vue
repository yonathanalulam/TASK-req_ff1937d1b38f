<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div class="modal" role="dialog" aria-modal="true">
      <header class="modal-header">
        <h2>{{ isEdit ? 'Edit Address' : 'Add Address' }}</h2>
        <button class="btn-close" @click="$emit('close')" aria-label="Close">&times;</button>
      </header>

      <form class="modal-body" @submit.prevent="handleSubmit" novalidate>

        <div class="field">
          <label>Label</label>
          <input v-model="form.label" data-testid="input-label" placeholder="Home, Work, …" />
        </div>

        <div class="field" :class="{ error: errors.address_line1 }">
          <label>Address Line 1 <span class="required">*</span></label>
          <input
            v-model="form.address_line1"
            data-testid="input-line1"
            placeholder="123 Main St"
            @blur="validateField('address_line1')"
          />
          <span v-if="errors.address_line1" class="error-msg">{{ errors.address_line1 }}</span>
        </div>

        <div class="field">
          <label>Address Line 2</label>
          <input v-model="form.address_line2" data-testid="input-line2" placeholder="Apt, Suite…" />
        </div>

        <div class="form-row">
          <div class="field" :class="{ error: errors.city }">
            <label>City <span class="required">*</span></label>
            <input
              v-model="form.city"
              data-testid="input-city"
              @blur="validateField('city')"
            />
            <span v-if="errors.city" class="error-msg">{{ errors.city }}</span>
          </div>

          <div class="field" :class="{ error: errors.state }">
            <label>State <span class="required">*</span></label>
            <input
              v-model="form.state"
              data-testid="input-state"
              maxlength="2"
              placeholder="CA"
              style="width: 5rem;"
              @blur="validateField('state')"
            />
            <span v-if="errors.state" class="error-msg">{{ errors.state }}</span>
          </div>
        </div>

        <div class="field" :class="{ error: errors.zip }">
          <label>ZIP <span class="required">*</span></label>
          <input
            v-model="form.zip"
            data-testid="input-zip"
            placeholder="12345 or 12345-6789"
            @blur="validateField('zip')"
          />
          <span v-if="errors.zip" class="error-msg">{{ errors.zip }}</span>
        </div>

        <div v-if="apiError" class="api-error" data-testid="addr-api-error">{{ apiError }}</div>

        <footer class="modal-footer">
          <button type="button" class="btn-secondary" @click="$emit('close')">Cancel</button>
          <button type="submit" class="btn-primary" data-testid="btn-save-address" :disabled="saving">
            {{ saving ? 'Saving…' : isEdit ? 'Update' : 'Add Address' }}
          </button>
        </footer>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { useAddressStore } from '@/stores/address'
import { useToast } from '@/composables/useToast'

const props = defineProps({
  address: { type: Object, default: null }, // null = create mode
})
const emit = defineEmits(['close', 'saved'])

const addressStore = useAddressStore()
const { success, error: toastError } = useToast()

const isEdit = computed(() => props.address !== null)
const saving = ref(false)
const apiError = ref('')

const form = ref({
  label: '',
  address_line1: '',
  address_line2: '',
  city: '',
  state: '',
  zip: '',
})

const errors = ref({
  address_line1: '',
  city: '',
  state: '',
  zip: '',
})

// Populate form when editing
watch(
  () => props.address,
  (addr) => {
    if (addr) {
      form.value = {
        label: addr.label || '',
        address_line1: addr.address_line1 || '',
        address_line2: addr.address_line2 || '',
        city: addr.city || '',
        state: addr.state || '',
        zip: addr.zip || '',
      }
    } else {
      form.value = { label: '', address_line1: '', address_line2: '', city: '', state: '', zip: '' }
    }
    errors.value = { address_line1: '', city: '', state: '', zip: '' }
    apiError.value = ''
  },
  { immediate: true }
)

const ZIP_RE = /^\d{5}(-\d{4})?$/

function validateField(field) {
  switch (field) {
    case 'address_line1':
      errors.value.address_line1 = form.value.address_line1.trim() ? '' : 'Address line 1 is required.'
      break
    case 'city':
      errors.value.city = form.value.city.trim() ? '' : 'City is required.'
      break
    case 'state':
      errors.value.state = form.value.state.trim().length === 2 ? '' : 'Enter a 2-letter state code.'
      break
    case 'zip':
      errors.value.zip = ZIP_RE.test(form.value.zip) ? '' : 'Enter a valid ZIP (NNNNN or NNNNN-NNNN).'
      break
  }
}

function validateAll() {
  ;['address_line1', 'city', 'state', 'zip'].forEach(validateField)
  return !Object.values(errors.value).some(Boolean)
}

async function handleSubmit() {
  apiError.value = ''
  if (!validateAll()) return

  saving.value = true
  try {
    const payload = {
      label: form.value.label || 'Home',
      address_line1: form.value.address_line1.trim(),
      address_line2: form.value.address_line2.trim(),
      city: form.value.city.trim(),
      state: form.value.state.trim().toUpperCase(),
      zip: form.value.zip.trim(),
    }

    if (isEdit.value) {
      await addressStore.updateAddress(props.address.id, payload)
      success('Address updated.')
    } else {
      await addressStore.createAddress(payload)
      success('Address added.')
    }
    emit('saved')
    emit('close')
  } catch (err) {
    const msg = err.response?.data?.error?.message || 'Failed to save address.'
    apiError.value = msg
    toastError(msg)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 200;
}

.modal {
  background: #fff;
  border-radius: 10px;
  width: 100%;
  max-width: 480px;
  box-shadow: 0 20px 60px rgba(0,0,0,.2);
  overflow: hidden;
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid #e5e7eb;
}
.modal-header h2 { margin: 0; font-size: 1.05rem; color: #111827; }

.btn-close {
  background: none;
  border: none;
  font-size: 1.4rem;
  color: #6b7280;
  cursor: pointer;
  line-height: 1;
}
.btn-close:hover { color: #111827; }

.modal-body { padding: 1.25rem; display: flex; flex-direction: column; gap: .85rem; }

.form-row { display: flex; gap: .75rem; }
.form-row .field { flex: 1; }

.field { display: flex; flex-direction: column; gap: .25rem; }

label { font-size: .82rem; font-weight: 600; color: #374151; }

input {
  padding: .45rem .7rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .92rem;
  outline: none;
  transition: border-color .15s;
}
input:focus { border-color: #6366f1; }

.field.error input { border-color: #ef4444; }
.error-msg { font-size: .78rem; color: #ef4444; }

.api-error {
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #991b1b;
  border-radius: 6px;
  padding: .45rem .7rem;
  font-size: .82rem;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: .75rem;
  padding-top: .5rem;
}

.btn-secondary {
  background: #fff;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  padding: .5rem 1rem;
  font-size: .9rem;
  cursor: pointer;
  color: #374151;
  transition: background .15s;
}
.btn-secondary:hover { background: #f9fafb; }

.btn-primary {
  background: #6366f1;
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: .5rem 1.25rem;
  font-size: .9rem;
  cursor: pointer;
  transition: background .15s;
}
.btn-primary:hover:not(:disabled) { background: #4f46e5; }
.btn-primary:disabled { opacity: .6; cursor: default; }

.required { color: #ef4444; }
</style>
