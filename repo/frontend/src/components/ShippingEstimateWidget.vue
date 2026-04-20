<template>
  <div class="shipping-widget">
    <h3>Shipping Estimate</h3>

    <div class="field">
      <label for="sw-region">Region</label>
      <select
        id="sw-region"
        v-model="selectedRegionId"
        data-testid="select-region"
      >
        <option value="0" disabled>Select a region…</option>
        <option v-for="r in shipping.regions" :key="r.id" :value="r.id">
          {{ r.name }}
        </option>
      </select>
    </div>

    <div class="field">
      <label>Delivery Method</label>
      <div class="radio-group">
        <label class="radio-label">
          <input
            type="radio"
            value="pickup"
            v-model="deliveryMethod"
            data-testid="radio-pickup"
          />
          Pickup (free)
        </label>
        <label class="radio-label">
          <input
            type="radio"
            value="courier"
            v-model="deliveryMethod"
            data-testid="radio-courier"
          />
          Courier
        </label>
      </div>
    </div>

    <template v-if="deliveryMethod === 'courier'">
      <div class="field">
        <label for="sw-weight">Weight (kg)</label>
        <input
          id="sw-weight"
          type="number"
          min="0"
          step="0.1"
          v-model.number="weightKg"
          data-testid="input-weight"
          placeholder="0.0"
        />
      </div>
      <div class="field">
        <label for="sw-qty">Quantity</label>
        <input
          id="sw-qty"
          type="number"
          min="1"
          v-model.number="quantity"
          data-testid="input-quantity"
          placeholder="1"
        />
      </div>
    </template>

    <button
      class="btn-estimate"
      data-testid="btn-estimate"
      :disabled="selectedRegionId === 0 || estimating"
      @click="runEstimate"
    >
      {{ estimating ? 'Calculating…' : 'Get Estimate' }}
    </button>

    <div v-if="estimateError" class="estimate-error">{{ estimateError }}</div>

    <div v-if="result" class="estimate-result">
      <div class="result-row">
        <span class="result-label">Fee</span>
        <span class="result-value" data-testid="estimate-fee">
          {{ result.fee === 0 ? 'Free' : `$${result.fee.toFixed(2)}` }}
        </span>
      </div>
      <div v-if="result.estimated_arrival_window" class="result-row">
        <span class="result-label">Estimated Arrival</span>
        <span class="result-value" data-testid="estimate-window">
          {{ result.estimated_arrival_window }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useShippingStore } from '@/stores/shipping'

const shipping = useShippingStore()

const selectedRegionId = ref(0)
const deliveryMethod = ref('pickup')
const weightKg = ref(0)
const quantity = ref(1)
const estimating = ref(false)
const estimateError = ref('')
const result = ref(null)

onMounted(async () => {
  await shipping.fetchRegions()
  if (shipping.regions.length > 0) {
    selectedRegionId.value = shipping.regions[0].id
  }
})

async function runEstimate() {
  estimateError.value = ''
  result.value = null
  estimating.value = true
  try {
    const payload = {
      region_id: selectedRegionId.value,
      delivery_method: deliveryMethod.value,
      weight_kg: deliveryMethod.value === 'courier' ? weightKg.value : 0,
      quantity: deliveryMethod.value === 'courier' ? (quantity.value || 1) : 1,
    }
    result.value = await shipping.getEstimate(payload)
  } catch (err) {
    const code = err.response?.data?.error?.code
    if (code === 'no_template') {
      estimateError.value = 'No shipping option found for these parameters.'
    } else if (err.response?.status === 404) {
      estimateError.value = 'Selected region was not found.'
    } else {
      estimateError.value = 'Could not get estimate. Please try again.'
    }
  } finally {
    estimating.value = false
  }
}
</script>

<style scoped>
.shipping-widget {
  background: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 1.25rem;
  margin-top: 1.5rem;
}

h3 { margin: 0 0 1rem; font-size: 1rem; color: #111827; }

.field { margin-bottom: .85rem; }

.field label {
  display: block;
  font-size: .85rem;
  font-weight: 600;
  color: #374151;
  margin-bottom: .25rem;
}

.field select,
.field input {
  width: 100%;
  padding: .4rem .6rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  background: #fff;
  box-sizing: border-box;
}

.radio-group { display: flex; gap: 1.25rem; }
.radio-label { font-size: .9rem; display: flex; align-items: center; gap: .35rem; cursor: pointer; }

.btn-estimate {
  width: 100%;
  background: #4f46e5;
  color: #fff;
  border: none;
  border-radius: 8px;
  padding: .5rem;
  font-size: .9rem;
  cursor: pointer;
  margin-top: .25rem;
  transition: background .15s;
}
.btn-estimate:hover:not(:disabled) { background: #4338ca; }
.btn-estimate:disabled { opacity: .55; cursor: not-allowed; }

.estimate-error { color: #dc2626; font-size: .85rem; margin-top: .6rem; }

.estimate-result {
  margin-top: .85rem;
  padding: .75rem;
  background: #ecfdf5;
  border: 1px solid #6ee7b7;
  border-radius: 8px;
}

.result-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: .9rem;
  padding: .2rem 0;
}

.result-label { color: #374151; }
.result-value { font-weight: 600; color: #065f46; }
</style>
