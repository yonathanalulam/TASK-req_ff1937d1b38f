import { test, expect } from '@playwright/test'

async function loginAs(page, username) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill(username)
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

test.describe('Reviews — summary widget', () => {
  test('summary widget visible on offering detail', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/catalog')
    const cards = page.getByTestId('card-offering')
    await expect(cards.first()).toBeVisible({ timeout: 10_000 })
    await cards.first().click()

    await expect(page.getByTestId('review-summary-total')).toBeVisible({ timeout: 6_000 })
    await expect(page.getByTestId('review-summary-avg')).toBeVisible()
    await expect(page.getByTestId('review-summary-positive-rate')).toBeVisible()
  })

  test('offering detail has links to reviews and Q&A', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/catalog')
    const cards = page.getByTestId('card-offering')
    await expect(cards.first()).toBeVisible({ timeout: 10_000 })
    await cards.first().click()

    await expect(page.getByTestId('link-reviews')).toBeVisible()
    await expect(page.getByTestId('link-qa')).toBeVisible()
  })
})

test.describe('Reviews — list', () => {
  test('reviews page loads for a valid offering', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/catalog')
    const cards = page.getByTestId('card-offering')
    await expect(cards.first()).toBeVisible({ timeout: 10_000 })
    await cards.first().click()

    await page.getByTestId('link-reviews').click()
    await expect(page).toHaveURL(/\/catalog\/\d+\/reviews/, { timeout: 6_000 })
    // Summary widget is shown on the list view too
    await expect(page.getByTestId('review-summary-total')).toBeVisible()
  })
})

test.describe('Reviews — write review', () => {
  test('Write Review button appears on Completed ticket detail', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/tickets')

    const rows = page.getByTestId('ticket-row')
    const available = await rows.count()
    test.skip(available === 0, 'no tickets for this user')

    // Find a Completed ticket (if any)
    let target = null
    for (let i = 0; i < available; i++) {
      const statusText = await rows.nth(i).getByTestId('ticket-status').textContent()
      if (statusText?.trim() === 'Completed' || statusText?.trim() === 'Closed') {
        target = rows.nth(i)
        break
      }
    }
    test.skip(!target, 'no Completed/Closed ticket available for review flow')

    await target.click()
    await expect(page.getByTestId('btn-write-review')).toBeVisible({ timeout: 6_000 })
  })

  test('review modal opens and star rating works', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/tickets')

    const rows = page.getByTestId('ticket-row')
    const available = await rows.count()
    test.skip(available === 0, 'no tickets for this user')
    let target = null
    for (let i = 0; i < available; i++) {
      const statusText = await rows.nth(i).getByTestId('ticket-status').textContent()
      if (statusText?.trim() === 'Completed' || statusText?.trim() === 'Closed') {
        target = rows.nth(i)
        break
      }
    }
    test.skip(!target, 'no Completed/Closed ticket available for review flow')

    await target.click()
    await page.getByTestId('btn-write-review').click()
    await expect(page.getByTestId('modal-review')).toBeVisible()

    await page.getByTestId('star-4').click()
    await page.getByTestId('textarea-review-text').fill('Overall pretty good')
    await expect(page.getByTestId('btn-submit-review')).toBeEnabled()
  })
})
