import {
  addTask,
  expandTrackerItem,
  expect,
  test,
  trackerItem,
  trackerSection,
  uniqueTitle,
  waitForSseSettle,
} from './helpers';

test('confirm modal cancels, closes on Escape and confirms a permanent delete', async ({ page }) => {
  const title = uniqueTitle('Cancel gym membership');
  const item = await addTask(page, title);
  const modal = page.locator('#confirm-modal');

  // Cancel leaves the item untouched.
  await expandTrackerItem(item);
  await item.getByRole('button', { name: 'trash' }).click();
  await expect(modal).toHaveClass(/\bvisible\b/);
  await expect(page.locator('#confirm-modal-ok')).toBeFocused();
  await modal.getByRole('button', { name: 'Cancel' }).click();
  await expect(modal).not.toHaveClass(/\bvisible\b/);
  await expect(trackerItem(page, title)).toBeVisible();

  // Confirm moves it to trash.
  await item.getByRole('button', { name: 'trash' }).click();
  await modal.getByRole('button', { name: 'Confirm' }).click();
  await expect(trackerItem(page, title)).toHaveCount(0);
  await waitForSseSettle(page);

  const deleted = trackerSection(page, /^Recently Deleted \(\d+\)/);
  await deleted.locator('summary').click();
  const deletedItem = deleted.locator('.tracker-item', { hasText: title });

  // Permanent delete is flagged as destructive: warning shown, danger button.
  await deletedItem.getByTitle('Permanently delete').click();
  await expect(modal).toHaveClass(/\bvisible\b/);
  await expect(modal.getByRole('heading')).toHaveText('Permanently delete this item? This cannot be undone.');
  await expect(page.locator('#confirm-modal-warning')).toBeVisible();
  await expect(page.locator('#confirm-modal-ok')).toHaveClass(/confirm-btn-danger/);

  await page.keyboard.press('Escape');
  await expect(modal).not.toHaveClass(/\bvisible\b/);
  await expect(deletedItem).toBeVisible();

  await deletedItem.getByTitle('Permanently delete').click();
  await modal.getByRole('button', { name: 'Confirm' }).click();
  await expect(page).toHaveURL(/\/todos/);
  await expect(page.locator('.tracker-item', { hasText: title })).toHaveCount(0);
});
