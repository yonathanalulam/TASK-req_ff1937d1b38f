import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const useReviewStore = defineStore('review', () => {
  const reviewsByOffering = ref({}) // offeringId → { items: [], next_cursor: 0 }
  const summaryByOffering = ref({}) // offeringId → { total_reviews, average_rating, positive_rate }

  async function fetchReviews(offeringId, cursor = 0, limit = 20) {
    const params = { cursor, limit }
    const { data } = await axios.get(`/api/v1/service-offerings/${offeringId}/reviews`, { params })
    const prior = reviewsByOffering.value[offeringId]?.items ?? []
    reviewsByOffering.value = {
      ...reviewsByOffering.value,
      [offeringId]: {
        items: cursor === 0 ? data.items : [...prior, ...data.items],
        next_cursor: data.next_cursor,
      },
    }
    return data
  }

  async function fetchSummary(offeringId) {
    const { data } = await axios.get(`/api/v1/service-offerings/${offeringId}/review-summary`)
    summaryByOffering.value = { ...summaryByOffering.value, [offeringId]: data }
    return data
  }

  async function createReview(ticketId, { rating, text }, images = []) {
    if (images.length === 0) {
      const { data } = await axios.post(`/api/v1/tickets/${ticketId}/reviews`, { rating, text })
      return data.review
    }
    const form = new FormData()
    form.append('rating', String(rating))
    if (text) form.append('text', text)
    for (const img of images) form.append('images', img)
    const { data } = await axios.post(`/api/v1/tickets/${ticketId}/reviews`, form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    return data.review
  }

  async function updateReview(ticketId, reviewId, { rating, text }) {
    const { data } = await axios.put(
      `/api/v1/tickets/${ticketId}/reviews/${reviewId}`,
      { rating, text },
    )
    return data.review
  }

  async function reportReview(reviewId, reason, details = '') {
    const { data } = await axios.post(`/api/v1/reviews/${reviewId}/reports`, { reason, details })
    return data.report
  }

  return {
    reviewsByOffering,
    summaryByOffering,
    fetchReviews,
    fetchSummary,
    createReview,
    updateReview,
    reportReview,
  }
})
