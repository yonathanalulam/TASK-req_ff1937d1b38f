import { test, expect } from '@playwright/test'

// Helper: log in with seed admin credentials and land on dashboard.
async function loginAsAdmin(page) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill('admin')
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

// Helper: log in with seed regular_user credentials.
async function loginAsUser(page) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill('regular_user')
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

test.describe('Profile', () => {
  test('profile page loads and shows masked phone after update', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/profile')
    await expect(page.getByTestId('input-display-name')).toBeVisible()

    // Update display name and set a phone
    await page.getByTestId('input-display-name').fill('Test Regular User')
    await page.getByTestId('input-phone').fill('4155551234')
    await page.getByTestId('btn-save-profile').click()

    // Masked phone should appear after save
    await expect(page.getByTestId('masked-phone')).toBeVisible({ timeout: 6_000 })
    const maskedText = await page.getByTestId('masked-phone').textContent()
    expect(maskedText).toContain('***')
    expect(maskedText).toContain('1234') // last 4 digits visible
  })

  test('profile page requires auth', async ({ page }) => {
    await page.goto('/profile')
    await expect(page).toHaveURL(/\/login/)
  })
})

test.describe('Preferences', () => {
  test('preference toggle persists across page reload', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/preferences')

    // Uncheck notify_in_app
    const toggle = page.getByTestId('toggle-notify-in-app')
    await expect(toggle).toBeVisible()

    // Make sure it is currently checked, then uncheck
    const isChecked = await toggle.isChecked()
    if (isChecked) {
      await toggle.uncheck()
    } else {
      // Already unchecked — check it and then uncheck to confirm toggle works
      await toggle.check()
      await toggle.uncheck()
    }
    await expect(toggle).not.toBeChecked()

    await page.getByTestId('btn-save-prefs').click()

    // Wait for success toast or page quiet
    await page.waitForTimeout(1_000)

    // Reload and verify the preference persisted
    await page.reload()
    await expect(page.getByTestId('toggle-notify-in-app')).not.toBeChecked()
  })
})

test.describe('Address Book', () => {
  test('create, verify default badge, set second default, delete', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/addresses')

    // ── Create first address ──────────────────────────────────────────────────
    await page.getByTestId('btn-add-address').click()
    await expect(page.getByTestId('input-line1')).toBeVisible()

    await page.getByTestId('input-label').fill('Home')
    await page.getByTestId('input-line1').fill('123 Oak Street')
    await page.getByTestId('input-city').fill('Portland')
    await page.getByTestId('input-state').fill('OR')
    await page.getByTestId('input-zip').fill('97201')
    await page.getByTestId('btn-save-address').click()

    // Modal closes; first address should show "Default" badge
    await expect(page.getByTestId('badge-default')).toBeVisible({ timeout: 6_000 })

    // ── Create second address ─────────────────────────────────────────────────
    await page.getByTestId('btn-add-address').click()
    await page.getByTestId('input-label').fill('Work')
    await page.getByTestId('input-line1').fill('456 Pine Ave')
    await page.getByTestId('input-city').fill('Portland')
    await page.getByTestId('input-state').fill('OR')
    await page.getByTestId('input-zip').fill('97202')
    await page.getByTestId('btn-save-address').click()

    await expect(page.locator('.address-card')).toHaveCount(2, { timeout: 6_000 })

    // ── Set second address as default ─────────────────────────────────────────
    // Find the "Set as Default" button in the non-default card
    const setDefaultBtns = page.locator('[data-testid^="btn-set-default-"]')
    await expect(setDefaultBtns).toHaveCount(1) // only the second card has this button
    await setDefaultBtns.first().click()

    // Now there should be exactly 1 default badge, on the second card
    await expect(page.getByTestId('badge-default')).toHaveCount(1, { timeout: 6_000 })

    // ── Inline ZIP validation ─────────────────────────────────────────────────
    await page.getByTestId('btn-add-address').click()
    await page.getByTestId('input-line1').fill('789 Elm')
    await page.getByTestId('input-city').fill('NYC')
    await page.getByTestId('input-state').fill('NY')
    await page.getByTestId('input-zip').fill('BADZIP')
    await page.getByTestId('btn-save-address').click()
    // Should show inline error, NOT close modal
    await expect(page.locator('.field.error')).toBeVisible()
    await page.keyboard.press('Escape')

    // ── Delete first address (the non-default one) ────────────────────────────
    // Dismiss modal if still open
    const closeBtn = page.locator('.btn-close')
    if (await closeBtn.isVisible()) {
      await closeBtn.click()
    }

    // Find a delete button for a non-default card
    const deleteBtns = page.locator('[data-testid^="btn-delete-"]')
    await expect(deleteBtns).toHaveCount(2)

    // Intercept confirm() dialog
    page.on('dialog', (dialog) => dialog.accept())
    await deleteBtns.first().click()

    await expect(page.locator('.address-card')).toHaveCount(1, { timeout: 6_000 })
  })

  test('invalid ZIP shows inline error and blocks submission', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/addresses')

    await page.getByTestId('btn-add-address').click()
    await page.getByTestId('input-line1').fill('1 Main')
    await page.getByTestId('input-city').fill('City')
    await page.getByTestId('input-state').fill('CA')
    await page.getByTestId('input-zip').fill('ABC')
    await page.getByTestId('btn-save-address').click()

    // Modal stays open; ZIP field has error class
    await expect(page.locator('[data-testid="input-zip"]').locator('..').locator('..')).toBeVisible()
    // The error message should be present in the DOM
    await expect(page.locator('.error-msg').filter({ hasText: /ZIP/i })).toBeVisible()
  })

  test('edit address updates fields', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/addresses')

    // Create one address first
    await page.getByTestId('btn-add-address').click()
    await page.getByTestId('input-label').fill('Temp')
    await page.getByTestId('input-line1').fill('100 Start Rd')
    await page.getByTestId('input-city').fill('Austin')
    await page.getByTestId('input-state').fill('TX')
    await page.getByTestId('input-zip').fill('73301')
    await page.getByTestId('btn-save-address').click()

    await expect(page.locator('.address-card')).toHaveCount(1, { timeout: 6_000 })

    // Edit it
    const editBtn = page.locator('[data-testid^="btn-edit-"]').first()
    await editBtn.click()

    await page.getByTestId('input-line1').fill('200 Updated Blvd')
    await page.getByTestId('btn-save-address').click()

    // Verify updated address shows new line
    await expect(page.locator('.card-body')).toContainText('200 Updated Blvd', { timeout: 6_000 })
  })
})
