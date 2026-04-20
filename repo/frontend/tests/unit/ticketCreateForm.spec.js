// Unit tests for the ticket creation form. We mount the real
// TicketCreateView and stub the Pinia stores it depends on.
//
// The focus is on the local validation rules: required fields, end-after-
// start time ordering, file count/size/type constraints. These live in the
// component's setup script so rendering is the cleanest way to exercise
// them deterministically.

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

// Stub stores before the component imports them. vi.mock is hoisted.
const catalog = vi.hoisted(() => ({
  offerings: [{ id: 1, name: 'Cleaning', category_id: 10 }],
  categories: [{ id: 10, name: 'Home Services' }],
  fetchCategories: vi.fn(async () => {}),
  fetchOfferings: vi.fn(async () => {}),
}))
const addressStore = vi.hoisted(() => ({
  addresses: [
    { id: 5, label: 'Home', address_line1: '1 Main', city: 'X', state: 'NY' },
  ],
  fetchAddresses: vi.fn(async () => {}),
}))
const ticket = vi.hoisted(() => ({
  createTicket: vi.fn(async () => ({ id: 99 })),
}))

vi.mock('@/stores/catalog', () => ({ useCatalogStore: () => catalog }))
vi.mock('@/stores/address', () => ({ useAddressStore: () => addressStore }))
vi.mock('@/stores/ticket', () => ({ useTicketStore: () => ticket }))

const routerPush = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush, back: vi.fn() }),
}))

import TicketCreateView from '@/views/TicketCreateView.vue'

function mountForm() {
  return mount(TicketCreateView, {
    global: {
      plugins: [createPinia()],
      stubs: { 'router-link': true },
    },
  })
}

describe('TicketCreateView validation', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    routerPush.mockReset()
    ticket.createTicket.mockReset().mockResolvedValue({ id: 99 })
  })

  it('blocks submit when required fields are empty and surfaces errors', async () => {
    const wrapper = mountForm()
    await wrapper.find('form').trigger('submit.prevent')
    expect(ticket.createTicket).not.toHaveBeenCalled()
    const msgs = wrapper.findAll('.error-msg').map((w) => w.text())
    expect(msgs.join('\n')).toContain('Required')
  })

  it('rejects an end time earlier than the start time', async () => {
    const wrapper = mountForm()
    await wrapper.find('[data-testid="select-offering"]').setValue('1')
    await wrapper.find('[data-testid="select-ticket-category"]').setValue('10')
    await wrapper.find('[data-testid="select-address"]').setValue('5')
    await wrapper.find('[data-testid="input-preferred-start"]').setValue('2026-04-20T12:00')
    await wrapper.find('[data-testid="input-preferred-end"]').setValue('2026-04-20T10:00')
    await wrapper.find('form').trigger('submit.prevent')
    expect(ticket.createTicket).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('End must be after start.')
  })

  it('submits when the form is valid and routes to the new ticket', async () => {
    const wrapper = mountForm()
    await wrapper.find('[data-testid="select-offering"]').setValue('1')
    await wrapper.find('[data-testid="select-ticket-category"]').setValue('10')
    await wrapper.find('[data-testid="select-address"]').setValue('5')
    await wrapper.find('[data-testid="input-preferred-start"]').setValue('2026-04-20T09:00')
    await wrapper.find('[data-testid="input-preferred-end"]').setValue('2026-04-20T11:00')
    await wrapper.find('form').trigger('submit.prevent')
    // Let the submit promise resolve.
    await Promise.resolve()
    await Promise.resolve()
    expect(ticket.createTicket).toHaveBeenCalledTimes(1)
    const payload = ticket.createTicket.mock.calls[0][0]
    expect(payload.offering_id).toBe(1)
    expect(payload.category_id).toBe(10)
    expect(payload.address_id).toBe(5)
  })
})
