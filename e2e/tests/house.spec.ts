import { expect, test, uniqueTitle, waitForSseSettle } from './helpers';

test('add a maintenance item and log a completion', async ({ page }) => {
  const title = uniqueTitle('Flush hot water tank');

  await page.goto('/house');
  await page.locator('details.tracker-add-form > summary', { hasText: 'Add maintenance item' }).click();
  const form = page.locator('form[action="/house/maintenance/add"]');
  await form.getByLabel('Item name').fill(title);
  await form.getByLabel('Cadence').fill('2w');
  await form.getByRole('button', { name: 'add' }).click();

  // Never done, so it starts overdue.
  const row = page.locator('tr.house-row-maint', { hasText: title });
  await expect(row).toHaveClass(/house-row-overdue/);
  await expect(row.locator('.badge-overdue')).toHaveText('overdue');

  await row.getByRole('button', { name: 'done' }).click();
  await row.getByLabel('Completion note').fill('drained and refilled');
  await row.getByRole('button', { name: 'log' }).click();

  await expect(page).toHaveURL(/\/house/);
  const loggedRow = page.locator('tr.house-row-maint', { hasText: title });
  await expect(loggedRow).not.toHaveClass(/house-row-overdue/);
  await expect(loggedRow.locator('.badge-status-due')).toBeVisible();

  await waitForSseSettle(page);
  await loggedRow.click();
  await expect(loggedRow).toHaveAttribute('aria-expanded', 'true');
  const detail = loggedRow.locator('xpath=following-sibling::tr[1]');
  await expect(detail).not.toHaveClass(/house-detail-hidden/);
  await expect(detail).toContainText('every 2w');
  await expect(detail).toContainText('1 entry');
  await detail.locator('summary', { hasText: 'View log history' }).click();
  await expect(detail.locator('.house-log-note')).toContainText('drained and refilled');
});

test('seeded maintenance item shows its log and cadence', async ({ page }) => {
  await page.goto('/house');
  const row = page.locator('tr.house-row-maint', { hasText: 'Clean gutters' });
  await expect(row).toBeVisible();
  await row.click();
  const detail = row.locator('xpath=following-sibling::tr[1]');
  await expect(detail).toContainText('every 6m');
  await expect(detail).toContainText('1 entry');
  await expect(page.locator('tr.house-row-proj', { hasText: 'Paint the back fence' })).toBeVisible();
});
