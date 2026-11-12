import { test, expect } from '@playwright/test'

// Pages reached without a session: the link in a password-reset mail is opened
// by someone who is, by definition, not signed in.
//
// The root layout used to fire authenticated requests there too. Their 401 ran
// the session-expiry handler, which cleared the query cache and so cancelled
// the page's own token check mid-flight: the page stayed blank. It only showed
// when the check was slower than the 401, as on a real deployment, so the check
// is held back here to make the race deterministic.
test('a reset link opened without a session shows its page', async ({ page }) => {
  await page.route('**/api/password-reset/**', async (route) => {
    const response = await route.fetch()
    await new Promise((resolve) => setTimeout(resolve, 500))
    await route.fulfill({ response })
  })

  const unauthorized: string[] = []
  page.on('response', (response) => {
    const path = new URL(response.url()).pathname
    // /auth/me answering 401 is how the app learns nobody is signed in.
    if (response.status() === 401 && path !== '/api/auth/me') unauthorized.push(path)
  })

  // No token is valid here; a well-formed one that matches nothing is enough to
  // prove the page renders its answer instead of nothing.
  await page.goto(`/reset-password/${'0'.repeat(64)}`)
  await expect(page.getByText('Invalid or Expired Link')).toBeVisible()
  expect(unauthorized).toEqual([])
})
