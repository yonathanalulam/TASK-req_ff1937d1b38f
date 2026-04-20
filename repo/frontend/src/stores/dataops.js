import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

// Data Operations dashboard store.
//
// Surface split:
//   - Legal holds live under /api/v1/admin/legal-holds (administrator only).
//   - Ingestion sources/jobs + lakehouse catalog live under /api/v1/dataops/*
//     and are accessible to data_operator AND administrator via session/CSRF.
//   - The HMAC-signed /api/v1/internal/data/* surface remains for machine
//     clients; the browser does not use it.
export const useDataOpsStore = defineStore('dataops', () => {
  // ── Legal holds (admin) ───────────────────────────────────────────────────
  const holds = ref([])

  async function fetchHolds() {
    const { data } = await axios.get('/api/v1/admin/legal-holds')
    holds.value = data.holds
    return data.holds
  }

  async function placeHold({ source_id, job_id, reason }) {
    const payload = { reason }
    if (source_id) payload.source_id = source_id
    if (job_id) payload.job_id = job_id
    const { data } = await axios.post('/api/v1/admin/legal-holds', payload)
    await fetchHolds()
    return data.hold
  }

  async function releaseHold(id) {
    await axios.delete(`/api/v1/admin/legal-holds/${id}`)
    holds.value = holds.value.filter((h) => h.id !== id)
  }

  // ── Sources + jobs + catalog (data_operator / administrator) ──────────────
  const sources = ref([])
  const jobs = ref([])
  const catalog = ref([])
  const lastRun = ref(null)

  async function fetchSources() {
    const { data } = await axios.get('/api/v1/dataops/sources')
    sources.value = data.sources ?? []
    return sources.value
  }

  async function fetchJobs(sourceId) {
    const params = sourceId ? { source_id: sourceId } : {}
    const { data } = await axios.get('/api/v1/dataops/jobs', { params })
    jobs.value = data.jobs ?? []
    return jobs.value
  }

  async function createJob(sourceId) {
    const { data } = await axios.post('/api/v1/dataops/jobs', { source_id: sourceId })
    await fetchJobs()
    return data.job
  }

  async function runJob(jobId) {
    const { data } = await axios.post(`/api/v1/dataops/jobs/${jobId}/run`)
    lastRun.value = data.run ?? null
    await fetchJobs()
    return data.run
  }

  async function fetchCatalog(sourceId) {
    const params = sourceId ? { source_id: sourceId } : {}
    const { data } = await axios.get('/api/v1/dataops/catalog', { params })
    catalog.value = data.items ?? []
    return catalog.value
  }

  async function runLifecycle() {
    const { data } = await axios.post('/api/v1/admin/lakehouse/lifecycle/run')
    return data.lifecycle
  }

  return {
    holds, fetchHolds, placeHold, releaseHold,
    sources, jobs, catalog, lastRun,
    fetchSources, fetchJobs, createJob, runJob, fetchCatalog,
    runLifecycle,
  }
})
