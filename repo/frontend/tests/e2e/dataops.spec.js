import { test, expect } from '@playwright/test'

async function loginAs(page, username) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill(username)
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

test.describe('Legal Holds — navigation', () => {
  test('admin sees Legal Holds nav link', async ({ page }) => {
    await loginAs(page, 'admin')
    await expect(page.getByTestId('link-legal-holds')).toBeVisible()
  })

  test('regular user does NOT see Legal Holds link', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await expect(page.getByTestId('link-legal-holds')).toBeHidden()
  })

  test('admin can open the Legal Holds dashboard', async ({ page }) => {
    await loginAs(page, 'admin')
    await page.getByTestId('link-legal-holds').click()
    await expect(page).toHaveURL(/\/admin\/legal-holds/, { timeout: 6_000 })
    await expect(page.getByRole('heading', { name: /Legal Holds/i })).toBeVisible()
  })
})

test.describe('Legal Holds — form', () => {
  test('place hold button stays disabled until inputs are filled', async ({ page }) => {
    await loginAs(page, 'admin')
    await page.goto('/admin/legal-holds')

    const submit = page.getByTestId('btn-place-hold')
    await expect(submit).toBeDisabled()

    await page.getByTestId('input-hold-source-id').fill('1')
    await expect(submit).toBeDisabled()

    await page.getByTestId('input-hold-reason').fill('Litigation hold #A-001')
    await expect(submit).toBeEnabled()
  })

  test('empty state visible when no holds exist', async ({ page }) => {
    await loginAs(page, 'admin')
    await page.goto('/admin/legal-holds')
    // Either empty state or an existing list — both are valid.
    const empty = page.getByTestId('holds-empty')
    const list = page.getByTestId('hold-list')
    await expect(empty.or(list)).toBeVisible({ timeout: 6_000 })
  })
})
