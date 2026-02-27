import { test, expect } from '@playwright/test';

test('setup page loads and db url is empty', async ({ page }) => {
  await page.goto('http://localhost:5173/setup');

  // Wait for the step 2 (Database) to be reachable/visible.
  // Initially step is 1. We might need to mock API responses or just check initial state if possible.
  // The issue description says the variable is initialized.
  // Let's see if we can check the default state of the component.
  // Actually, we need to click through step 1 to get to step 2.
  // Step 1 requires 'testMuscle' to pass. We might need to mock that.

  // Mocking the API responses
  await page.route('/api/v1/setup/test-muscle*', async route => {
    const json = { healthy: true, version: '0.0.1', cpu: '10%', ram_mb: 1024 };
    await route.fulfill({ json });
  });

  // Navigate to setup
  await page.goto('http://localhost:5173/setup');

  // Verify we are on step 1
  await expect(page.getByText('Hardware & Muscle')).toBeVisible();

  // Wait for the "Continue" button to be enabled (it depends on muscleStatus.healthy)
  const continueBtnStep1 = page.getByRole('button', { name: 'Continue' });
  await expect(continueBtnStep1).toBeEnabled();
  await continueBtnStep1.click();

  // Now we should be on step 2
  await expect(page.getByText('Persistence & Networking')).toBeVisible();

  // Check the DB URL input
  const dbInput = page.getByLabel('Database Connection');
  await expect(dbInput).toBeVisible();

  // It should be empty now
  await expect(dbInput).toHaveValue('');

  // It should have the placeholder
  await expect(dbInput).toHaveAttribute('placeholder', 'postgres://user:pass@host:5432/dbname?sslmode=disable');

  // Take a screenshot
  await page.screenshot({ path: 'frontend_verification/setup_step_2.png' });
});
