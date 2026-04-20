import { test, expect } from '@playwright/test'

test.describe('Health Check', () => {
  test('app loads without JS errors', async ({ page }) => {
    const errors = []
    page.on('pageerror', (err) => errors.push(err.message))

    await page.goto('/')
    await page.waitForLoadState('networkidle')

    expect(errors).toHaveLength(0)
  })

  test('/health page displays backend status', async ({ page }) => {
    await page.goto('/health')
    await page.waitForLoadState('networkidle')

    // Wait for status card to render (loading resolves)
    const statusEl = page.getByTestId('health-status')
    await expect(statusEl).toBeVisible({ timeout: 10_000 })

    const text = await statusEl.textContent()
    expect(text).toContain('ok')
  })

  test('nav contains "Service Portal" brand', async ({ page }) => {
    await page.goto('/')
    await expect(page.locator('.brand')).toHaveText('Service Portal')
  })
})
