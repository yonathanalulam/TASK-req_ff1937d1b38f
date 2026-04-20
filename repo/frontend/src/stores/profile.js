import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const useProfileStore = defineStore('profile', () => {
  const profile = ref(null)
  const preferences = ref(null)
  const favorites = ref([])
  const favoritesNextCursor = ref(0)
  const history = ref([])
  const historyNextCursor = ref(0)

  // ── Profile ────────────────────────────────────────────────────────────────

  async function fetchProfile() {
    const { data } = await axios.get('/api/v1/users/me/profile')
    profile.value = data.profile
    return data.profile
  }

  async function updateProfile(payload) {
    const { data } = await axios.put('/api/v1/users/me/profile', payload)
    profile.value = data.profile
    return data.profile
  }

  // ── Preferences ────────────────────────────────────────────────────────────

  async function fetchPreferences() {
    const { data } = await axios.get('/api/v1/users/me/preferences')
    preferences.value = data.preferences
    return data.preferences
  }

  async function updatePreferences(payload) {
    const { data } = await axios.put('/api/v1/users/me/preferences', payload)
    preferences.value = data.preferences
    return data.preferences
  }

  // ── Favorites ──────────────────────────────────────────────────────────────

  async function fetchFavorites(cursor = 0, limit = 20) {
    const { data } = await axios.get('/api/v1/users/me/favorites', {
      params: { cursor, limit },
    })
    if (cursor === 0) {
      favorites.value = data.items
    } else {
      favorites.value = [...favorites.value, ...data.items]
    }
    favoritesNextCursor.value = data.next_cursor
    return data
  }

  async function addFavorite(offeringId) {
    await axios.post('/api/v1/users/me/favorites', { offering_id: offeringId })
    await fetchFavorites()
  }

  async function removeFavorite(offeringId) {
    await axios.delete(`/api/v1/users/me/favorites/${offeringId}`)
    favorites.value = favorites.value.filter((f) => f.offering_id !== offeringId)
  }

  // ── History ────────────────────────────────────────────────────────────────

  async function fetchHistory(cursor = 0, limit = 20) {
    const { data } = await axios.get('/api/v1/users/me/history', {
      params: { cursor, limit },
    })
    if (cursor === 0) {
      history.value = data.items
    } else {
      history.value = [...history.value, ...data.items]
    }
    historyNextCursor.value = data.next_cursor
    return data
  }

  async function clearHistory() {
    await axios.delete('/api/v1/users/me/history')
    history.value = []
    historyNextCursor.value = 0
  }

  return {
    profile,
    preferences,
    favorites,
    favoritesNextCursor,
    history,
    historyNextCursor,
    fetchProfile,
    updateProfile,
    fetchPreferences,
    updatePreferences,
    fetchFavorites,
    addFavorite,
    removeFavorite,
    fetchHistory,
    clearHistory,
  }
})
