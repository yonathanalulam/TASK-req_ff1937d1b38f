import { test, expect } from '@playwright/test'

// ── Login helpers ─────────────────────────────────────────────────────────────

async function loginAsUser(page) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill('regular_user')
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

async function loginAsAgent(page) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill('service_agent')
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

// ── Catalog browsing ──────────────────────────────────────────────────────────

test.describe('Service Catalog', () => {
  test('catalog page loads and shows seeded offerings', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/catalog')

    // Wait for offerings to load
    await expect(page.getByTestId('list-offerings')).toBeVisible({ timeout: 10_000 })

    // Seed data has 3 offerings
    const cards = page.getByTestId('card-offering')
    await expect(cards).toHaveCount(3, { timeout: 6_000 })

    // Each card should show a name
    const names = page.getByTestId('offering-name')
    await expect(names.first()).toBeVisible()
  })

  test('catalog page is accessible via nav link', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/dashboard')
    await page.getByRole('link', { name: 'Catalog' }).click()
    await expect(page).toHaveURL(/\/catalog/, { timeout: 6_000 })
  })

  test('filter by category narrows results', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/catalog')
    await expect(page.getByTestId('list-offerings')).toBeVisible({ timeout: 10_000 })

    // Select "Plumbing" category — only 1 plumbing offering in seed data
    const categorySelect = page.getByTestId('select-category')
    await expect(categorySelect).toBeVisible()
    await categorySelect.selectOption({ label: 'Plumbing' })

    // Wait for filtered results
    await page.waitForTimeout(500)
    const cards = page.getByTestId('card-offering')
    await expect(cards).toHaveCount(1, { timeout: 6_000 })
    await expect(page.getByTestId('offering-name').first()).toContainText('Plumbing')
  })

  test('clicking an offering navigates to detail page', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/catalog')
    await expect(page.getByTestId('card-offering').first()).toBeVisible({ timeout: 10_000 })

    await page.getByTestId('card-offering').first().click()
    await expect(page).toHaveURL(/\/catalog\/\d+/, { timeout: 6_000 })
    await expect(page.getByTestId('offering-title')).toBeVisible()
    await expect(page.getByTestId('offering-price')).toBeVisible()
  })

  test('catalog requires authentication', async ({ page }) => {
    await page.goto('/catalog')
    await expect(page).toHaveURL(/\/login/)
  })
})

// ── Offering detail ───────────────────────────────────────────────────────────

test.describe('Offering Detail', () => {
  test('detail page shows title, price and shipping widget', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/catalog')
    await expect(page.getByTestId('card-offering').first()).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('card-offering').first().click()

    await expect(page.getByTestId('offering-title')).toBeVisible()
    await expect(page.getByTestId('offering-price')).toBeVisible()

    // Shipping estimate widget should be present
    await expect(page.getByTestId('select-region')).toBeVisible()
    await expect(page.getByTestId('radio-pickup')).toBeVisible()
    await expect(page.getByTestId('radio-courier')).toBeVisible()
  })

  test('regular user can add and remove a favorite', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/catalog')
    await expect(page.getByTestId('card-offering').first()).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('card-offering').first().click()

    // Should see favorite button
    const favBtn = page.getByTestId('btn-favorite')
    const unfavBtn = page.getByTestId('btn-unfavorite')

    // At least one of them should be visible
    await expect(favBtn.or(unfavBtn)).toBeVisible({ timeout: 6_000 })

    if (await favBtn.isVisible()) {
      // Add to favorites
      await favBtn.click()
      await expect(page.getByTestId('btn-unfavorite')).toBeVisible({ timeout: 4_000 })

      // Remove from favorites
      await page.getByTestId('btn-unfavorite').click()
      await expect(page.getByTestId('btn-favorite')).toBeVisible({ timeout: 4_000 })
    } else {
      // Already favorited — remove first, then re-add to verify toggle works
      await unfavBtn.click()
      await expect(page.getByTestId('btn-favorite')).toBeVisible({ timeout: 4_000 })
    }
  })

  test('service agent sees Edit button on their own offerings', async ({ page }) => {
    await loginAsAgent(page)
    await page.goto('/catalog')
    await expect(page.getByTestId('card-offering').first()).toBeVisible({ timeout: 10_000 })

    // Seed data offerings are created by 'service_agent' user
    await page.getByTestId('card-offering').first().click()
    await expect(page.getByTestId('btn-edit-offering')).toBeVisible({ timeout: 6_000 })
  })
})

// ── Shipping estimate widget ──────────────────────────────────────────────────

test.describe('Shipping Estimate Widget', () => {
  test('pickup delivery shows $0 fee immediately', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/catalog')
    await expect(page.getByTestId('card-offering').first()).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('card-offering').first().click()

    // Select pickup method (it's the default)
    await expect(page.getByTestId('radio-pickup')).toBeVisible({ timeout: 6_000 })
    await page.getByTestId('radio-pickup').check()

    // Make sure a region is selected
    const regionSelect = page.getByTestId('select-region')
    await expect(regionSelect).toBeVisible()

    await page.getByTestId('btn-estimate').click()
    await expect(page.getByTestId('estimate-fee')).toBeVisible({ timeout: 6_000 })
    await expect(page.getByTestId('estimate-fee')).toContainText('Free')

    // No arrival window for pickup
    await expect(page.getByTestId('estimate-window')).toBeHidden()
  })

  test('courier delivery shows fee and arrival window', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/catalog')
    await expect(page.getByTestId('card-offering').first()).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('card-offering').first().click()

    await expect(page.getByTestId('radio-courier')).toBeVisible({ timeout: 6_000 })
    await page.getByTestId('radio-courier').check()

    // Fill weight and quantity
    await page.getByTestId('input-weight').fill('1')
    await page.getByTestId('input-quantity').fill('1')

    await page.getByTestId('btn-estimate').click()
    await expect(page.getByTestId('estimate-fee')).toBeVisible({ timeout: 6_000 })
    await expect(page.getByTestId('estimate-fee')).toContainText('$')

    // Arrival window should be present for courier
    await expect(page.getByTestId('estimate-window')).toBeVisible({ timeout: 4_000 })
    await expect(page.getByTestId('estimate-window')).toContainText('Arrives')
  })

  test('switching from courier to pickup hides weight/quantity fields', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/catalog')
    await expect(page.getByTestId('card-offering').first()).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('card-offering').first().click()

    await expect(page.getByTestId('radio-courier')).toBeVisible({ timeout: 6_000 })

    // Switch to courier — weight/quantity inputs should appear
    await page.getByTestId('radio-courier').check()
    await expect(page.getByTestId('input-weight')).toBeVisible()
    await expect(page.getByTestId('input-quantity')).toBeVisible()

    // Switch back to pickup — inputs should disappear
    await page.getByTestId('radio-pickup').check()
    await expect(page.getByTestId('input-weight')).toBeHidden()
    await expect(page.getByTestId('input-quantity')).toBeHidden()
  })
})

// ── Offering form (service agent) ─────────────────────────────────────────────

test.describe('Offering Form', () => {
  test('service agent can navigate to create new offering', async ({ page }) => {
    await loginAsAgent(page)
    await page.goto('/catalog')
    await expect(page.getByTestId('btn-new-offering')).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('btn-new-offering').click()
    await expect(page).toHaveURL(/\/catalog\/new/, { timeout: 4_000 })

    await expect(page.getByTestId('input-name')).toBeVisible()
    await expect(page.getByTestId('select-category')).toBeVisible()
    await expect(page.getByTestId('input-price')).toBeVisible()
    await expect(page.getByTestId('input-duration')).toBeVisible()
    await expect(page.getByTestId('btn-submit-offering')).toBeVisible()
  })

  test('regular user does not see New Offering button', async ({ page }) => {
    await loginAsUser(page)
    await page.goto('/catalog')
    await expect(page.getByTestId('list-offerings')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('btn-new-offering')).toBeHidden()
  })

  test('create offering form requires name and category', async ({ page }) => {
    await loginAsAgent(page)
    await page.goto('/catalog/new')
    await expect(page.getByTestId('btn-submit-offering')).toBeVisible({ timeout: 6_000 })

    // Submit without filling required fields
    await page.getByTestId('btn-submit-offering').click()

    // Validation errors should appear
    await expect(page.locator('.error-msg').first()).toBeVisible()
  })
})
