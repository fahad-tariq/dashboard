import {
  addTask,
  appendLine,
  expandPlanItem,
  expandTrackerItem,
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
  await expandTrackerItem(item);

  const refresh = page.waitForResponse((r) => new URL(r.url()).pathname === '/todos' && r.request().headers()['hx-request'] === 'true');
  appendLine(userFile('personal.md'), `- [ ] ${external}`);
  await refresh;

  // The refresh was fetched but not swapped in.
  await expect(item).not.toHaveClass(/\bminimised\b/);
  await expect(trackerItem(page, external)).toHaveCount(0);
  await page.reload();
  await expect(trackerItem(page, external)).toBeVisible();
});

test('open completion note on /house survives an SSE refresh', async ({ page }) => {
  const external = uniqueTitle('Added while typing');
  const connected = waitForSseConnection(page);
  await page.goto('/house');
  await connected;
  await waitForSseSettle(page);

  const row = page.locator('tr.house-row-maint', { hasText: 'Clean gutters' });
  await row.getByRole('button', { name: 'done' }).click();
  const note = row.getByLabel('Completion note');
  await note.fill('half typed');

  const refresh = page.waitForResponse((r) => new URL(r.url()).pathname === '/house' && r.request().headers()['hx-request'] === 'true');
  appendLine(sharedFile('maintenance.md'), `- [ ] ${external} [cadence: 1y]`);
  await refresh;

  // The refresh was fetched but not swapped in, so the note is still there.
  await expect(note).toHaveValue('half typed');
  await expect(page.locator('tr.house-row-maint', { hasText: external })).toHaveCount(0);
});

test('expanded plan item suppresses the SSE refresh on the homepage', async ({ page }) => {
  const title = uniqueTitle('Stay expanded');
  const task = await addTask(page, title, { body: 'Detail that should stay visible' });
  await task.getByTitle('Do today').click();
  await expect(trackerItem(page, title).getByTitle('Do today')).toHaveCount(0);

  const connected = waitForSseConnection(page);
  await page.goto('/');
  await connected;
  await waitForSseSettle(page);
  const item = planItem(page, title);
  await expandPlanItem(item);

  const refresh = page.waitForResponse((r) => new URL(r.url()).pathname === '/' && r.request().headers()['hx-request'] === 'true');
  appendLine(sharedFile('family.md'), `- [ ] ${uniqueTitle('Family external')}`);
  await refresh;
  await page.waitForTimeout(500);

  await expect(planItem(page, title)).not.toHaveClass(/\bminimised\b/);
});
