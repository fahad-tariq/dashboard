import {
  addTask,
  expandTrackerItem,
  expect,
  test,
  trackerItem,
  trackerSection,
  uniqueTitle,
} from './helpers';

test('add, complete, move to trash and restore a task', async ({ page }) => {
  const doneTitle = uniqueTitle('Water the herbs');
  await addTask(page, doneTitle, { tags: 'garden' });

  // Complete: the item leaves the open list and appears under Done.
  await trackerItem(page, doneTitle).getByTitle('Complete').click();
  await expect(trackerItem(page, doneTitle)).toHaveCount(0);
  const done = trackerSection(page, /^Done \(\d+\)/);
  await done.locator('summary').click();
  await expect(done.locator('.tracker-item', { hasText: doneTitle })).toBeVisible();

  // Trash via the item's actions panel, which asks for confirmation first.
  const trashTitle = uniqueTitle('Return library books');
  const item = await addTask(page, trashTitle);
  await expandTrackerItem(item);
  await item.getByRole('button', { name: 'trash' }).click();
  const modal = page.locator('#confirm-modal');
  await expect(modal).toHaveClass(/\bvisible\b/);
  await expect(modal.getByRole('heading')).toHaveText('Move this item to trash?');
  await modal.getByRole('button', { name: 'Confirm' }).click();

  await expect(page).toHaveURL(/\/todos/);
  await expect(trackerItem(page, trashTitle)).toHaveCount(0);
  const deleted = trackerSection(page, /^Recently Deleted \(\d+\)/);
  await deleted.locator('summary').click();
  const deletedItem = deleted.locator('.tracker-item', { hasText: trashTitle });
  await expect(deletedItem).toBeVisible();

  // Restore puts it back in the open list.
  await deletedItem.getByTitle('Restore').click();
  await expect(trackerItem(page, trashTitle)).toBeVisible();
  await expect(trackerSection(page, /^Recently Deleted/).locator('.tracker-item', { hasText: trashTitle })).toHaveCount(0);
});

test('add, toggle and remove a sub-step', async ({ page }) => {
  const title = uniqueTitle('Service the bike');
  const item = await addTask(page, title);
  await expandTrackerItem(item);

  // Sub-step forms swap only this item via htmx; the page does not reload.
  await page.evaluate(() => {
    (window as unknown as { __e2eNoReload: boolean }).__e2eNoReload = true;
  });

  await item.getByLabel('Add a sub-step').fill('Pump the tyres');
  await item.getByRole('button', { name: 'add', exact: true }).click();
  const step = item.locator('.substep-item', { hasText: 'Pump the tyres' });
  await expect(step).toBeVisible();
  await expect(step).not.toHaveClass(/substep-item-done/);
  await expect(item.locator('.badge-substeps')).toHaveText('0/1');

  await step.getByTitle('Toggle').click();
  await expect(item.locator('.substep-item', { hasText: 'Pump the tyres' })).toHaveClass(/substep-item-done/);
  await expect(item.locator('.badge-substeps')).toHaveText('1/1');

  await item.locator('.substep-item', { hasText: 'Pump the tyres' }).getByTitle('Remove').click();
  await expect(item.locator('.substep-item')).toHaveCount(0);
  await expect(item.locator('.badge-substeps')).toHaveCount(0);

  // The item stays expanded across the swaps and the page never navigated.
  await expect(item).not.toHaveClass(/\bminimised\b/);
  expect(await page.evaluate(() => (window as unknown as { __e2eNoReload?: boolean }).__e2eNoReload)).toBe(true);
});

test('seeded sub-steps render with progress', async ({ page }) => {
  await page.goto('/todos');
  const item = trackerItem(page, 'Plan weekend hike');
  await expect(item.locator('.badge-substeps')).toHaveText('1/2');
  await expandTrackerItem(item);
  await expect(item.locator('.substep-item-done', { hasText: 'Pick a trail' })).toBeVisible();
  await expect(item.locator('.tracker-item-body')).toHaveText('Somewhere within two hours of Sydney.');
});
