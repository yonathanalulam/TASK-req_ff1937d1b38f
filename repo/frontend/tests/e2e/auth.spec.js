import { test, expect } from '@playwright/test'

test.describe('Authentication', () => {
  test('login page renders required fields', async ({ page }) => {
    await page.goto('/login')
    await expect(page.getByTestId('input-username')).toBeVisible()
    await expect(page.getByTestId('input-password')).toBeVisible()
    await expect(page.getByTestId('btn-login')).toBeVisible()
  })

  test('inline validation fires on empty submit', async ({ page }) => {
    await page.goto('/login')
    await page.getByTestId('btn-login').click()

    // Both fields should show inline errors
    await expect(page.locator('.field.error')).toHaveCount(2)
  })

  test('invalid credentials shows api error', async ({ page }) => {
    await page.goto('/login')
    await page.getByTestId('input-username').fill('nonexistent_user')
    await page.getByTestId('input-password').fill('WrongPass1')
    await page.getByTestId('btn-login').click()

    await expect(page.getByTestId('api-error')).toBeVisible({ timeout: 8_000 })
    const text = await page.getByTestId('api-error').textContent()
    expect(text).toBeTruthy()
  })

  test('unauthenticated user redirected from dashboard to login', async ({ page }) => {
    await page.goto('/dashboard')
    await expect(page).toHaveURL(/\/login/)
  })

  test('login with valid seed credentials succeeds', async ({ page }) => {
    // Seeded admin user — password is "password" (bcrypt hash inserted via SQL seed)
    await page.goto('/login')
    await page.getByTestId('input-username').fill('admin')
    await page.getByTestId('input-password').fill('password')
    await page.getByTestId('btn-login').click()

    // Should redirect to dashboard
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
    await expect(page.locator('.nav-user')).toBeVisible()
  })

  test('logout clears session and redirects to login', async ({ page }) => {
    // Login first
    await page.goto('/login')
    await page.getByTestId('input-username').fill('admin')
    await page.getByTestId('input-password').fill('password')
    await page.getByTestId('btn-login').click()
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })

    // Logout
    await page.locator('.btn-logout').click()
    await expect(page).toHaveURL(/\/login/, { timeout: 5_000 })
  })
})
