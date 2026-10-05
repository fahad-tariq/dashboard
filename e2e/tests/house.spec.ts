import { expect, expectNoReload, markNoReload, test, uniqueTitle, waitForSseSettle } from './helpers';

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

  await markNoReload(page);
  await row.getByRole('button', { name: 'done' }).click();
  await row.getByLabel('Completion note').fill('drained and refilled');
  await row.getByRole('button', { name: 'log' }).click();

  await expect(page).toHaveURL(/\/house/);
  await expect(page.locator('#toast')).toBeVisible();
  const loggedRow = page.locator('tr.house-row-maint', { hasText: title });
  await expect(loggedRow).not.toHaveClass(/house-row-overdue/);
  await expect(loggedRow.locator('.badge-status-due')).toBeVisible();
  // The logged completion closes the note popover, and the page never reloaded.
  await expect(loggedRow.getByLabel('Completion note')).toBeHidden();
  await expectNoReload(page);

  await waitForSseSettle(page);
  const toggle = loggedRow.locator('button.house-row-toggle');
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-expanded', 'true');
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
  // A click anywhere on the row still toggles it, as a mouse convenience.
  await row.locator('td').nth(1).click();
  await expect(row.locator('button.house-row-toggle')).toHaveAttribute('aria-expanded', 'true');
  const detail = row.locator('xpath=following-sibling::tr[1]');
  await expect(detail).toContainText('every 6m');
  await expect(detail).toContainText('1 entry');
  await expect(page.locator('tr.house-row-proj', { hasText: 'Paint the back fence' })).toBeVisible();
});
