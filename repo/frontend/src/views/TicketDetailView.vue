<template>
  <div class="ticket-detail-view">
    <div v-if="loading" class="loading">Loading…</div>
    <div v-else-if="!ticket.currentTicket" class="not-found">Ticket not found.</div>
    <template v-else>
      <button class="btn-back" @click="$router.back()">← Back</button>

      <div class="detail-header">
        <h1>Ticket #{{ ticket.currentTicket.id }}</h1>
        <span class="status-badge" :class="statusClass" data-testid="ticket-detail-status">
          {{ ticket.currentTicket.status }}
        </span>
      </div>

      <!-- SLA block -->
      <div class="sla-block" :class="{ breached: ticket.currentTicket.sla_breached }">
        <div v-if="ticket.currentTicket.sla_deadline" class="sla-line" data-testid="ticket-sla-deadline">
          SLA deadline: <strong>{{ formatTime(ticket.currentTicket.sla_deadline) }}</strong>
          <span v-if="!ticket.currentTicket.sla_breached" class="sla-remaining">
            ({{ slaRemaining }})
          </span>
        </div>
        <div v-if="ticket.currentTicket.sla_breached" class="sla-breached-banner" data-testid="ticket-sla-breached">
          SLA has been breached
        </div>
      </div>

      <!-- Summary -->
      <div class="detail-meta">
        <div>Preferred: {{ formatTime(ticket.currentTicket.preferred_start) }} – {{ formatTime(ticket.currentTicket.preferred_end) }}</div>
        <div>Delivery: {{ ticket.currentTicket.delivery_method }}</div>
        <div v-if="ticket.currentTicket.shipping_fee > 0">
          Shipping fee: ${{ ticket.currentTicket.shipping_fee.toFixed(2) }}
        </div>
        <div v-if="ticket.currentTicket.cancel_reason">
          Cancel reason: {{ ticket.currentTicket.cancel_reason }}
        </div>
      </div>

      <!-- Transition buttons -->
      <div class="transition-bar">
        <button
          v-if="canAgentDispatch"
          class="btn-secondary"
          data-testid="btn-transition-dispatched"
          @click="doTransition('Dispatched')"
        >
          Mark Dispatched
        </button>
        <button
          v-if="canAgentInService"
          class="btn-secondary"
          data-testid="btn-transition-inservice"
          @click="doTransition('In Service')"
        >
          Mark In Service
        </button>
        <button
          v-if="canAgentComplete"
          class="btn-secondary"
          data-testid="btn-transition-completed"
          @click="doTransition('Completed')"
        >
          Mark Completed
        </button>
        <button
          v-if="canAdminClose"
          class="btn-secondary"
          data-testid="btn-transition-closed"
          @click="doTransition('Closed')"
        >
          Close Ticket
        </button>
        <button
          v-if="canUserCancel"
          class="btn-danger"
          data-testid="btn-cancel-ticket"
          @click="onCancel"
        >
          Cancel Request
        </button>
        <button
          v-if="canWriteReview"
          class="btn-primary"
          data-testid="btn-write-review"
          @click="showReviewModal = true"
        >
          Write Review
        </button>
      </div>

      <div v-if="transitionError" class="form-error">{{ transitionError }}</div>

      <!-- Notes -->
      <section class="notes-section">
        <h2>Notes</h2>
        <div v-if="ticket.notes.length === 0" class="empty-sm">No notes yet.</div>
        <div v-else class="note-list">
          <div v-for="n in ticket.notes" :key="n.id" class="note-item" data-testid="note-item">
            <div class="note-meta">#{{ n.author_id }} · {{ formatTime(n.created_at) }}</div>
            <div class="note-content">{{ n.content }}</div>
          </div>
        </div>
        <div class="note-form">
          <textarea
            v-model="newNote"
            placeholder="Add a note…"
            data-testid="textarea-note"
            rows="2"
          ></textarea>
          <button
            class="btn-secondary"
            data-testid="btn-add-note"
            :disabled="!newNote.trim()"
            @click="addNote"
          >
            Add Note
          </button>
        </div>
      </section>

      <!-- Attachments -->
      <section class="attachments-section">
        <h2>Attachments</h2>
        <div v-if="ticket.attachments.length === 0" class="empty-sm">No files attached.</div>
        <div v-else class="attachment-list">
          <div
            v-for="a in ticket.attachments"
            :key="a.id"
            class="attachment-item"
            data-testid="attachment-item"
          >
            <span class="att-name">{{ a.original_name }}</span>
            <span class="att-size">{{ (a.size_bytes / 1024).toFixed(0) }} KB</span>
            <button
              v-if="canDeleteAttachments"
              class="btn-mini-danger"
              data-testid="btn-delete-attachment"
              @click="onDeleteAttachment(a.id)"
            >
              Delete
            </button>
          </div>
        </div>
      </section>

      <!-- Review modal -->
      <ReviewFormModal
        v-if="showReviewModal"
        :ticket-id="ticket.currentTicket.id"
        @close="showReviewModal = false"
        @submitted="onReviewSubmitted"
      />
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useTicketStore } from '@/stores/ticket'
import { useAuthStore } from '@/stores/auth'
import ReviewFormModal from '@/components/ReviewFormModal.vue'

const route = useRoute()
const ticket = useTicketStore()
const auth = useAuthStore()

const loading = ref(true)
const newNote = ref('')
const transitionError = ref('')
const showReviewModal = ref(false)

// Force re-render of slaRemaining every 30s
const now = ref(Date.now())
let clockInterval = null

onMounted(async () => {
  const id = Number(route.params.id)
  try {
    await ticket.fetchTicket(id)
    await Promise.all([
      ticket.fetchNotes(id),
      ticket.fetchAttachments(id),
    ])
  } catch {
    ticket.currentTicket = null
  }
  loading.value = false
  clockInterval = setInterval(() => { now.value = Date.now() }, 30_000)
})

onUnmounted(() => {
  if (clockInterval) clearInterval(clockInterval)
})

const hasRole = (r) => auth.user?.roles?.includes(r) ?? false

const canAgentDispatch  = computed(() => hasRole('service_agent') && ticket.currentTicket?.status === 'Accepted')
const canAgentInService = computed(() => hasRole('service_agent') && ticket.currentTicket?.status === 'Dispatched')
const canAgentComplete  = computed(() => hasRole('service_agent') && ticket.currentTicket?.status === 'In Service')
const canAdminClose     = computed(() => hasRole('administrator') &&
  ticket.currentTicket && !['Closed', 'Cancelled'].includes(ticket.currentTicket.status))
const canUserCancel     = computed(() => {
  if (!ticket.currentTicket) return false
  return ticket.currentTicket.user_id === auth.user?.id &&
    ticket.currentTicket.status === 'Accepted'
})
const canWriteReview = computed(() => {
  if (!ticket.currentTicket) return false
  if (ticket.currentTicket.user_id !== auth.user?.id) return false
  return ['Completed', 'Closed'].includes(ticket.currentTicket.status)
})
const canDeleteAttachments = computed(() => {
  if (!ticket.currentTicket) return false
  return hasRole('administrator') || ticket.currentTicket.user_id === auth.user?.id
})

const statusClass = computed(() => {
  const s = ticket.currentTicket?.status ?? ''
  return {
    accepted: s === 'Accepted',
    dispatched: s === 'Dispatched',
    'in-service': s === 'In Service',
    completed: s === 'Completed',
    closed: s === 'Closed',
    cancelled: s === 'Cancelled',
  }
})

const slaRemaining = computed(() => {
  const d = ticket.currentTicket?.sla_deadline
  if (!d) return ''
  const diff = new Date(d).getTime() - now.value
  if (diff <= 0) return 'overdue'
  const mins = Math.floor(diff / 60000)
  if (mins < 60) return `${mins}m remaining`
  const h = Math.floor(mins / 60)
  const m = mins % 60
  return `${h}h ${m}m remaining`
})

async function doTransition(status) {
  transitionError.value = ''
  try {
    await ticket.updateStatus(ticket.currentTicket.id, status)
  } catch (err) {
    transitionError.value = err.response?.data?.error?.message ?? 'Transition failed.'
  }
}

async function onCancel() {
  const reason = window.prompt('Why are you cancelling?') ?? ''
  transitionError.value = ''
  try {
    await ticket.updateStatus(ticket.currentTicket.id, 'Cancelled', reason)
  } catch (err) {
    transitionError.value = err.response?.data?.error?.message ?? 'Cancel failed.'
  }
}

async function addNote() {
  const content = newNote.value.trim()
  if (!content) return
  try {
    await ticket.createNote(ticket.currentTicket.id, content)
    newNote.value = ''
  } catch (err) {
    transitionError.value = err.response?.data?.error?.message ?? 'Could not add note.'
  }
}

async function onDeleteAttachment(fileId) {
  if (!window.confirm('Delete this attachment?')) return
  try {
    await ticket.deleteAttachment(ticket.currentTicket.id, fileId)
  } catch (err) {
    transitionError.value = err.response?.data?.error?.message ?? 'Delete failed.'
  }
}

function onReviewSubmitted() {
  showReviewModal.value = false
}

function formatTime(iso) {
  if (!iso) return ''
  try { return new Date(iso).toLocaleString() } catch { return iso }
}
</script>

<style scoped>
.ticket-detail-view { max-width: 720px; }

.loading, .not-found { color: #6b7280; text-align: center; padding: 3rem 0; }

.btn-back {
  background: none;
  border: none;
  color: #4f46e5;
  font-size: .9rem;
  cursor: pointer;
  padding: 0;
  margin-bottom: .75rem;
}
.btn-back:hover { text-decoration: underline; }

.detail-header {
  display: flex;
  align-items: center;
  gap: .75rem;
  margin-bottom: .6rem;
}

h1 { margin: 0; font-size: 1.5rem; font-weight: 700; color: #111827; }
h2 { font-size: 1.05rem; margin: 1.4rem 0 .5rem; color: #111827; }

.status-badge {
  padding: .15rem .6rem;
  border-radius: 4px;
  font-size: .8rem;
  font-weight: 600;
  background: #e5e7eb;
  color: #374151;
}
.status-badge.accepted   { background: #dbeafe; color: #1d4ed8; }
.status-badge.dispatched { background: #fef3c7; color: #92400e; }
.status-badge.in-service { background: #fde68a; color: #78350f; }
.status-badge.completed  { background: #d1fae5; color: #065f46; }
.status-badge.closed     { background: #e5e7eb; color: #374151; }
.status-badge.cancelled  { background: #fee2e2; color: #991b1b; }

.sla-block {
  background: #f3f4f6;
  border-radius: 8px;
  padding: .6rem .9rem;
  font-size: .85rem;
  margin-bottom: .9rem;
}
.sla-block.breached { background: #fef2f2; border: 1px solid #fca5a5; color: #991b1b; }
.sla-line { color: #374151; }
.sla-remaining { color: #6b7280; margin-left: .35rem; }
.sla-breached-banner { font-weight: 700; margin-top: .3rem; }

.detail-meta { color: #374151; font-size: .9rem; margin-bottom: 1rem; display: flex; flex-direction: column; gap: .2rem; }

.transition-bar { display: flex; flex-wrap: wrap; gap: .5rem; margin-bottom: .5rem; }

.btn-primary, .btn-secondary, .btn-danger {
  border: none;
  border-radius: 8px;
  padding: .4rem .9rem;
  font-size: .85rem;
  cursor: pointer;
}
.btn-primary { background: #4f46e5; color: #fff; }
.btn-primary:hover { background: #4338ca; }
.btn-secondary { background: #f3f4f6; color: #374151; border: 1px solid #d1d5db; }
.btn-secondary:hover { background: #e5e7eb; }
.btn-danger { background: #fee2e2; color: #991b1b; border: 1px solid #fca5a5; }
.btn-danger:hover { background: #fecaca; }

.form-error {
  background: #fef2f2;
  border: 1px solid #fca5a5;
  border-radius: 6px;
  padding: .45rem .75rem;
  color: #b91c1c;
  font-size: .85rem;
  margin: .5rem 0;
}

.empty-sm { color: #9ca3af; font-size: .88rem; padding: .5rem 0; }

.note-list { display: flex; flex-direction: column; gap: .5rem; }
.note-item { background: #fff; border: 1px solid #e5e7eb; border-radius: 8px; padding: .55rem .8rem; }
.note-meta { font-size: .75rem; color: #6b7280; margin-bottom: .2rem; }
.note-content { font-size: .9rem; color: #111827; }

.note-form { display: flex; gap: .5rem; margin-top: .7rem; align-items: flex-start; }
.note-form textarea {
  flex: 1;
  padding: .4rem .6rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  font-family: inherit;
  resize: vertical;
  box-sizing: border-box;
}

.attachment-list { display: flex; flex-direction: column; gap: .35rem; }
.attachment-item {
  display: flex;
  align-items: center;
  gap: .6rem;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  padding: .4rem .7rem;
  font-size: .85rem;
}
.att-name { flex: 1; color: #111827; }
.att-size { color: #6b7280; font-size: .8rem; }

.btn-mini-danger {
  background: #fee2e2;
  color: #991b1b;
  border: none;
  border-radius: 4px;
  padding: .1rem .45rem;
  font-size: .75rem;
  cursor: pointer;
}
</style>
