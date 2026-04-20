import { test, expect } from '@playwright/test'

async function loginAs(page, username) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill(username)
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

test.describe('Notifications — bell + drawer', () => {
  test('bell icon visible after login', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await expect(page.getByTestId('btn-notifications')).toBeVisible()
  })

  test('clicking bell opens the drawer', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.getByTestId('btn-notifications').click()
    await expect(page.getByTestId('notification-drawer')).toBeVisible()
  })

  test('drawer can be closed by clicking outside', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.getByTestId('btn-notifications').click()
    await expect(page.getByTestId('notification-drawer')).toBeVisible()
    // Click on the backdrop (outside the drawer panel)
    await page.locator('.drawer-backdrop').click({ position: { x: 10, y: 10 } })
    await expect(page.getByTestId('notification-drawer')).toBeHidden({ timeout: 4_000 })
  })

  test('outbox link from drawer navigates to outbox view', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.getByTestId('btn-notifications').click()
    await page.getByTestId('link-outbox').click()
    await expect(page).toHaveURL(/\/notifications\/outbox/, { timeout: 6_000 })
    await expect(page.getByRole('heading', { name: /Notification Outbox/i })).toBeVisible()
  })
})

test.describe('Notifications — ticket status change', () => {
  test('badge increments after a ticket status transition', async ({ page, context, browser }) => {
    // Customer creates a ticket
    await loginAs(page, 'regular_user')
    await page.goto('/addresses')
    if ((await page.locator('.address-card').count()) === 0) {
      await page.getByTestId('btn-add-address').click()
      await page.getByTestId('input-label').fill('Home')
      await page.getByTestId('input-line1').fill('100 Notify Lane')
      await page.getByTestId('input-city').fill('Portland')
      await page.getByTestId('input-state').fill('OR')
      await page.getByTestId('input-zip').fill('97201')
      await page.getByTestId('btn-save-address').click()
      await expect(page.locator('.address-card')).toHaveCount(1, { timeout: 6_000 })
    }
    await page.goto('/tickets/new')
    await page.getByTestId('select-offering').selectOption({ index: 1 })
    await page.getByTestId('select-ticket-category').selectOption({ index: 1 })
    await page.getByTestId('select-address').selectOption({ index: 1 })
    await page.getByTestId('input-preferred-start').fill('2026-06-01T10:00')
    await page.getByTestId('input-preferred-end').fill('2026-06-01T12:00')
    await page.getByTestId('btn-submit-ticket').click()
    await expect(page).toHaveURL(/\/tickets\/\d+/, { timeout: 10_000 })
    const ticketUrl = page.url()

    // Service agent moves the ticket forward
    const agentCtx = await browser.newContext()
    const agentPage = await agentCtx.newPage()
    await loginAs(agentPage, 'service_agent')
    await agentPage.goto(ticketUrl)
    await agentPage.getByTestId('btn-transition-dispatched').click()
    await expect(agentPage.getByTestId('ticket-detail-status')).toContainText('Dispatched',
      { timeout: 6_000 })
    await agentCtx.close()

    // Customer reloads dashboard — bell badge should have at least 1
    await page.goto('/dashboard')
    // Force fresh /me call by reloading
    await page.reload()
    await expect(page.getByTestId('notification-badge')).toBeVisible({ timeout: 6_000 })
  })
})
