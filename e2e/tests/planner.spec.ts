import {
  addTask,
  expect,
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
  await expect(page.locator('.plan-progress-text')).toHaveText(/^\d+\/\d+ done$/);

  await item.getByRole('button', { name: 'done' }).click();
  await expect(planItem(page, title)).toHaveClass(/plan-item-done/);
  await expect(planItem(page, title).getByRole('button', { name: 'done' })).toHaveCount(0);

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

  // The star button on /todos plans the task for today.
  await task.getByTitle('Do today').click();
  await expect(trackerItem(page, title).getByTitle('Do today')).toHaveCount(0);

  await page.goto('/');
  const item = planItem(page, title);
  await expect(item).toHaveClass(/\bminimised\b/);
  await expect(item.locator('.plan-item-body')).toBeHidden();
  await waitForSseSettle(page);

  await item.locator('.plan-item-title').click();
  await expect(item).not.toHaveClass(/\bminimised\b/);
  await expect(item).toHaveAttribute('aria-expanded', 'true');
  await expect(item).toHaveAttribute('draggable', 'false');
  await expect(item.locator('.plan-item-body')).toHaveText('Ask about the hot water system');
  await expect(item.locator('.plan-item-tags .badge-tag')).toHaveText('house');
  const link = item.getByRole('link', { name: 'open in list' });
  await expect(link).toBeVisible();
  await expect(link).toHaveAttribute('href', /^\/todos#item-call-the-plumber-/);

  await item.locator('.plan-item-title').click();
  await expect(item).toHaveClass(/\bminimised\b/);
  await expect(item).toHaveAttribute('aria-expanded', 'false');
  await expect(item).toHaveAttribute('draggable', 'true');
});

test.describe('reorder buttons', () => {
  // The up/down buttons are only shown under `@media (pointer: coarse)`.
  // Chromium reports a coarse pointer when touch emulation is on.
  test.use({ hasTouch: true });

  test('reorder plan items with the arrow buttons', async ({ page }) => {
    const first = uniqueTitle('Reorder first');
    const second = uniqueTitle('Reorder second');
    await addTask(page, first);
    await addTask(page, second);
    await planFromPicker(page, first);
    await planFromPicker(page, second);

    const personalTitles = () =>
      page.locator('.plan-today-tasks .plan-item[data-list="todos"] .plan-item-title').allTextContents();

    await page.goto('/');
    expect(await page.evaluate(() => window.matchMedia('(pointer: coarse)').matches)).toBe(true);
    await waitForSseSettle(page);
    const moveUp = planItem(page, second).getByTitle('Move up');
    await expect(moveUp).toBeVisible();

    // Bubble `second` to the top of the personal list, one POST per click.
    for (let i = 0; i < 20; i++) {
      const titles = await personalTitles();
      if (titles[0] === second) break;
      await Promise.all([
        page.waitForResponse((r) => new URL(r.url()).pathname === '/plan/reorder' && r.ok()),
        planItem(page, second).getByTitle('Move up').click(),
      ]);
      await waitForSseSettle(page);
    }

    // Order is persisted via [plan-order: N], so it survives a reload.
    await page.reload();
    expect((await personalTitles())[0]).toBe(second);

    await Promise.all([
      page.waitForResponse((r) => new URL(r.url()).pathname === '/plan/reorder' && r.ok()),
      planItem(page, second).getByTitle('Move down').click(),
    ]);
    await waitForSseSettle(page);
    await page.reload();
    const titles = await personalTitles();
    expect(titles[1]).toBe(second);
    expect(titles[0]).not.toBe(second);
  });
});
