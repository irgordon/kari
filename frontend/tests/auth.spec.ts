import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import { fileURLToPath } from 'node:url'

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

test.describe('email capability confirmation', () => {
  test.beforeAll(async ({ request, baseURL }) => {
    if (!process.env.COMPOSE_PROJECT_NAME?.startsWith('kari-smoke-') || new URL(baseURL!).hostname !== '127.0.0.1') throw new Error('Email fixtures require a disposable local topology-smoke stack')
    const status = await request.get('/api/v1/setup/status')
    if (!(await status.json() as { configured: boolean }).configured) {
      const setup = await request.post('/api/v1/setup/finalize', {
        headers: { Origin: baseURL!, 'X-Setup-Token': process.env.KARI_E2E_SETUP_TOKEN ?? '' },
        data: { admin_email: email, admin_password: password },
      })
      expect(setup.status()).toBe(201)
    }
  })
  test('opening a link keeps its token out of the URL and waits for confirmation', async ({ page }) => {
    let redemptions = 0
    page.on('request', request => {
      if (request.method() === 'POST' && request.url().endsWith('/auth/email/magic')) redemptions++
    })
    await page.goto(`/auth/email/magic#token=${'a'.repeat(43)}`)
    await expect(page.getByRole('heading', { name: 'Confirm sign-in' })).toBeVisible()
    await expect(page).toHaveURL(/\/auth\/email\/magic$/)
    expect(redemptions).toBe(0)
    await page.getByRole('button', { name: 'Confirm sign-in' }).click()
    await expect(page.getByRole('alert')).toContainText('invalid, expired, already used')
    expect(redemptions).toBe(1)
  })

  test('verified links sign in and reset the password through the real frontend', async ({ page, context }) => {
    const seed = (purpose: 'verify' | 'magic' | 'reset') => {
      const token = randomBytes(32).toString('base64url')
      const hash = createHash('sha256').update(token).digest('hex')
      const compose = fileURLToPath(new URL('../../scripts/compose.sh', import.meta.url))
      const container = execFileSync(compose, ['ps', '-q', 'db'], { encoding: 'utf8' }).trim()
      // Controlled fixture constants and a hex hash enter only the disposable database.
      execFileSync('docker', ['exec', '-i', container, 'psql', '-v', 'ON_ERROR_STOP=1', '-U', 'kari', '-d', 'kari'], {
        input: `INSERT INTO account_email_tokens (user_id, purpose, token_hash, credential_version, expires_at)
          SELECT id, '${purpose}', '${hash}', credential_version, NOW() + INTERVAL '15 minutes' FROM users WHERE email = '${email}'
          ON CONFLICT (user_id, purpose) DO UPDATE SET token_hash = EXCLUDED.token_hash, credential_version = EXCLUDED.credential_version, expires_at = EXCLUDED.expires_at, consumed_at = NULL;`,
        stdio: ['pipe', 'ignore', 'pipe'],
      })
      return token
    }
    await signIn(page)
    await page.goto(`/auth/email/verify#token=${seed('verify')}`)
    await page.getByRole('button', { name: 'Verify email', exact: true }).click()
    await expect(page).toHaveURL(/\/account$/)
    await expect(page.getByText('Email verified', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Sign out' }).click()
    await expect(page).toHaveURL(/\/login$/)
    await page.goto(`/auth/email/magic#token=${seed('magic')}`)
    await page.getByRole('button', { name: 'Confirm sign-in', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible()
    const captured = (await context.cookies()).map(cookie => `${cookie.name}=${cookie.value}`).join('; ')
    await page.goto(`/auth/email/reset#token=${seed('reset')}`)
    await page.getByLabel('New password', { exact: true }).fill('Replacement-Password-84!')
    await page.getByLabel('Confirm new password').fill('Replacement-Password-84!')
    await page.getByRole('button', { name: 'Update password', exact: true }).click()
    await expect(page.getByRole('status')).toContainText('Password updated')
    expect((await page.request.get('/api/v1/auth/me', { headers: { Cookie: captured } })).status()).toBe(401)
    await page.getByRole('link', { name: 'Return to sign-in' }).click()
    await page.getByLabel('Email address').fill(email)
    await page.getByLabel('Password', { exact: true }).fill('Replacement-Password-84!')
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible()
  })
})
