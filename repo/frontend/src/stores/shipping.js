import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const useShippingStore = defineStore('shipping', () => {
  const regions = ref([])
  const templates = ref([])
  const estimateResult = ref(null)

  async function fetchRegions() {
    const { data } = await axios.get('/api/v1/shipping/regions')
    regions.value = data.regions
    return data.regions
  }

  async function fetchTemplates(regionId = 0) {
    const params = {}
    if (regionId) params.region_id = regionId
    const { data } = await axios.get('/api/v1/shipping/templates', { params })
    templates.value = data.templates
    return data.templates
  }

  async function getEstimate(payload) {
    const { data } = await axios.post('/api/v1/shipping/estimate', payload)
    estimateResult.value = data
    return data
  }

  return {
    regions,
    templates,
    estimateResult,
    fetchRegions,
    fetchTemplates,
    getEstimate,
  }
})
