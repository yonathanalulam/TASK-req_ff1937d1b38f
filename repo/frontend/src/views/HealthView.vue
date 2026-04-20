<template>
  <div class="health-view">
    <h1>System Health</h1>

    <div v-if="loading" class="status-card loading">
      Checking…
    </div>

    <div v-else-if="error" class="status-card error">
      <strong>Error:</strong> {{ error }}
    </div>

    <div v-else class="status-card" :class="health.status === 'ok' ? 'ok' : 'degraded'">
      <p data-testid="health-status">Status: <strong>{{ health.status }}</strong></p>
      <p>Database: <strong>{{ health.database }}</strong></p>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import axios from 'axios'

const loading = ref(true)
const error = ref(null)
const health = ref({ status: '', database: '' })

onMounted(async () => {
  try {
    const { data } = await axios.get('/health')
    health.value = data
  } catch (e) {
    error.value = e.message || 'Unable to reach backend'
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.health-view {
  max-width: 480px;
}

.status-card {
  margin-top: 1rem;
  padding: 1.25rem 1.5rem;
  border-radius: 8px;
  border: 1px solid #ddd;
  font-size: 0.95rem;
  line-height: 1.6;
}

.ok       { background: #f0fdf4; border-color: #86efac; }
.degraded { background: #fef9c3; border-color: #fde047; }
.error    { background: #fef2f2; border-color: #fca5a5; }
.loading  { background: #f8fafc; color: #64748b; }
</style>
