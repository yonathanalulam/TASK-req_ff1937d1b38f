import { test, expect } from '@playwright/test'

// ── Login helpers (seed credentials: password "password") ─────────────────────

async function loginAs(page, username) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill(username)
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

async function ensureSavedAddress(page) {
  await page.goto('/addresses')
  const cards = page.locator('.address-card')
  if ((await cards.count()) === 0) {
    await page.getByTestId('btn-add-address').click()
    await page.getByTestId('input-label').fill('Home')
    await page.getByTestId('input-line1').fill('100 Test Lane')
    await page.getByTestId('input-city').fill('Portland')
    await page.getByTestId('input-state').fill('OR')
    await page.getByTestId('input-zip').fill('97201')
    await page.getByTestId('btn-save-address').click()
    await expect(page.locator('.address-card')).toHaveCount(1, { timeout: 6_000 })
  }
}

// ── Navigation ────────────────────────────────────────────────────────────────

test.describe('Tickets — navigation', () => {
  test('tickets page is accessible via nav link', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.getByRole('link', { name: 'Tickets' }).click()
    await expect(page).toHaveURL(/\/tickets/, { timeout: 6_000 })
  })

  test('tickets require authentication', async ({ page }) => {
    await page.goto('/tickets')
    await expect(page).toHaveURL(/\/login/)
  })
})

// ── Ticket submission ────────────────────────────────────────────────────────

test.describe('Tickets — create', () => {
  test('regular user can submit a ticket', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await ensureSavedAddress(page)

    await page.goto('/tickets/new')
    await expect(page.getByTestId('select-offering')).toBeVisible()

    // Pick first offering, first category, first address
    await page.getByTestId('select-offering').selectOption({ index: 1 })
    await page.getByTestId('select-ticket-category').selectOption({ index: 1 })
    await page.getByTestId('select-address').selectOption({ index: 1 })
    await page.getByTestId('input-preferred-start').fill('2026-06-01T10:00')
    await page.getByTestId('input-preferred-end').fill('2026-06-01T12:00')

    await page.getByTestId('btn-submit-ticket').click()

    // Should end up on ticket detail with Accepted status
    await expect(page).toHaveURL(/\/tickets\/\d+/, { timeout: 10_000 })
    await expect(page.getByTestId('ticket-detail-status')).toContainText('Accepted')
  })

  test('submission requires all fields', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/tickets/new')
    await page.getByTestId('btn-submit-ticket').click()
    // Validation errors should appear
    await expect(page.locator('.error-msg').first()).toBeVisible()
  })
})

// ── Ticket detail ─────────────────────────────────────────────────────────────

test.describe('Tickets — detail', () => {
  test('detail page shows SLA deadline for a fresh ticket', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await ensureSavedAddress(page)
    await page.goto('/tickets/new')

    await page.getByTestId('select-offering').selectOption({ index: 1 })
    await page.getByTestId('select-ticket-category').selectOption({ index: 1 })
    await page.getByTestId('select-address').selectOption({ index: 1 })
    await page.getByTestId('input-preferred-start').fill('2026-06-01T10:00')
    await page.getByTestId('input-preferred-end').fill('2026-06-01T12:00')
    await page.getByTestId('btn-submit-ticket').click()
    await expect(page).toHaveURL(/\/tickets\/\d+/, { timeout: 10_000 })

    await expect(page.getByTestId('ticket-sla-deadline')).toBeVisible()
    await expect(page.getByTestId('ticket-sla-breached')).toBeHidden()
  })

  test('regular user can add a note', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/tickets')
    const rows = page.getByTestId('ticket-row')
    await expect(rows.first()).toBeVisible({ timeout: 10_000 })
    await rows.first().click()

    await expect(page.getByTestId('textarea-note')).toBeVisible()
    await page.getByTestId('textarea-note').fill('Please ring bell twice')
    await page.getByTestId('btn-add-note').click()
    await expect(page.getByTestId('note-item').last()).toContainText('Please ring bell twice',
      { timeout: 6_000 })
  })

  test('regular user can cancel a ticket in Accepted state', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/tickets')
    const rows = page.getByTestId('ticket-row')
    await expect(rows.first()).toBeVisible({ timeout: 10_000 })

    // Find an Accepted ticket
    let target = null
    const count = await rows.count()
    for (let i = 0; i < count; i++) {
      const statusText = await rows.nth(i).getByTestId('ticket-status').textContent()
      if (statusText?.trim() === 'Accepted') {
        target = rows.nth(i)
        break
      }
    }
    test.skip(!target, 'no Accepted ticket available for this user')
    await target.click()

    // Prepare to accept prompt dialog
    page.on('dialog', async (dialog) => {
      if (dialog.type() === 'prompt') await dialog.accept('test cancel')
      else await dialog.accept()
    })
    await page.getByTestId('btn-cancel-ticket').click()
    await expect(page.getByTestId('ticket-detail-status')).toContainText('Cancelled',
      { timeout: 6_000 })
  })
})

// ── Service agent flow ───────────────────────────────────────────────────────

test.describe('Tickets — agent lifecycle', () => {
  test('service agent can move a ticket through Dispatched → In Service → Completed', async ({ page }) => {
    await loginAs(page, 'service_agent')
    await page.goto('/tickets')
    const rows = page.getByTestId('ticket-row')

    const available = await rows.count()
    test.skip(available === 0, 'no tickets visible for service_agent')

    // Find an Accepted ticket
    let target = null
    for (let i = 0; i < available; i++) {
      const statusText = await rows.nth(i).getByTestId('ticket-status').textContent()
      if (statusText?.trim() === 'Accepted') {
        target = rows.nth(i)
        break
      }
    }
    test.skip(!target, 'no Accepted ticket available')
    await target.click()

    await page.getByTestId('btn-transition-dispatched').click()
    await expect(page.getByTestId('ticket-detail-status')).toContainText('Dispatched',
      { timeout: 6_000 })

    await page.getByTestId('btn-transition-inservice').click()
    await expect(page.getByTestId('ticket-detail-status')).toContainText('In Service',
      { timeout: 6_000 })

    await page.getByTestId('btn-transition-completed').click()
    await expect(page.getByTestId('ticket-detail-status')).toContainText('Completed',
      { timeout: 6_000 })
  })
})
