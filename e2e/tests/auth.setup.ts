import { expect, test as setup } from '@playwright/test';
import { authFile } from './paths';

// Logs in once and saves the session cookie for every other spec. The login
// limiter allows 5 attempts per minute per IP, so specs should not re-login.
setup('authenticate', async ({ page }) => {
  const email = process.env.E2E_EMAIL;
  const password = process.env.E2E_PASSWORD;
  if (!email || !password) throw new Error('E2E_EMAIL and E2E_PASSWORD must be set; run via e2e/run.sh');

  await page.goto('/login');
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Login' }).click();
  await expect(page).toHaveURL(/\/$/);
  await page.context().storageState({ path: authFile });
});
