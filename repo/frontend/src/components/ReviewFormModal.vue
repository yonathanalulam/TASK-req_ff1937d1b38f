<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div class="modal" data-testid="modal-review">
      <button class="btn-close" @click="$emit('close')">×</button>
      <h2>Write Review</h2>

      <div class="field">
        <label>Rating</label>
        <div class="star-row">
          <button
            v-for="n in 5"
            :key="n"
            type="button"
            class="star-btn"
            :class="{ filled: rating >= n }"
            :data-testid="`star-${n}`"
            @click="rating = n"
          >
            ★
          </button>
        </div>
      </div>

      <div class="field">
        <label for="rv-text">Your Review</label>
        <textarea
          id="rv-text"
          v-model="text"
          rows="4"
          placeholder="Tell others about your experience…"
          data-testid="textarea-review-text"
        ></textarea>
      </div>

      <div class="field">
        <label for="rv-images">Images (JPG/PNG, up to 3, 5 MB each)</label>
        <input
          id="rv-images"
          type="file"
          accept=".jpg,.jpeg,.png,image/jpeg,image/png"
          multiple
          data-testid="input-review-images"
          @change="onImages"
        />
        <div v-if="imageErrors.length > 0" class="error-msg">
          <div v-for="(e, i) in imageErrors" :key="i">{{ e }}</div>
        </div>
      </div>

      <div v-if="submitError" class="form-error" data-testid="review-error">{{ submitError }}</div>

      <div class="actions">
        <button class="btn-cancel" @click="$emit('close')">Cancel</button>
        <button
          class="btn-submit"
          data-testid="btn-submit-review"
          :disabled="submitting || rating === 0"
          @click="submit"
        >
          {{ submitting ? 'Saving…' : 'Submit Review' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useReviewStore } from '@/stores/review'

const props = defineProps({
  ticketId: { type: Number, required: true },
})
const emit = defineEmits(['close', 'submitted'])

const reviewStore = useReviewStore()

const rating = ref(0)
const text = ref('')
const selectedImages = ref([])
const imageErrors = ref([])
const submitting = ref(false)
const submitError = ref('')

const MAX_IMAGES = 3
const MAX_BYTES = 5 * 1024 * 1024
const ALLOWED = new Set(['image/jpeg', 'image/png'])

function onImages(e) {
  const files = Array.from(e.target.files ?? [])
  imageErrors.value = []
  if (files.length > MAX_IMAGES) {
    imageErrors.value.push(`At most ${MAX_IMAGES} images allowed`)
    selectedImages.value = []
    return
  }
  for (const f of files) {
    if (f.size > MAX_BYTES) imageErrors.value.push(`${f.name} exceeds 5 MB`)
    if (!ALLOWED.has(f.type)) imageErrors.value.push(`${f.name} is not JPG or PNG`)
  }
  if (imageErrors.value.length === 0) {
    selectedImages.value = files
  }
}

async function submit() {
  if (rating.value < 1) return
  submitting.value = true
  submitError.value = ''
  try {
    const r = await reviewStore.createReview(
      props.ticketId,
      { rating: rating.value, text: text.value.trim() },
      selectedImages.value,
    )
    emit('submitted', r)
  } catch (err) {
    const code = err.response?.data?.error?.code
    if (code === 'already_exists') {
      submitError.value = 'You have already reviewed this ticket.'
    } else if (code === 'ticket_not_eligible') {
      submitError.value = 'This ticket is not eligible for a review yet.'
    } else {
      submitError.value = err.response?.data?.error?.message ?? 'Could not submit review.'
    }
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.modal-backdrop {
  position: fixed; inset: 0;
  background: rgba(17, 24, 39, .5);
  display: flex; align-items: center; justify-content: center;
  z-index: 500;
}

.modal {
  background: #fff;
  border-radius: 12px;
  padding: 1.25rem 1.5rem;
  width: 460px;
  max-width: 90vw;
  max-height: 90vh;
  overflow-y: auto;
  position: relative;
  box-shadow: 0 10px 40px rgba(0,0,0,.25);
}

.btn-close {
  position: absolute; top: .5rem; right: .7rem;
  background: none; border: none;
  font-size: 1.5rem; cursor: pointer; color: #6b7280;
}
.btn-close:hover { color: #111827; }

h2 { margin: 0 0 1rem; font-size: 1.15rem; color: #111827; }

.field { margin-bottom: .9rem; }
.field label {
  display: block;
  font-size: .85rem;
  font-weight: 600;
  color: #374151;
  margin-bottom: .25rem;
}
.field textarea, .field input {
  width: 100%;
  padding: .45rem .65rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  box-sizing: border-box;
  font-family: inherit;
}

.star-row { display: flex; gap: .2rem; }
.star-btn {
  background: none;
  border: none;
  font-size: 1.6rem;
  color: #d1d5db;
  cursor: pointer;
  padding: 0 .1rem;
  line-height: 1;
  transition: color .1s;
}
.star-btn.filled { color: #f59e0b; }
.star-btn:hover { color: #fbbf24; }

.error-msg { color: #dc2626; font-size: .8rem; margin-top: .2rem; }

.form-error {
  background: #fef2f2;
  border: 1px solid #fca5a5;
  border-radius: 6px;
  padding: .5rem .75rem;
  color: #b91c1c;
  font-size: .85rem;
  margin-bottom: .5rem;
}

.actions { display: flex; justify-content: flex-end; gap: .5rem; margin-top: .5rem; }
.btn-cancel {
  background: #f3f4f6;
  color: #374151;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: .45rem 1rem;
  font-size: .9rem;
  cursor: pointer;
}
.btn-submit {
  background: #4f46e5;
  color: #fff;
  border: none;
  border-radius: 8px;
  padding: .45rem 1.1rem;
  font-size: .9rem;
  cursor: pointer;
}
.btn-submit:hover:not(:disabled) { background: #4338ca; }
.btn-submit:disabled { opacity: .55; cursor: not-allowed; }
</style>
