<template>
  <div v-if="summary" class="review-summary-widget">
    <div class="row">
      <span class="label">Reviews</span>
      <span class="value" data-testid="review-summary-total">{{ summary.total_reviews }}</span>
    </div>
    <div class="row">
      <span class="label">Average</span>
      <span class="value" data-testid="review-summary-avg">
        <template v-if="summary.total_reviews > 0">
          {{ summary.average_rating.toFixed(1) }} / 5
        </template>
        <template v-else>—</template>
      </span>
    </div>
    <div class="row">
      <span class="label">Positive</span>
      <span class="value" data-testid="review-summary-positive-rate">
        {{ Math.round(summary.positive_rate * 100) }}%
      </span>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted } from 'vue'
import { useReviewStore } from '@/stores/review'

const props = defineProps({
  offeringId: { type: Number, required: true },
})

const reviewStore = useReviewStore()
const summary = ref(null)

async function load() {
  summary.value = await reviewStore.fetchSummary(props.offeringId)
}

onMounted(load)
watch(() => props.offeringId, load)
</script>

<style scoped>
.review-summary-widget {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: .75rem 1rem;
  display: flex;
  gap: 1.5rem;
  font-size: .85rem;
}

.row { display: flex; flex-direction: column; gap: .1rem; }
.label { color: #6b7280; font-size: .78rem; text-transform: uppercase; letter-spacing: .02em; }
.value { color: #111827; font-weight: 700; font-size: 1rem; }
</style>
