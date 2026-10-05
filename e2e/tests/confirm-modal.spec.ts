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

test('confirm dialog cancels, closes on Escape and confirms a permanent delete', async ({ page }) => {
  const title = uniqueTitle('Cancel gym membership');
  const item = await addTask(page, title);
  const modal = page.locator('#confirm-modal');

  // Moving to the other list still asks; Cancel leaves the item untouched.
  await expandTrackerItem(item);
  await item.getByRole('button', { name: 'move to family' }).click();
  await expect(modal).toHaveAttribute('open');
  await expect(modal.getByRole('heading')).toHaveText('Move to family?');
  // Only permanent actions carry the warning and the danger button.
  await expect(page.locator('#confirm-modal-warning')).toBeHidden();
  await expect(page.locator('#confirm-modal-ok')).not.toHaveClass(/confirm-btn-danger/);
  await expect(page.locator('#confirm-modal-ok')).toBeFocused();
  await modal.getByRole('button', { name: 'Cancel' }).click();
  await expect(modal).not.toHaveAttribute('open');
  await expect(trackerItem(page, title)).toBeVisible();

  // Trash needs no confirmation: it can be undone from the toast.
  await item.getByRole('button', { name: 'trash' }).click();
  await expect(modal).not.toHaveAttribute('open');
  await expect(trackerItem(page, title)).toHaveCount(0);
  await expect(page.locator('#toast .toast-undo')).toBeVisible();
  await waitForSseSettle(page);

  const deleted = trackerSection(page, /^Recently Deleted \(\d+\)/);
  await deleted.locator('summary').click();
  const deletedItem = deleted.locator('.tracker-item', { hasText: title });

  // Permanent delete is flagged as destructive: warning shown, danger button.
  await deletedItem.getByTitle('Permanently delete').click();
  await expect(modal).toHaveAttribute('open');
  await expect(modal.getByRole('heading')).toHaveText('Permanently delete this item? This cannot be undone.');
  await expect(page.locator('#confirm-modal-warning')).toBeVisible();
  await expect(page.locator('#confirm-modal-ok')).toHaveClass(/confirm-btn-danger/);

  await page.keyboard.press('Escape');
  await expect(modal).not.toHaveAttribute('open');
  await expect(deletedItem).toBeVisible();

  await deletedItem.getByTitle('Permanently delete').click();
  await modal.getByRole('button', { name: 'Confirm' }).click();
  await expect(modal).not.toHaveAttribute('open');
  await expect(page).toHaveURL(/\/todos/);
  await expect(page.locator('.tracker-item', { hasText: title })).toHaveCount(0);
});
