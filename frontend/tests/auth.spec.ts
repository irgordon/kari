import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

const email = 'owner@example.test'
const password = 'Correct-Horse-42!'

async function signIn(page: Page) {
  await page.goto('/login')
  await page.getByLabel('Email address').fill(email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible()
}

test.describe('real frontend, API, and PostgreSQL', () => {
  test.describe.configure({ mode: 'serial' })
  test.beforeEach(({ page }) => {
    page.on('response', response => {
      if (response.status() >= 429) console.warn(`HTTP ${response.status()}: ${new URL(response.url()).pathname}`)
    })
  })

  test('blank installation creates one administrator and enters the application', async ({ page }) => {
    await page.goto('/')
    await expect(page).toHaveURL(/\/setup$/)
    await page.getByLabel('Setup token').fill('invalid-token')
    await page.getByLabel('Email address').fill(email)
    await page.getByLabel('Password', { exact: true }).fill(password)
    await page.getByLabel('Confirm password').fill(password)
    await page.getByRole('button', { name: 'Create administrator', exact: true }).click()
    await expect(page.getByRole('alert')).toContainText('Invalid or expired setup token')
    await page.getByLabel('Setup token').fill(process.env.KARI_E2E_SETUP_TOKEN ?? '')
    await page.getByLabel('Confirm password').fill('Mismatch-Password1!')
    await page.getByRole('button', { name: 'Create administrator', exact: true }).click()
    await expect(page.getByRole('alert')).toContainText('Passwords do not match')
    await page.getByLabel('Confirm password').fill(password)
    await page.getByRole('button', { name: 'Create administrator', exact: true }).click()
    await expect(page).toHaveURL(/\/login\?setup=complete$/)
    await expect(page.getByRole('status')).toContainText('Administrator created')
    await page.getByLabel('Email address').fill(email)
    await page.getByLabel('Password', { exact: true }).fill('Incorrect-Password1!')
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
    await expect(page.getByRole('alert')).toContainText('Invalid credentials')
    await page.getByLabel('Password', { exact: true }).fill(password)
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible()
    await page.reload()
    await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible()
    await page.goto('/setup')
    await expect(page).toHaveURL(/\/$/)
    const complete = await page.request.post('/api/v1/setup/finalize', {
      headers: { Origin: new URL(page.url()).origin, 'X-Setup-Token': process.env.KARI_E2E_SETUP_TOKEN ?? '' },
      data: { admin_email: 'replacement@example.test', admin_password: password },
    })
    expect(complete.status()).toBe(409)
  })

  test('missing access cookie refreshes once, rotates cookies, and rejects replay', async ({ page, context }) => {
    await signIn(page)
    const before = await context.cookies()
    const refresh = before.find(cookie => cookie.name === 'kari_refresh_token')!
    expect(refresh.httpOnly).toBe(true)
    expect(refresh.sameSite).toBe('Strict')
    expect(await page.evaluate(() => document.cookie)).not.toContain('kari_')
    await context.clearCookies({ name: 'kari_access_token' })
    const renewal = page.waitForResponse(response => response.url().endsWith('/auth/refresh') && response.status() === 200)
    await page.reload()
    await renewal
    await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible()
    expect((await context.cookies()).find(cookie => cookie.name === 'kari_refresh_token')!.value).not.toBe(refresh.value)
    // Rejection must leave the browser's current session intact.
    const replay = await page.request.post('/api/v1/auth/refresh', {
      headers: { Origin: new URL(page.url()).origin, Cookie: `kari_refresh_token=${refresh.value}` },
    })
    expect(replay.status()).toBe(401)
    expect((await page.request.get('/api/v1/auth/me')).status()).toBe(200)
  })

  test('two tabs restore a shared session without racing refresh rotation', async ({ page, context }) => {
    await signIn(page)
    const second = await context.newPage()
    await second.goto('/')
    await expect(second.getByRole('button', { name: 'Sign out' })).toBeVisible()
    await context.clearCookies({ name: 'kari_access_token' })
    await Promise.all([page.reload(), second.reload()])
    await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible()
    await expect(second.getByRole('button', { name: 'Sign out' })).toBeVisible()
  })

  test('logout revokes captured credentials and protected navigation returns to login', async ({ page, context }) => {
    await signIn(page)
    const cookies = await context.cookies()
    const captured = cookies.map(cookie => `${cookie.name}=${cookie.value}`).join('; ')
    const origin = new URL(page.url()).origin
    const foreign = await page.request.post('/api/v1/auth/logout', { headers: { Origin: 'https://attacker.example' } })
    expect(foreign.status()).toBe(403)
    await page.getByRole('button', { name: 'Sign out' }).click()
    await expect(page).toHaveURL(/\/login$/)
    const access = await page.request.get('/api/v1/auth/me', { headers: { Cookie: captured } })
    expect(access.status()).toBe(401)
    const refresh = await page.request.post('/api/v1/auth/refresh', { headers: { Origin: origin, Cookie: captured } })
    expect(refresh.status()).toBe(401)
    await page.goto('/settings')
    await expect(page).toHaveURL(/\/login$/)
    await expect(page.getByRole('button', { name: 'Sign out' })).toHaveCount(0)
  })

  test('protected navigation recovers from a real temporary read-rate limit', async ({ page }) => {
    let status = 0
    for (let attempt = 0; attempt < 80 && status !== 429; attempt++) {
      status = (await page.request.get('/api/v1/auth/me')).status()
    }
    expect(status).toBe(429)
    await page.goto('/settings')
    await expect(page).toHaveURL(/\/login$/)
    await expect(page.getByRole('heading', { name: 'Sign in to Karı' })).toBeVisible()
  })
})
