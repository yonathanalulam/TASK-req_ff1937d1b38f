<template>
  <div class="qa-thread-view">
    <button class="btn-back" @click="$router.back()">← Back</button>
    <h1>Questions &amp; Answers</h1>

    <div class="new-question" v-if="isRegularUser">
      <button class="btn-primary" data-testid="btn-ask-question" @click="showAsk = !showAsk">
        Ask a Question
      </button>
      <div v-if="showAsk" class="ask-form">
        <textarea
          v-model="newQuestion"
          rows="3"
          placeholder="Type your question…"
          data-testid="textarea-question"
        ></textarea>
        <button
          class="btn-submit"
          data-testid="btn-submit-question"
          :disabled="!newQuestion.trim()"
          @click="submitQuestion"
        >
          Submit
        </button>
      </div>
    </div>

    <div v-if="loading" class="loading">Loading…</div>
    <div v-else-if="threads.length === 0" class="empty">No questions yet.</div>
    <div v-else class="thread-list" data-testid="list-qa-threads">
      <div
        v-for="t in threads"
        :key="t.id"
        class="thread-item"
        data-testid="qa-thread-item"
      >
        <div class="question" data-testid="qa-thread-question">{{ t.question }}</div>
        <div class="question-meta">Asked {{ formatTime(t.created_at) }}</div>

        <div v-if="t.replies?.length" class="replies">
          <div
            v-for="p in t.replies"
            :key="p.id"
            class="reply"
            data-testid="qa-post-item"
          >
            <div class="reply-content">{{ p.content }}</div>
            <div class="reply-meta">
              <span>Answered {{ formatTime(p.created_at) }}</span>
              <button
                v-if="canDelete"
                class="btn-mini-danger"
                data-testid="btn-delete-post"
                @click="deletePost(p.id)"
              >
                Delete
              </button>
            </div>
          </div>
        </div>

        <div v-if="canReply" class="reply-action">
          <button
            v-if="replyingTo !== t.id"
            class="btn-secondary"
            data-testid="btn-reply"
            @click="replyingTo = t.id"
          >
            Reply
          </button>
          <div v-else class="reply-form">
            <textarea
              v-model="replyContent"
              rows="2"
              data-testid="textarea-reply"
            ></textarea>
            <div class="reply-actions">
              <button class="btn-cancel" @click="replyingTo = 0">Cancel</button>
              <button
                class="btn-submit"
                data-testid="btn-submit-reply"
                :disabled="!replyContent.trim()"
                @click="submitReply(t.id)"
              >
                Reply
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-if="nextCursor > 0" class="load-more">
      <button class="btn-secondary" data-testid="btn-load-more-qa" @click="loadMore">
        Load More
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useQAStore } from '@/stores/qa'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const qaStore = useQAStore()
const auth = useAuthStore()

const offeringId = Number(route.params.id)
const loading = ref(true)

const threads = computed(() => qaStore.threadsByOffering[offeringId]?.items ?? [])
const nextCursor = computed(() => qaStore.threadsByOffering[offeringId]?.next_cursor ?? 0)

const hasRole = (r) => auth.user?.roles?.includes(r) ?? false
const isRegularUser = computed(() => hasRole('regular_user') || hasRole('administrator'))
const canReply = computed(() => hasRole('service_agent') || hasRole('administrator'))
const canDelete = computed(() => hasRole('moderator') || hasRole('administrator'))

const showAsk = ref(false)
const newQuestion = ref('')
const replyingTo = ref(0)
const replyContent = ref('')

onMounted(async () => {
  await qaStore.fetchThreads(offeringId, 0)
  loading.value = false
})

async function loadMore() {
  await qaStore.fetchThreads(offeringId, nextCursor.value)
}

async function submitQuestion() {
  if (!newQuestion.value.trim()) return
  await qaStore.createThread(offeringId, newQuestion.value.trim())
  newQuestion.value = ''
  showAsk.value = false
}

async function submitReply(threadId) {
  if (!replyContent.value.trim()) return
  await qaStore.createReply(offeringId, threadId, replyContent.value.trim())
  replyContent.value = ''
  replyingTo.value = 0
}

async function deletePost(postId) {
  if (!window.confirm('Delete this post?')) return
  await qaStore.deletePost(postId, offeringId)
}

function formatTime(iso) {
  try { return new Date(iso).toLocaleDateString() } catch { return iso }
}
</script>

<style scoped>
.qa-thread-view { max-width: 720px; }

.btn-back {
  background: none; border: none; color: #4f46e5;
  font-size: .9rem; cursor: pointer; padding: 0;
  margin-bottom: .75rem;
}
.btn-back:hover { text-decoration: underline; }

h1 { margin: 0 0 1rem; }

.new-question { margin-bottom: 1.5rem; }
.ask-form {
  margin-top: .6rem;
  display: flex;
  flex-direction: column;
  gap: .4rem;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: .7rem;
}
.ask-form textarea, .reply-form textarea {
  width: 100%;
  padding: .4rem .6rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  font-family: inherit;
  box-sizing: border-box;
  resize: vertical;
}

.loading, .empty { color: #6b7280; text-align: center; padding: 2rem 0; }

.thread-list { display: flex; flex-direction: column; gap: .85rem; }

.thread-item {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: .8rem 1rem;
}

.question { font-weight: 600; color: #111827; }
.question-meta { font-size: .78rem; color: #6b7280; margin-top: .15rem; margin-bottom: .5rem; }

.replies { border-left: 3px solid #e5e7eb; padding-left: .7rem; margin: .5rem 0; display: flex; flex-direction: column; gap: .5rem; }
.reply { background: #f9fafb; border-radius: 6px; padding: .5rem .7rem; }
.reply-content { color: #111827; font-size: .9rem; }
.reply-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: .75rem;
  color: #6b7280;
  margin-top: .25rem;
}

.reply-action { margin-top: .5rem; }
.reply-form { display: flex; flex-direction: column; gap: .3rem; margin-top: .4rem; }
.reply-actions { display: flex; justify-content: flex-end; gap: .4rem; }

.btn-primary {
  background: #4f46e5; color: #fff; border: none;
  border-radius: 8px; padding: .4rem 1rem; font-size: .85rem; cursor: pointer;
}
.btn-primary:hover { background: #4338ca; }
.btn-secondary {
  background: #f3f4f6; color: #374151; border: 1px solid #d1d5db;
  border-radius: 8px; padding: .3rem .8rem; font-size: .8rem; cursor: pointer;
}
.btn-cancel {
  background: #f3f4f6; color: #374151; border: 1px solid #d1d5db;
  border-radius: 8px; padding: .3rem .8rem; font-size: .8rem; cursor: pointer;
}
.btn-submit {
  background: #4f46e5; color: #fff; border: none;
  border-radius: 8px; padding: .3rem .9rem; font-size: .8rem; cursor: pointer;
}
.btn-submit:disabled { opacity: .55; cursor: not-allowed; }

.btn-mini-danger {
  background: #fee2e2; color: #991b1b; border: none;
  border-radius: 4px; padding: .1rem .4rem; font-size: .7rem; cursor: pointer;
}

.load-more { text-align: center; margin-top: 1rem; }
</style>
