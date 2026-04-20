<template>
  <div class="ticket-list-view">
    <div class="view-header">
      <h1>My Tickets</h1>
      <button
        class="btn-primary"
        data-testid="btn-new-ticket"
        @click="$router.push('/tickets/new')"
      >
        + New Ticket
      </button>
    </div>

    <div class="filters">
      <label for="filter-status">Status</label>
      <select
        id="filter-status"
        v-model="selectedStatus"
        data-testid="select-status-filter"
        @change="applyFilter"
      >
        <option value="">All</option>
        <option value="Accepted">Accepted</option>
        <option value="Dispatched">Dispatched</option>
        <option value="In Service">In Service</option>
        <option value="Completed">Completed</option>
        <option value="Closed">Closed</option>
        <option value="Cancelled">Cancelled</option>
      </select>
    </div>

    <div v-if="loading" class="loading">Loading tickets…</div>
    <div v-else-if="ticket.tickets.length === 0" class="empty" data-testid="empty-tickets">
      No tickets yet.
    </div>
    <div v-else class="ticket-list" data-testid="list-tickets">
      <div
        v-for="t in ticket.tickets"
        :key="t.id"
        class="ticket-row"
        data-testid="ticket-row"
        @click="$router.push(`/tickets/${t.id}`)"
      >
        <div class="row-main">
          <div class="ticket-id">#{{ t.id }}</div>
          <div class="ticket-meta">
            <span class="status-badge" :class="statusClass(t.status)" data-testid="ticket-status">
              {{ t.status }}
            </span>
            <span v-if="t.sla_breached" class="sla-breached" data-testid="ticket-sla-warning">
              SLA breached
            </span>
          </div>
        </div>
        <div class="row-sub">
          Preferred: {{ formatTime(t.preferred_start) }} – {{ formatTime(t.preferred_end) }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useTicketStore } from '@/stores/ticket'

const ticket = useTicketStore()
const loading = ref(false)
const selectedStatus = ref('')

onMounted(async () => {
  loading.value = true
  await ticket.fetchTickets()
  loading.value = false
})

async function applyFilter() {
  loading.value = true
  await ticket.fetchTickets(selectedStatus.value)
  loading.value = false
}

function formatTime(iso) {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

function statusClass(s) {
  return {
    accepted: s === 'Accepted',
    dispatched: s === 'Dispatched',
    'in-service': s === 'In Service',
    completed: s === 'Completed',
    closed: s === 'Closed',
    cancelled: s === 'Cancelled',
  }
}
</script>

<style scoped>
.ticket-list-view { max-width: 820px; }

.view-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1rem;
}

.filters { display: flex; gap: .5rem; margin-bottom: 1.25rem; align-items: center; }
.filters label { font-size: .85rem; font-weight: 600; color: #374151; }
.filters select {
  padding: .35rem .6rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  background: #fff;
}

.loading, .empty { color: #6b7280; text-align: center; padding: 3rem 0; }

.ticket-list { display: flex; flex-direction: column; gap: .6rem; }

.ticket-row {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: .9rem 1.1rem;
  cursor: pointer;
  transition: box-shadow .15s, transform .1s;
}
.ticket-row:hover { box-shadow: 0 3px 10px rgba(0,0,0,.08); transform: translateY(-1px); }

.row-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: .3rem;
}

.ticket-id { font-weight: 700; color: #111827; }
.ticket-meta { display: flex; align-items: center; gap: .5rem; }

.status-badge {
  padding: .15rem .55rem;
  border-radius: 4px;
  font-size: .78rem;
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

.sla-breached {
  background: #fee2e2;
  color: #991b1b;
  padding: .1rem .5rem;
  border-radius: 4px;
  font-size: .75rem;
  font-weight: 700;
}

.row-sub { font-size: .85rem; color: #6b7280; }

.btn-primary {
  background: #4f46e5;
  color: #fff;
  border: none;
  border-radius: 8px;
  padding: .5rem 1.1rem;
  font-size: .9rem;
  cursor: pointer;
}
.btn-primary:hover { background: #4338ca; }
</style>
