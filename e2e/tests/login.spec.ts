import { expect, test } from './helpers';

// Fresh, logged-out context. Uses 2 of the limiter's 5 attempts per minute.
test.use({ storageState: { cookies: [], origins: [] } });

test('redirects to login, rejects a bad password, then logs in and out', async ({ page }) => {
  const email = process.env.E2E_EMAIL ?? '';
  const password = process.env.E2E_PASSWORD ?? '';

  await page.goto('/todos');
  await expect(page).toHaveURL(/\/login\?next=%2Ftodos/);

  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill('not-the-password');
  await page.getByRole('button', { name: 'Login' }).click();
  await expect(page.getByText('Incorrect email or password.')).toBeVisible();

  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Login' }).click();
  await expect(page).toHaveURL(/\/todos$/);
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();

  await page.getByRole('button', { name: 'Log out' }).click();
  await expect(page).toHaveURL(/\/login/);
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
});
