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

test('untick a done task on the plan keeps focus on its circle', async ({ page }) => {
  const title = uniqueTitle('Water the plants');
  await addTask(page, title);
  const item = await planFromPicker(page, title);

  await item.getByRole('button', { name: `done ${title}` }).click();
  await expect(planItem(page, title)).toHaveClass(/plan-item-done/);
  await waitForSseSettle(page);

  await planItem(page, title).getByRole('button', { name: `Mark ${title} not done` }).click();
  await expect(planItem(page, title)).not.toHaveClass(/plan-item-done/);
  await expect(planItem(page, title).getByRole('button', { name: `done ${title}` })).toBeFocused();
  // Reopening offers no undo, so the toast shows no undo button.
  await expect(page.locator('#toast .toast-text')).toHaveText('Reopened.');
  await expect(page.locator('#toast .toast-undo')).toBeHidden();
});

test('ticking a task on the plan can be undone from the toast', async ({ page }) => {
  const title = uniqueTitle('Book the car service');
  await addTask(page, title);
  const item = await planFromPicker(page, title);

  await item.getByRole('button', { name: `done ${title}` }).click();
  await expect(planItem(page, title)).toHaveClass(/plan-item-done/);
  await page.locator('#toast .toast-undo').click();
  await expect(planItem(page, title)).not.toHaveClass(/plan-item-done/);
});

test('bulk select on the plan spans lists: tomorrow and complete', async ({ page }) => {
  const mine = uniqueTitle('Return the library books');
  const shared = uniqueTitle('Book the school photos');
  const other = uniqueTitle('Top up the travel card');
  await addTask(page, mine);
  await addTask(page, shared, { list: 'family' });
  await addTask(page, other);
  for (const title of [mine, shared, other]) await planFromPicker(page, title);

  const toggle = page.locator('#select-toggle');
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await expect(planItem(page, mine).locator('.plan-tick')).toBeHidden();
  await page.getByLabel(`Select ${mine}`).check();
  // A click on the row selects it in select mode, rather than expanding it.
  await planItem(page, shared).locator('.plan-item-title').click();
  await expect(planItem(page, shared)).toHaveClass(/\bminimised\b/);
  await expect(page.locator('#bulk-bar-count')).toHaveText('2 selected');

  await page.locator('#bulk-bar').getByRole('button', { name: 'tomorrow' }).click();
  await expect(page.locator('#flash')).toHaveText('2 tasks moved to tomorrow.');
  await expect(planItem(page, mine)).toHaveCount(0);
  await expect(planItem(page, shared)).toHaveCount(0);
  await expect(planItem(page, other)).toBeVisible();

  await page.locator('#select-toggle').click();
  await page.getByLabel(`Select ${other}`).check();
  await page.locator('#bulk-bar').getByRole('button', { name: 'complete' }).click();
  await expect(page.locator('#flash')).toHaveText('1 task done.');
  await expect(planItem(page, other)).toHaveClass(/plan-item-done/);
});

test('bulk drop and trash on the plan', async ({ page }) => {
  const dropped = uniqueTitle('Sort the recycling');
  const trashed = uniqueTitle('Cancel the old gym');
  await addTask(page, dropped);
  await addTask(page, trashed);
  for (const title of [dropped, trashed]) await planFromPicker(page, title);

  await page.locator('#select-toggle').click();
  await page.getByLabel(`Select ${dropped}`).check();
  await page.locator('#bulk-bar').getByRole('button', { name: 'drop' }).click();
  await expect(page.locator('#flash')).toHaveText('1 task removed from the plan.');
  await expect(planItem(page, dropped)).toHaveCount(0);

  await page.locator('#select-toggle').click();
  await page.getByLabel(`Select ${trashed}`).check();
  await page.locator('#bulk-bar').getByRole('button', { name: 'trash' }).click();
  await page.locator('#confirm-modal-ok').click();
  await expect(page.locator('#flash')).toHaveText('1 task moved to trash.');
  await expect(planItem(page, trashed)).toHaveCount(0);

  await page.goto('/todos');
  await expect(trackerItem(page, dropped)).toBeVisible();
  await expect(trackerItem(page, trashed)).toHaveCount(0);
});
