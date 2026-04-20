import { test, expect } from '@playwright/test'

async function loginAs(page, username) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill(username)
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

test.describe('HMAC Keys — navigation', () => {
  test('admin sees HMAC Keys nav link', async ({ page }) => {
    await loginAs(page, 'admin')
    await expect(page.getByTestId('link-hmac-keys')).toBeVisible()
  })

  test('regular user does NOT see HMAC Keys link', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await expect(page.getByTestId('link-hmac-keys')).toBeHidden()
  })

  test('admin can open the HMAC Keys view', async ({ page }) => {
    await loginAs(page, 'admin')
    await page.getByTestId('link-hmac-keys').click()
    await expect(page).toHaveURL(/\/admin\/hmac-keys/, { timeout: 6_000 })
    await expect(page.getByRole('heading', { name: /HMAC Signing Keys/i })).toBeVisible()
  })
})

test.describe('HMAC Keys — route role gating', () => {
  // /admin/hmac-keys is administrator-only. A regular_user that types the URL
  // directly must be redirected away so they never load the management UI.
  test('regular_user redirected away from /admin/hmac-keys', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await page.goto('/admin/hmac-keys')
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 6_000 })
  })
})

test.describe('HMAC Keys — create flow', () => {
  test('create button stays disabled until key_id is filled', async ({ page }) => {
    await loginAs(page, 'admin')
    await page.goto('/admin/hmac-keys')

    const submit = page.getByTestId('btn-create-key')
    await expect(submit).toBeDisabled()

    await page.getByTestId('input-new-key-id').fill('e2e-test-key')
    await expect(submit).toBeEnabled()
  })

  test('creating a key reveals the secret exactly once', async ({ page }) => {
    await loginAs(page, 'admin')
    await page.goto('/admin/hmac-keys')

    // Use a unique key_id per run so repeated runs don't collide on UNIQUE.
    const keyId = `e2e-${Date.now().toString(36)}`

    await page.getByTestId('input-new-key-id').fill(keyId)
    await page.getByTestId('btn-create-key').click()

    // Reveal banner appears with the plaintext secret.
    const reveal = page.getByTestId('reveal-card')
    await expect(reveal).toBeVisible({ timeout: 6_000 })

    const secret = await page.getByTestId('reveal-secret').innerText()
    // 32 random bytes → 64 hex characters.
    expect(secret).toMatch(/^[0-9a-f]{64}$/)

    // Key row appears in the table.
    await expect(page.getByTestId(`key-row-${keyId}`)).toBeVisible()

    // Dismissing the reveal hides the secret and it's gone for good.
    await page.getByTestId('btn-dismiss-reveal').click()
    await expect(reveal).toBeHidden()
  })

  test('rotating a key surfaces a different secret', async ({ page }) => {
    await loginAs(page, 'admin')
    await page.goto('/admin/hmac-keys')

    // Prime: create a new key, capture its secret.
    const keyId = `e2e-rot-${Date.now().toString(36)}`
    await page.getByTestId('input-new-key-id').fill(keyId)
    await page.getByTestId('btn-create-key').click()
    await expect(page.getByTestId('reveal-card')).toBeVisible({ timeout: 6_000 })
    const original = await page.getByTestId('reveal-secret').innerText()
    await page.getByTestId('btn-dismiss-reveal').click()

    // Rotate via the row action. Playwright auto-accepts the confirm() prompt.
    page.once('dialog', (d) => d.accept())
    await page.getByTestId(`btn-rotate-${keyId}`).click()

    await expect(page.getByTestId('reveal-card')).toBeVisible({ timeout: 6_000 })
    const rotated = await page.getByTestId('reveal-secret').innerText()
    expect(rotated).toMatch(/^[0-9a-f]{64}$/)
    expect(rotated).not.toBe(original)
  })
})
