import {
  addTask,
  expandPlanItem,
  expandTrackerItem,
  expect,
  itemIdPattern,
  planFromPicker,
  planItem,
  test,
  trackerItem,
  trackerSection,
  uniqueTitle,
  waitForSseSettle,
} from './helpers';

test('plan a task from the picker and complete it on the homepage', async ({ page }) => {
  const title = uniqueTitle('Post the parcel');
  await addTask(page, title);

  const item = await planFromPicker(page, title);
  await expect(item).not.toHaveClass(/plan-item-done/);
  await expect(page.locator('.plan-progress-text')).toHaveText(/^\d+ of \d+ done$/);

  await item.getByRole('button', { name: `done ${title}` }).click();
  await expect(planItem(page, title)).toHaveClass(/plan-item-done/);
  await expect(planItem(page, title).getByRole('button', { name: `done ${title}` })).toHaveCount(0);

  // Completion is written through to the underlying task list.
  await page.goto('/todos');
  await expect(trackerItem(page, title)).toHaveCount(0);
  const done = trackerSection(page, /^Done \(\d+\)/);
  await done.locator('summary').click();
  await expect(done.locator('.tracker-item', { hasText: title })).toBeVisible();
});

test('expand and collapse a plan item planned from the todo list', async ({ page }) => {
  const title = uniqueTitle('Call the plumber');
  const task = await addTask(page, title, { body: 'Ask about the hot water system', tags: 'house' });
  const rowId = await task.getAttribute('id');
  expect(rowId).toMatch(new RegExp(`^item-${itemIdPattern}$`));

  // "do today" in the expanded row on /todos plans the task for today.
  await expandTrackerItem(task);
  await task.getByTitle('Do today').click();
  await expect(trackerItem(page, title).getByTitle('Do today')).toHaveCount(0);

  await page.goto('/');
  const item = planItem(page, title);
  await expect(item).toHaveClass(/\bminimised\b/);
  await expect(item.locator('.plan-item-body')).toBeHidden();
  await waitForSseSettle(page);

  const toggle = item.getByRole('button', { name: title, exact: true });
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await expandPlanItem(item);
  await expect(item).toHaveAttribute('draggable', 'false');
  const detail = page.locator(`[id="${await toggle.getAttribute('aria-controls')}"]`);
  await expect(detail).toBeVisible();
  await expect(item.locator('.plan-item-body')).toHaveText('Ask about the hot water system');
  await expect(item.locator('.plan-item-tags .badge-tag')).toHaveText('house');
  const link = item.getByRole('link', { name: 'open in list' });
  await expect(link).toBeVisible();
  await expect(link).toHaveAttribute('href', `/todos#${rowId}`);

  await toggle.click();
  await expect(item).toHaveClass(/\bminimised\b/);
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await expect(item).toHaveAttribute('draggable', 'true');
});

test('reorder plan items with the arrow buttons', async ({ page }) => {
  const first = uniqueTitle('Reorder first');
  const second = uniqueTitle('Reorder second');
  await addTask(page, first);
  await addTask(page, second);
  await planFromPicker(page, first);
  await planFromPicker(page, second);

  const personalTitles = () =>
    page.locator('.plan-today-tasks .plan-item[data-list="todos"] .plan-item-title').allTextContents();
  const moveUp = () => planItem(page, second).getByRole('button', { name: `Move ${second} up` });
  const moveDown = () => planItem(page, second).getByRole('button', { name: `Move ${second} down` });

  // The arrows sit in the expanded row on every pointer type, not only
  // coarse ones. A row keeps its expanded state through refreshes.
  await page.goto('/');
  await waitForSseSettle(page);
  await expandPlanItem(planItem(page, second));
  await expect(moveUp()).toBeVisible();

  // Bubble `second` to the top of the personal list, one POST per click.
  for (let i = 0; i < 20; i++) {
    const titles = await personalTitles();
    if (titles[0] === second) break;
    await Promise.all([
      page.waitForResponse((r) => new URL(r.url()).pathname === '/plan/reorder' && r.ok()),
      moveUp().click(),
    ]);
    await expect(page.locator('#announcer')).toContainText(`Moved ${second} to position`);
    await waitForSseSettle(page);
  }

  // Order is persisted via [plan-order: N], so it survives a reload.
  await page.reload();
  expect((await personalTitles())[0]).toBe(second);
  await expandPlanItem(planItem(page, second));

  await Promise.all([
    page.waitForResponse((r) => new URL(r.url()).pathname === '/plan/reorder' && r.ok()),
    moveDown().click(),
  ]);
  await expect(page.locator('#announcer')).toContainText(`Moved ${second} to position 2 of`);
  await waitForSseSettle(page);
  await page.reload();
  const titles = await personalTitles();
  expect(titles[1]).toBe(second);
  expect(titles[0]).not.toBe(second);
});
