import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const useModerationStore = defineStore('moderation', () => {
  const queue = ref([])
  const terms = ref([])
  const violationsByUser = ref({})

  async function fetchQueue(status = 'pending') {
    const { data } = await axios.get('/api/v1/moderation/queue', { params: { status } })
    queue.value = data.items
    return data.items
  }

  async function approve(id, reason = '') {
    const { data } = await axios.post(`/api/v1/moderation/queue/${id}/approve`, { reason })
    queue.value = queue.value.filter((it) => it.id !== id)
    return data.item
  }

  async function reject(id, reason = '') {
    const { data } = await axios.post(`/api/v1/moderation/queue/${id}/reject`, { reason })
    queue.value = queue.value.filter((it) => it.id !== id)
    return data
  }

  async function fetchTerms() {
    const { data } = await axios.get('/api/v1/admin/sensitive-terms')
    terms.value = data.terms
    return data.terms
  }

  async function addTerm(term, klass) {
    const { data } = await axios.post('/api/v1/admin/sensitive-terms', { term, class: klass })
    await fetchTerms()
    return data.term
  }

  async function deleteTerm(id) {
    await axios.delete(`/api/v1/admin/sensitive-terms/${id}`)
    terms.value = terms.value.filter((t) => t.id !== id)
  }

  async function fetchUserViolations(userId) {
    const { data } = await axios.get(`/api/v1/admin/users/${userId}/violations`)
    violationsByUser.value = { ...violationsByUser.value, [userId]: data }
    return data
  }

  return {
    queue,
    terms,
    violationsByUser,
    fetchQueue,
    approve,
    reject,
    fetchTerms,
    addTerm,
    deleteTerm,
    fetchUserViolations,
  }
})
