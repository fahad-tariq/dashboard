import {
  addTask,
  appendLine,
  expect,
  planItem,
  sharedFile,
  test,
  trackerItem,
  uniqueTitle,
  userFile,
  waitForSseConnection,
  waitForSseSettle,
} from './helpers';

type MarkedWindow = Window & { __e2eNoReload?: boolean };

test('external edit to personal.md appears on /todos without a reload', async ({ page }) => {
  const title = uniqueTitle('Added in an editor');

  const connected = waitForSseConnection(page);
  await page.goto('/todos');
  await connected;
  await waitForSseSettle(page);
  await page.evaluate(() => {
    (window as MarkedWindow).__e2eNoReload = true;
  });

  appendLine(userFile('personal.md'), `- [ ] ${title} [added: 2026-10-01] [tags: external]`);

  await expect(trackerItem(page, title)).toBeVisible({ timeout: 10_000 });
  await expect(trackerItem(page, title).locator('.badge-tag')).toHaveText('external');
  expect(await page.evaluate(() => (window as MarkedWindow).__e2eNoReload)).toBe(true);
});

test('expanded tracker item suppresses the SSE refresh', async ({ page }) => {
  const title = uniqueTitle('Keep me open');
  const external = uniqueTitle('Hidden until reload');
  const item = await addTask(page, title);
  await waitForSseSettle(page);
  await item.locator('.tracker-item-header').click();
  await expect(item).not.toHaveClass(/\bminimised\b/);

  const refresh = page.waitForResponse((r) => new URL(r.url()).pathname === '/todos' && r.request().headers()['hx-request'] === 'true');
  appendLine(userFile('personal.md'), `- [ ] ${external}`);
  await refresh;

  // The refresh was fetched but not swapped in.
  await expect(item).not.toHaveClass(/\bminimised\b/);
  await expect(trackerItem(page, external)).toHaveCount(0);
  await page.reload();
  await expect(trackerItem(page, external)).toBeVisible();
});

// CLAUDE.md says planDetailExpanded (and planDragInProgress) suppress SSE swaps,
// but tracker.js only suppresses swaps whose target is .tracker-page, so the
// homepage re-renders and collapses the expanded item.
test.fixme('expanded plan item suppresses the SSE refresh on the homepage', async ({ page }) => {
  const title = uniqueTitle('Stay expanded');
  const task = await addTask(page, title, { body: 'Detail that should stay visible' });
  await task.getByTitle('Do today').click();
  await expect(trackerItem(page, title).getByTitle('Do today')).toHaveCount(0);

  const connected = waitForSseConnection(page);
  await page.goto('/');
  await connected;
  await waitForSseSettle(page);
  const item = planItem(page, title);
  await item.locator('.plan-item-title').click();
  await expect(item).not.toHaveClass(/\bminimised\b/);

  const refresh = page.waitForResponse((r) => new URL(r.url()).pathname === '/' && r.request().headers()['hx-request'] === 'true');
  appendLine(sharedFile('family.md'), `- [ ] ${uniqueTitle('Family external')}`);
  await refresh;
  await page.waitForTimeout(500);

  await expect(planItem(page, title)).not.toHaveClass(/\bminimised\b/);
});
