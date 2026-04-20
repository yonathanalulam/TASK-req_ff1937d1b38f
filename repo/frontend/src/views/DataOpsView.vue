<template>
  <div class="dataops-view">
    <h1>Data Operations</h1>
    <p class="subtitle">
      Manage ingestion sources, run pipeline jobs, and inspect the
      Bronze / Silver / Gold lakehouse catalog.
    </p>

    <!-- Sources -->
    <section class="card">
      <h2>Sources</h2>
      <div v-if="loadingSources" class="loading">Loading…</div>
      <div v-else-if="store.sources.length === 0" class="empty" data-testid="sources-empty">
        No sources registered.
      </div>
      <table v-else class="grid" data-testid="source-list">
        <thead>
          <tr><th>ID</th><th>Name</th><th>Type</th><th>Active</th><th>Actions</th></tr>
        </thead>
        <tbody>
          <tr v-for="s in store.sources" :key="s.id">
            <td>{{ s.id }}</td>
            <td>{{ s.name }}</td>
            <td>{{ s.source_type }}</td>
            <td>{{ s.is_active ? 'yes' : 'no' }}</td>
            <td>
              <button
                class="btn-mini"
                :data-testid="`btn-create-job-${s.id}`"
                :disabled="busy"
                @click="onCreateJob(s.id)"
              >New Job</button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- Jobs -->
    <section class="card">
      <h2>Jobs</h2>
      <div v-if="loadingJobs" class="loading">Loading…</div>
      <div v-else-if="store.jobs.length === 0" class="empty" data-testid="jobs-empty">
        No jobs yet.
      </div>
      <table v-else class="grid" data-testid="job-list">
        <thead>
          <tr>
            <th>ID</th><th>Source</th><th>Status</th>
            <th>Rows</th><th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="j in store.jobs" :key="j.id">
            <td>{{ j.id }}</td>
            <td>{{ j.source_id }}</td>
            <td :class="['status', `status-${j.status}`]">{{ j.status }}</td>
            <td>{{ j.rows_ingested }}/{{ j.rows_expected }}</td>
            <td>
              <button
                class="btn-mini"
                :data-testid="`btn-run-job-${j.id}`"
                :disabled="busy || j.status === 'running' || j.status === 'completed'"
                @click="onRunJob(j.id)"
              >Run</button>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-if="store.lastRun" class="last-run" data-testid="last-run">
        Last run: job {{ store.lastRun.job_id }} — {{ store.lastRun.status }}
        <span v-if="store.lastRun.bronze_id">
          · Bronze #{{ store.lastRun.bronze_id }}
        </span>
        <span v-if="store.lastRun.silver_id">
          · Silver #{{ store.lastRun.silver_id }}
        </span>
        <span v-if="store.lastRun.gold_id">
          · Gold #{{ store.lastRun.gold_id }}
        </span>
      </div>
    </section>

    <!-- Catalog -->
    <section class="card">
      <h2>Lakehouse catalog</h2>
      <div v-if="loadingCatalog" class="loading">Loading…</div>
      <div v-else-if="store.catalog.length === 0" class="empty" data-testid="catalog-empty">
        No lakehouse artifacts.
      </div>
      <table v-else class="grid" data-testid="catalog-list">
        <thead>
          <tr>
            <th>ID</th><th>Source</th><th>Layer</th>
            <th>Rows</th><th>Ingested</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in store.catalog" :key="m.id">
            <td>{{ m.id }}</td>
            <td>{{ m.source_id }}</td>
            <td>{{ m.layer }}</td>
            <td>{{ m.row_count }}</td>
            <td>{{ formatTime(m.ingested_at) }}</td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useDataOpsStore } from '@/stores/dataops'

const store = useDataOpsStore()
const loadingSources = ref(true)
const loadingJobs = ref(true)
const loadingCatalog = ref(true)
const busy = ref(false)

async function refreshAll() {
  loadingSources.value = true
  loadingJobs.value = true
  loadingCatalog.value = true
  try { await store.fetchSources() } catch (e) { console.error(e) }
  loadingSources.value = false
  try { await store.fetchJobs() } catch (e) { console.error(e) }
  loadingJobs.value = false
  try { await store.fetchCatalog() } catch (e) { console.error(e) }
  loadingCatalog.value = false
}

onMounted(refreshAll)

async function onCreateJob(sourceId) {
  busy.value = true
  try {
    await store.createJob(sourceId)
  } catch (e) {
    console.error('create job failed', e)
  } finally {
    busy.value = false
  }
}

async function onRunJob(jobId) {
  busy.value = true
  try {
    await store.runJob(jobId)
    await store.fetchCatalog()
  } catch (e) {
    console.error('run job failed', e)
  } finally {
    busy.value = false
  }
}

function formatTime(iso) {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}
</script>

<style scoped>
.dataops-view { max-width: 960px; }
h1 { margin: 0 0 .3rem; }
.subtitle { color: #6b7280; font-size: .9rem; margin: 0 0 1.25rem; }
.card {
  background: #fff; border: 1px solid #e5e7eb; border-radius: 12px;
  padding: 1rem 1.2rem; margin-bottom: 1rem;
}
h2 { margin: 0 0 .55rem; font-size: 1rem; color: #111827; }
.loading, .empty { color: #6b7280; padding: 1rem 0; text-align: center; }
.grid { width: 100%; border-collapse: collapse; font-size: .88rem; }
.grid th, .grid td { text-align: left; padding: .4rem .5rem; border-bottom: 1px solid #f3f4f6; }
.grid th { color: #374151; font-weight: 600; }
.status { font-weight: 600; text-transform: capitalize; }
.status-completed { color: #15803d; }
.status-failed { color: #b91c1c; }
.status-running { color: #1d4ed8; }
.status-pending { color: #6b7280; }
.btn-mini {
  background: #4f46e5; color: #fff; border: none;
  border-radius: 4px; padding: .2rem .6rem;
  font-size: .75rem; cursor: pointer;
}
.btn-mini:disabled { opacity: .5; cursor: not-allowed; }
.last-run { margin-top: .75rem; font-size: .82rem; color: #374151; }
</style>
