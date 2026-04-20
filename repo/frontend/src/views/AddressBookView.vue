<template>
  <div class="address-book">
    <div class="page-header">
      <h1>Address Book</h1>
      <button class="btn-primary" data-testid="btn-add-address" @click="openCreate">
        + Add Address
      </button>
    </div>

    <div v-if="loading" class="loading">Loading…</div>

    <div v-else-if="addressStore.addresses.length === 0" class="empty-state" data-testid="empty-addresses">
      No saved addresses yet.
    </div>

    <ul v-else class="address-list">
      <li
        v-for="addr in addressStore.addresses"
        :key="addr.id"
        class="address-card"
        :class="{ 'is-default': addr.is_default }"
        :data-testid="`address-card-${addr.id}`"
      >
        <div class="card-header">
          <span class="label">{{ addr.label }}</span>
          <span v-if="addr.is_default" class="badge-default" data-testid="badge-default">Default</span>
        </div>

        <div class="card-body">
          <p>{{ addr.address_line1 }}</p>
          <p v-if="addr.address_line2">{{ addr.address_line2 }}</p>
          <p>{{ addr.city }}, {{ addr.state }} {{ addr.zip }}</p>
        </div>

        <div class="card-actions">
          <button
            v-if="!addr.is_default"
            class="btn-link"
            :data-testid="`btn-set-default-${addr.id}`"
            @click="handleSetDefault(addr.id)"
          >
            Set as Default
          </button>
          <button
            class="btn-link"
            :data-testid="`btn-edit-${addr.id}`"
            @click="openEdit(addr)"
          >
            Edit
          </button>
          <button
            class="btn-link btn-danger"
            :data-testid="`btn-delete-${addr.id}`"
            @click="handleDelete(addr.id)"
          >
            Delete
          </button>
        </div>
      </li>
    </ul>

    <!-- Address form modal -->
    <AddressFormModal
      v-if="showModal"
      :address="editingAddress"
      @close="showModal = false"
      @saved="onSaved"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useAddressStore } from '@/stores/address'
import { useToast } from '@/composables/useToast'
import AddressFormModal from '@/components/AddressFormModal.vue'

const addressStore = useAddressStore()
const { success, error: toastError } = useToast()

const loading = ref(true)
const showModal = ref(false)
const editingAddress = ref(null)

onMounted(async () => {
  try {
    await addressStore.fetchAddresses()
  } catch {
    toastError('Failed to load addresses.')
  } finally {
    loading.value = false
  }
})

function openCreate() {
  editingAddress.value = null
  showModal.value = true
}

function openEdit(addr) {
  editingAddress.value = addr
  showModal.value = true
}

function onSaved() {
  // Modal emits 'saved' then 'close'; store already refreshed by modal
}

async function handleSetDefault(id) {
  try {
    await addressStore.setDefault(id)
    success('Default address updated.')
  } catch {
    toastError('Failed to update default address.')
  }
}

async function handleDelete(id) {
  if (!confirm('Delete this address?')) return
  try {
    await addressStore.deleteAddress(id)
    success('Address deleted.')
  } catch {
    toastError('Failed to delete address.')
  }
}
</script>

<style scoped>
.address-book { max-width: 720px; }

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.5rem;
}

h1 { font-size: 1.4rem; color: #1a1a2e; margin: 0; }

.loading, .empty-state { color: #6b7280; }

.address-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: .75rem;
}

.address-card {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1rem 1.25rem;
  transition: border-color .15s, box-shadow .15s;
}
.address-card.is-default {
  border-color: #6366f1;
  box-shadow: 0 0 0 2px rgba(99,102,241,.15);
}

.card-header {
  display: flex;
  align-items: center;
  gap: .5rem;
  margin-bottom: .4rem;
}

.label { font-weight: 700; font-size: .95rem; color: #111827; }

.badge-default {
  background: #eef2ff;
  color: #4f46e5;
  font-size: .72rem;
  font-weight: 600;
  padding: .15rem .5rem;
  border-radius: 999px;
  border: 1px solid #c7d2fe;
}

.card-body p { margin: .15rem 0; font-size: .9rem; color: #374151; }

.card-actions {
  display: flex;
  gap: .75rem;
  margin-top: .75rem;
  padding-top: .75rem;
  border-top: 1px solid #f3f4f6;
}

.btn-link {
  background: none;
  border: none;
  color: #6366f1;
  font-size: .85rem;
  cursor: pointer;
  padding: 0;
  text-decoration: underline;
  text-underline-offset: 2px;
}
.btn-link:hover { color: #4f46e5; }
.btn-link.btn-danger { color: #ef4444; }
.btn-link.btn-danger:hover { color: #dc2626; }

.btn-primary {
  background: #6366f1;
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: .5rem 1rem;
  font-size: .9rem;
  cursor: pointer;
  transition: background .15s;
}
.btn-primary:hover { background: #4f46e5; }
</style>
