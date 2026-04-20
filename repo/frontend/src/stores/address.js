import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const useAddressStore = defineStore('address', () => {
  const addresses = ref([])

  async function fetchAddresses() {
    const { data } = await axios.get('/api/v1/users/me/addresses')
    addresses.value = data.addresses
    return data.addresses
  }

  async function createAddress(payload) {
    const { data } = await axios.post('/api/v1/users/me/addresses', payload)
    await fetchAddresses()
    return data.address
  }

  async function updateAddress(id, payload) {
    const { data } = await axios.put(`/api/v1/users/me/addresses/${id}`, payload)
    await fetchAddresses()
    return data.address
  }

  async function deleteAddress(id) {
    await axios.delete(`/api/v1/users/me/addresses/${id}`)
    addresses.value = addresses.value.filter((a) => a.id !== id)
  }

  async function setDefault(id) {
    const { data } = await axios.put(`/api/v1/users/me/addresses/${id}/default`)
    await fetchAddresses()
    return data.address
  }

  return {
    addresses,
    fetchAddresses,
    createAddress,
    updateAddress,
    deleteAddress,
    setDefault,
  }
})
