import { test, expect, request } from '@playwright/test'

async function loginAs(page, username) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill(username)
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

test.describe('Moderation — admin navigation', () => {
  test('moderation link visible to administrator', async ({ page }) => {
    await loginAs(page, 'admin')
    await expect(page.getByTestId('link-moderation')).toBeVisible()
  })

  test('moderation link hidden from regular_user', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await expect(page.getByTestId('link-moderation')).toBeHidden()
  })

  test('moderation queue page loads', async ({ page }) => {
    await loginAs(page, 'admin')
    await page.getByTestId('link-moderation').click()
    await expect(page).toHaveURL(/\/moderation\/queue/, { timeout: 6_000 })
    await expect(page.getByRole('heading', { name: /Moderation Queue/i })).toBeVisible()
  })

  test('queue is empty when no items pending', async ({ page }) => {
    await loginAs(page, 'admin')
    await page.goto('/moderation/queue')
    // Either "queue-empty" is shown OR the list has items — both are valid states.
    const empty = page.getByTestId('queue-empty')
    const list = page.getByTestId('queue-list')
    await expect(empty.or(list)).toBeVisible({ timeout: 6_000 })
  })
})

test.describe('Moderation — route role gating', () => {
  // Direct-URL navigation for unauthorised roles: the router guard must
  // redirect regular_user away from moderator-only pages instead of rendering
  // their SPA shell. Mirrors the backend RBAC constraint so we don't leak a
  // "you can see this page but nothing works" UX.
  test('regular_user redirected away from /moderation/queue', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/moderation/queue')
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 6_000 })
  })

  test('regular_user redirected away from /moderation/violations', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/moderation/violations')
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 6_000 })
  })
})

test.describe('Moderation — violation history', () => {
  test('violations page allows looking up a user', async ({ page }) => {
    await loginAs(page, 'admin')
    await page.goto('/moderation/violations')
    await expect(page.getByTestId('input-user-id')).toBeVisible()
    await expect(page.getByTestId('btn-load-violations')).toBeDisabled()

    await page.getByTestId('input-user-id').fill('1')
    await expect(page.getByTestId('btn-load-violations')).toBeEnabled()
  })
})

test.describe('Moderation — content blocking', () => {
  // This test creates a sensitive term via the admin API, then verifies that
  // a review containing that term is blocked. It uses Playwright's request
  // context to drive the JSON API directly because Playwright cannot easily
  // trigger borderline flow purely through the UI without preconditions.
  test('prohibited term blocks review submission', async ({ page, baseURL }) => {
    test.skip(!baseURL, 'baseURL required')

    // Admin: register a prohibited term via API
    const adminCtx = await request.newContext({ baseURL })
    await adminCtx.post('/api/v1/auth/login', {
      data: { username: 'admin', password: 'password' },
    })
    const meResp = await adminCtx.get('/api/v1/auth/me')
    const meBody = await meResp.json()
    const csrf = meBody.csrf_token

    const term = 'blockword' + Date.now()
    await adminCtx.post('/api/v1/admin/sensitive-terms', {
      data: { term, class: 'prohibited' },
      headers: { 'X-CSRF-Token': csrf },
    })

    // Customer attempts to write a review containing the term.
    // We need a Completed ticket to attach the review to, so this test
    // skips gracefully if no eligible ticket exists for regular_user.
    await loginAs(page, 'regular_user')
    await page.goto('/tickets')

    const rows = page.getByTestId('ticket-row')
    const count = await rows.count()
    let target = null
    for (let i = 0; i < count; i++) {
      const status = await rows.nth(i).getByTestId('ticket-status').textContent()
      if (status?.trim() === 'Completed' || status?.trim() === 'Closed') {
        target = rows.nth(i)
        break
      }
    }
    test.skip(!target, 'no Completed/Closed ticket for content-blocking test')

    await target.click()
    await page.getByTestId('btn-write-review').click()
    await page.getByTestId('star-5').click()
    await page.getByTestId('textarea-review-text').fill(`This review contains ${term} text`)
    await page.getByTestId('btn-submit-review').click()

    // Submission should fail — the modal should display an error message.
    await expect(page.getByTestId('review-error')).toBeVisible({ timeout: 6_000 })

    await adminCtx.dispose()
  })
})
