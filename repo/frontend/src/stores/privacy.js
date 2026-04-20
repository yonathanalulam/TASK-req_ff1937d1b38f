import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const usePrivacyStore = defineStore('privacy', () => {
  const exportRequest = ref(null)
  const deletionRequest = ref(null)

  async function fetchExportStatus() {
    try {
      const { data } = await axios.get('/api/v1/users/me/export-request/status')
      exportRequest.value = data.export_request
    } catch (err) {
      if (err.response?.status === 404) {
        exportRequest.value = null
      } else {
        throw err
      }
    }
    return exportRequest.value
  }

  async function requestExport() {
    const { data } = await axios.post('/api/v1/users/me/export-request')
    exportRequest.value = data.export_request
    return data.export_request
  }

  function downloadUrl() {
    return '/api/v1/users/me/export-request/download'
  }

  async function fetchDeletionStatus() {
    try {
      const { data } = await axios.get('/api/v1/users/me/deletion-request/status')
      deletionRequest.value = data.deletion_request
    } catch (err) {
      if (err.response?.status === 404) {
        deletionRequest.value = null
      } else {
        throw err
      }
    }
    return deletionRequest.value
  }

  async function requestDeletion() {
    const { data } = await axios.post('/api/v1/users/me/deletion-request', { confirm: 'DELETE' })
    deletionRequest.value = data.deletion_request
    return data.deletion_request
  }

  return {
    exportRequest,
    deletionRequest,
    fetchExportStatus,
    requestExport,
    downloadUrl,
    fetchDeletionStatus,
    requestDeletion,
  }
})
