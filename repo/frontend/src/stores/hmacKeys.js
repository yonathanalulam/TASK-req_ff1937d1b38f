import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

// HMAC signing keys store (administrator-only).
//
// Backend returns the plaintext `secret` exactly once — on create and rotate
// responses. `lastReveal` caches the most recent reveal locally so the admin
// view can display it with a "copy now" warning. Callers should clear it
// explicitly after the secret has been distributed to the intended client.
export const useHmacKeysStore = defineStore('hmacKeys', () => {
  const keys = ref([])
  const lastReveal = ref(null) // { key_id, secret, action: 'create' | 'rotate', at }

  async function fetchKeys() {
    const { data } = await axios.get('/api/v1/admin/hmac-keys')
    keys.value = data.keys || []
    return keys.value
  }

  async function createKey(keyId) {
    const { data } = await axios.post('/api/v1/admin/hmac-keys', { key_id: keyId })
    lastReveal.value = {
      key_id: data.key.key_id,
      secret: data.secret,
      action: 'create',
      at: new Date().toISOString(),
    }
    await fetchKeys()
    return data
  }

  async function rotateKey(keyId) {
    const { data } = await axios.post('/api/v1/admin/hmac-keys/rotate', { key_id: keyId })
    lastReveal.value = {
      key_id: data.key.key_id,
      secret: data.secret,
      action: 'rotate',
      at: new Date().toISOString(),
    }
    await fetchKeys()
    return data
  }

  async function revokeKey(id) {
    await axios.delete(`/api/v1/admin/hmac-keys/${id}`)
    await fetchKeys()
  }

  function clearReveal() {
    lastReveal.value = null
  }

  return { keys, lastReveal, fetchKeys, createKey, rotateKey, revokeKey, clearReveal }
})
