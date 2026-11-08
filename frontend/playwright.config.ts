import { defineConfig } from '@playwright/test'

if (!process.env.KARI_E2E_URL) throw new Error('Run make topology-smoke to supply the disposable application stack')

export default defineConfig({
  testDir: './tests',
  workers: 1,
  retries: 0,
  timeout: 45_000,
  forbidOnly: true,
  reporter: 'list',
  outputDir: '../.artifacts/playwright',
  use: { baseURL: process.env.KARI_E2E_URL, browserName: 'chromium', trace: 'off' },
})
