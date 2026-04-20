<template>
  <div class="review-list-view">
    <button class="btn-back" @click="$router.back()">← Back</button>
    <h1>Reviews</h1>

    <ReviewSummaryWidget :offering-id="offeringId" />

    <div v-if="loading" class="loading">Loading…</div>
    <div v-else-if="items.length === 0" class="empty">No reviews yet.</div>
    <div v-else class="review-list" data-testid="list-reviews">
      <div v-for="r in items" :key="r.id" class="review-item" data-testid="review-item">
        <div class="review-head">
          <span class="stars">{{ stars(r.rating) }}</span>
          <span class="rating" data-testid="review-rating">{{ r.rating }}/5</span>
          <span class="date">{{ formatTime(r.created_at) }}</span>
        </div>
        <div class="review-text">{{ r.text }}</div>
        <div v-if="r.images?.length" class="review-images">
          <span v-for="img in r.images" :key="img.id" class="img-chip">
            {{ img.filename }}
          </span>
        </div>
        <div class="review-actions">
          <button
            class="btn-report"
            data-testid="btn-report-review"
            @click="openReport(r.id)"
          >
            Report
          </button>
        </div>
      </div>
    </div>

    <div v-if="nextCursor > 0" class="load-more">
      <button class="btn-secondary" data-testid="btn-load-more-reviews" @click="loadMore">
        Load More
      </button>
    </div>

    <!-- Report dialog -->
    <div v-if="reportingID" class="modal-backdrop" @click.self="closeReport">
      <div class="modal">
        <h3>Report Review</h3>
        <div class="field">
          <label>Reason</label>
          <select v-model="reportReason" data-testid="select-report-reason">
            <option value="spam">Spam</option>
            <option value="abusive">Abusive</option>
            <option value="irrelevant">Irrelevant</option>
          </select>
        </div>
        <div class="field">
          <label>Details (optional)</label>
          <textarea v-model="reportDetails" rows="3" placeholder="Add context…"></textarea>
        </div>
        <div v-if="reportError" class="form-error">{{ reportError }}</div>
        <div class="actions">
          <button class="btn-cancel" @click="closeReport">Cancel</button>
          <button class="btn-submit" data-testid="btn-submit-report" @click="submitReport">
            Submit Report
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useReviewStore } from '@/stores/review'
import ReviewSummaryWidget from '@/components/ReviewSummaryWidget.vue'

const route = useRoute()
const reviewStore = useReviewStore()

const offeringId = Number(route.params.id)
const loading = ref(true)

const items = computed(() => reviewStore.reviewsByOffering[offeringId]?.items ?? [])
const nextCursor = computed(() => reviewStore.reviewsByOffering[offeringId]?.next_cursor ?? 0)

const reportingID = ref(0)
const reportReason = ref('spam')
const reportDetails = ref('')
const reportError = ref('')

onMounted(async () => {
  await reviewStore.fetchReviews(offeringId, 0)
  loading.value = false
})

async function loadMore() {
  await reviewStore.fetchReviews(offeringId, nextCursor.value)
}

function stars(n) {
  return '★'.repeat(n) + '☆'.repeat(5 - n)
}

function formatTime(iso) {
  try { return new Date(iso).toLocaleDateString() } catch { return iso }
}

function openReport(id) {
  reportingID.value = id
  reportReason.value = 'spam'
  reportDetails.value = ''
  reportError.value = ''
}
function closeReport() { reportingID.value = 0 }

async function submitReport() {
  reportError.value = ''
  try {
    await reviewStore.reportReview(reportingID.value, reportReason.value, reportDetails.value)
    closeReport()
  } catch (err) {
    if (err.response?.status === 429) {
      reportError.value = 'Too many reports. Please try again later.'
    } else {
      reportError.value = err.response?.data?.error?.message ?? 'Could not submit report.'
    }
  }
}
</script>

<style scoped>
.review-list-view { max-width: 720px; }

.btn-back {
  background: none; border: none; color: #4f46e5;
  font-size: .9rem; cursor: pointer; padding: 0;
  margin-bottom: .75rem;
}
.btn-back:hover { text-decoration: underline; }

h1 { margin: 0 0 1rem; }

.loading, .empty { color: #6b7280; text-align: center; padding: 2rem 0; }

.review-list { display: flex; flex-direction: column; gap: .7rem; margin-top: 1rem; }

.review-item {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: .8rem 1rem;
}

.review-head { display: flex; align-items: center; gap: .6rem; margin-bottom: .4rem; }
.stars { color: #f59e0b; letter-spacing: -.5px; }
.rating { font-size: .85rem; font-weight: 600; color: #111827; }
.date { font-size: .8rem; color: #6b7280; margin-left: auto; }

.review-text { color: #111827; line-height: 1.5; font-size: .9rem; }

.review-images { display: flex; gap: .3rem; flex-wrap: wrap; margin-top: .4rem; }
.img-chip {
  background: #e5e7eb;
  padding: .15rem .5rem;
  border-radius: 4px;
  font-size: .75rem;
  color: #374151;
}

.review-actions { margin-top: .5rem; text-align: right; }
.btn-report {
  background: none;
  border: 1px solid #d1d5db;
  color: #6b7280;
  border-radius: 6px;
  padding: .2rem .6rem;
  font-size: .78rem;
  cursor: pointer;
}
.btn-report:hover { background: #f3f4f6; color: #991b1b; }

.load-more { text-align: center; margin-top: 1rem; }
.btn-secondary {
  background: #f3f4f6;
  color: #374151;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: .4rem .9rem;
  font-size: .85rem;
  cursor: pointer;
}

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
  width: 420px;
  max-width: 90vw;
}
h3 { margin: 0 0 .75rem; }

.field { margin-bottom: .7rem; }
.field label { display: block; font-size: .85rem; font-weight: 600; color: #374151; margin-bottom: .25rem; }
.field select, .field textarea, .field input {
  width: 100%;
  padding: .4rem .6rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  box-sizing: border-box;
  font-family: inherit;
}

.form-error {
  background: #fef2f2;
  border: 1px solid #fca5a5;
  border-radius: 6px;
  padding: .45rem .75rem;
  color: #b91c1c;
  font-size: .85rem;
}

.actions { display: flex; justify-content: flex-end; gap: .5rem; margin-top: .6rem; }
.btn-cancel {
  background: #f3f4f6; color: #374151; border: 1px solid #d1d5db;
  border-radius: 8px; padding: .4rem .9rem; font-size: .9rem; cursor: pointer;
}
.btn-submit {
  background: #4f46e5; color: #fff; border: none;
  border-radius: 8px; padding: .4rem 1rem; font-size: .9rem; cursor: pointer;
}
</style>
