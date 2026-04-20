import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const useCatalogStore = defineStore('catalog', () => {
  const categories = ref([])
  const offerings = ref([])
  const nextCursor = ref(0)
  const currentOffering = ref(null)

  // ── Categories ─────────────────────────────────────────────────────────────

  async function fetchCategories() {
    const { data } = await axios.get('/api/v1/service-categories')
    categories.value = data.categories
    return data.categories
  }

  // ── Offerings ──────────────────────────────────────────────────────────────

  async function fetchOfferings({ categoryId = 0, active = -1, cursor = 0, limit = 20 } = {}) {
    const params = { limit }
    if (categoryId) params.category_id = categoryId
    if (active === 1) params.active = 'true'
    else if (active === 0) params.active = 'false'
    if (cursor) params.cursor = cursor

    const { data } = await axios.get('/api/v1/service-offerings', { params })
    if (cursor === 0) {
      offerings.value = data.items
    } else {
      offerings.value = [...offerings.value, ...data.items]
    }
    nextCursor.value = data.next_cursor
    return data
  }

  async function fetchOffering(id) {
    const { data } = await axios.get(`/api/v1/service-offerings/${id}`)
    currentOffering.value = data.offering
    return data.offering
  }

  async function createOffering(payload) {
    const { data } = await axios.post('/api/v1/service-offerings', payload)
    return data.offering
  }

  async function updateOffering(id, payload) {
    const { data } = await axios.put(`/api/v1/service-offerings/${id}`, payload)
    return data.offering
  }

  async function toggleStatus(id, active) {
    const { data } = await axios.patch(`/api/v1/service-offerings/${id}/status`, { active })
    return data.offering
  }

  return {
    categories,
    offerings,
    nextCursor,
    currentOffering,
    fetchCategories,
    fetchOfferings,
    fetchOffering,
    createOffering,
    updateOffering,
    toggleStatus,
  }
})
