import { test, expect } from '@playwright/test'

async function loginAs(page, username) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill(username)
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

test.describe('Privacy Center — navigation', () => {
  test('Privacy nav link visible after login', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await expect(page.getByTestId('link-privacy')).toBeVisible()
  })

  test('Privacy page loads', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.getByTestId('link-privacy').click()
    await expect(page).toHaveURL(/\/privacy/, { timeout: 6_000 })
    await expect(page.getByTestId('export-section')).toBeVisible()
    await expect(page.getByTestId('deletion-section')).toBeVisible()
  })

  test('Privacy page requires auth', async ({ page }) => {
    await page.goto('/privacy')
    await expect(page).toHaveURL(/\/login/)
  })
})

test.describe('Privacy Center — export', () => {
  test('user can request a data export', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/privacy')

    // Status is "No active export" or shows existing one.
    await expect(page.getByTestId('export-status')).toBeVisible()

    // If a button is available, click it
    const requestBtn = page.getByTestId('btn-request-export')
    if (await requestBtn.isVisible()) {
      await requestBtn.click()
      // Status should flip to a non-empty value within a few seconds
      await expect(page.getByTestId('export-status')).not.toContainText('No active export',
        { timeout: 6_000 })
    }
  })
})

test.describe('Privacy Center — deletion confirmation', () => {
  // We don't actually delete the seeded user — we only verify the confirmation
  // gating works, then dismiss the modal.
  test('confirm button stays disabled until DELETE is typed', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/privacy')

    const openBtn = page.getByTestId('btn-open-delete')
    test.skip(!(await openBtn.isVisible()), 'deletion already pending for seed user')

    await openBtn.click()
    await expect(page.getByTestId('modal-delete')).toBeVisible()

    const confirmBtn = page.getByTestId('btn-confirm-delete')
    await expect(confirmBtn).toBeDisabled()

    // Wrong text → still disabled
    await page.getByTestId('input-delete-confirm').fill('delete')
    await expect(confirmBtn).toBeDisabled()

    await page.getByTestId('input-delete-confirm').fill('DELETE')
    await expect(confirmBtn).toBeEnabled()

    // Don't actually click — close the modal so we don't anonymize the seed user
    await page.keyboard.press('Escape')
  })
})
