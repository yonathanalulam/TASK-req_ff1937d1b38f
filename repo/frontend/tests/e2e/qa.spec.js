import { test, expect } from '@playwright/test'

async function loginAs(page, username) {
  await page.goto('/login')
  await page.getByTestId('input-username').fill(username)
  await page.getByTestId('input-password').fill('password')
  await page.getByTestId('btn-login').click()
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 })
}

async function openFirstOfferingQA(page) {
  await page.goto('/catalog')
  const cards = page.getByTestId('card-offering')
  await expect(cards.first()).toBeVisible({ timeout: 10_000 })
  await cards.first().click()
  await page.getByTestId('link-qa').click()
  await expect(page).toHaveURL(/\/catalog\/\d+\/qa/, { timeout: 6_000 })
}

test.describe('Q&A — navigation', () => {
  test('Q&A page is reachable from the offering detail', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await openFirstOfferingQA(page)
    await expect(page.getByRole('heading', { name: /Questions & Answers/i })).toBeVisible()
  })
})

test.describe('Q&A — regular user', () => {
  test('regular user can post a question', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await openFirstOfferingQA(page)

    await page.getByTestId('btn-ask-question').click()
    await page.getByTestId('textarea-question').fill('Do you work weekends?')
    await page.getByTestId('btn-submit-question').click()

    // Thread list should show the new question
    await expect(page.getByTestId('qa-thread-question').first())
      .toContainText('Do you work weekends?', { timeout: 6_000 })
  })

  test('regular user does NOT see Reply button', async ({ page }) => {
    await loginAs(page, 'regular_user')
    await openFirstOfferingQA(page)

    // Make sure there's at least one thread first
    const threads = page.getByTestId('qa-thread-item')
    if ((await threads.count()) === 0) {
      await page.getByTestId('btn-ask-question').click()
      await page.getByTestId('textarea-question').fill('Test question for reply check')
      await page.getByTestId('btn-submit-question').click()
      await expect(threads.first()).toBeVisible({ timeout: 6_000 })
    }
    await expect(page.getByTestId('btn-reply')).toHaveCount(0)
  })
})

test.describe('Q&A — service agent', () => {
  test('service agent can reply to a question', async ({ page, context }) => {
    // Make sure at least one question exists — post as regular_user first
    const page1 = page
    await loginAs(page1, 'regular_user')
    await openFirstOfferingQA(page1)

    await page1.getByTestId('btn-ask-question').click()
    await page1.getByTestId('textarea-question').fill('Agent test question')
    await page1.getByTestId('btn-submit-question').click()
    await expect(page1.getByTestId('qa-thread-question').first())
      .toBeVisible({ timeout: 6_000 })

    // Now log out and log in as service_agent
    const agentPage = await context.newPage()
    await loginAs(agentPage, 'service_agent')
    await openFirstOfferingQA(agentPage)

    const replyBtns = agentPage.getByTestId('btn-reply')
    await expect(replyBtns.first()).toBeVisible({ timeout: 6_000 })

    await replyBtns.first().click()
    await agentPage.getByTestId('textarea-reply').fill('Yes, with notice.')
    await agentPage.getByTestId('btn-submit-reply').click()

    await expect(agentPage.getByTestId('qa-post-item').last())
      .toContainText('Yes, with notice.', { timeout: 6_000 })
  })
})

test.describe('Q&A — moderator', () => {
  test('moderator sees delete button on posts', async ({ page }) => {
    await loginAs(page, 'moderator')
    await openFirstOfferingQA(page)

    // Need at least one post to test against
    const posts = page.getByTestId('qa-post-item')
    const count = await posts.count()
    test.skip(count === 0, 'no posts available for moderator delete test')

    await expect(page.getByTestId('btn-delete-post').first()).toBeVisible()
  })
})
