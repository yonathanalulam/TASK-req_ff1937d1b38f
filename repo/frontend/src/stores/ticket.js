import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

export const useTicketStore = defineStore('ticket', () => {
  const tickets = ref([])
  const currentTicket = ref(null)
  const notes = ref([])
  const attachments = ref([])

  async function fetchTickets(status = '') {
    const params = {}
    if (status) params.status = status
    const { data } = await axios.get('/api/v1/tickets', { params })
    tickets.value = data.tickets
    return data.tickets
  }

  async function fetchTicket(id) {
    const { data } = await axios.get(`/api/v1/tickets/${id}`)
    currentTicket.value = data.ticket
    return data.ticket
  }

  // Ticket creation supports optional attachments via multipart.
  async function createTicket(payload, files = []) {
    const form = new FormData()
    form.append('offering_id', String(payload.offering_id))
    form.append('category_id', String(payload.category_id))
    form.append('address_id', String(payload.address_id))
    form.append('preferred_start', payload.preferred_start)
    form.append('preferred_end', payload.preferred_end)
    if (payload.delivery_method) form.append('delivery_method', payload.delivery_method)
    if (payload.shipping_fee) form.append('shipping_fee', String(payload.shipping_fee))
    for (const f of files) form.append('attachments', f)

    const { data } = await axios.post('/api/v1/tickets', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    return data.ticket
  }

  async function updateStatus(id, status, cancelReason = '') {
    const body = { status }
    if (cancelReason) body.cancel_reason = cancelReason
    const { data } = await axios.patch(`/api/v1/tickets/${id}/status`, body)
    currentTicket.value = data.ticket
    return data.ticket
  }

  async function fetchNotes(id) {
    const { data } = await axios.get(`/api/v1/tickets/${id}/notes`)
    notes.value = data.notes
    return data.notes
  }

  async function createNote(id, content) {
    const { data } = await axios.post(`/api/v1/tickets/${id}/notes`, { content })
    notes.value = [...notes.value, data.note]
    return data.note
  }

  async function fetchAttachments(id) {
    const { data } = await axios.get(`/api/v1/tickets/${id}/attachments`)
    attachments.value = data.attachments
    return data.attachments
  }

  async function deleteAttachment(ticketId, fileId) {
    await axios.delete(`/api/v1/tickets/${ticketId}/attachments/${fileId}`)
    attachments.value = attachments.value.filter((a) => a.id !== fileId)
  }

  return {
    tickets,
    currentTicket,
    notes,
    attachments,
    fetchTickets,
    fetchTicket,
    createTicket,
    updateStatus,
    fetchNotes,
    createNote,
    fetchAttachments,
    deleteAttachment,
  }
})
