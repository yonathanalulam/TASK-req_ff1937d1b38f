import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const useQAStore = defineStore('qa', () => {
  const threadsByOffering = ref({}) // offeringId → { items, next_cursor }

  async function fetchThreads(offeringId, cursor = 0, limit = 20) {
    const params = { cursor, limit }
    const { data } = await axios.get(`/api/v1/service-offerings/${offeringId}/qa`, { params })
    const prior = threadsByOffering.value[offeringId]?.items ?? []
    threadsByOffering.value = {
      ...threadsByOffering.value,
      [offeringId]: {
        items: cursor === 0 ? data.items : [...prior, ...data.items],
        next_cursor: data.next_cursor,
      },
    }
    return data
  }

  async function createThread(offeringId, question) {
    const { data } = await axios.post(`/api/v1/service-offerings/${offeringId}/qa`, { question })
    // refresh list
    await fetchThreads(offeringId)
    return data.thread
  }

  async function createReply(offeringId, threadId, content) {
    const { data } = await axios.post(
      `/api/v1/service-offerings/${offeringId}/qa/${threadId}/replies`,
      { content },
    )
    // refresh list
    await fetchThreads(offeringId)
    return data.reply
  }

  async function deletePost(postId, offeringId) {
    await axios.delete(`/api/v1/qa/${postId}`)
    if (offeringId) await fetchThreads(offeringId)
  }

  return {
    threadsByOffering,
    fetchThreads,
    createThread,
    createReply,
    deletePost,
  }
})
