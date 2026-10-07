import { addTask, expandTrackerItem, expect, planItem, test, trackerItem, uniqueTitle, waitForSseSettle } from './helpers';

/** A YYYY-MM-DD date `days` from today in the server's timezone (run.sh). */
function serverDate(days: number): string {
  const d = new Date(Date.now() + days * 24 * 60 * 60 * 1000);
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Australia/Sydney' }).format(d);
}

test('a due date shows on the row and in Due soon, whose plan today keeps focus', async ({ page }) => {
  const title = uniqueTitle('Renew the rego');
  const task = await addTask(page, title);

  await expandTrackerItem(task);
  await task.locator('details.tracker-notes-edit > summary').click();
  const form = task.locator('form.tracker-notes-form');
  await form.getByLabel('Due', { exact: true }).fill(serverDate(1));
  await form.getByRole('button', { name: 'save' }).click();
  await expect(trackerItem(page, title).locator('.badge-due > [aria-hidden="true"]')).toHaveText(/^due /);
  await waitForSseSettle(page);

  await page.goto('/');
  await waitForSseSettle(page);
  const row = page.locator('.homepage-card .homepage-task').filter({ hasText: title });
  await expect(row.locator('.widget-meta')).toHaveText(/^due /);
  await expect(planItem(page, title)).toHaveCount(0);

  const button = row.getByRole('button', { name: `Plan ${title} for today` });
  await button.focus();
  await page.keyboard.press('Enter');
  await expect(planItem(page, title)).toBeVisible();
  await expect(row.locator('.badge-planned')).toHaveText('planned');
  await expect(button).toHaveCount(0);
  // The button is gone, so focus moves to the row's link rather than the page.
  await expect(row.getByRole('link', { name: title })).toBeFocused();
});
