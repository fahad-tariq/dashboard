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
import type { Page } from '@playwright/test';

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

test('external edit to ideas.md refreshes /ideas and the homepage but not /todos', async ({ context, page }) => {
  const title = uniqueTitle('Idea from an editor');
  const open = async (p: Page, path: string): Promise<void> => {
    const connected = waitForSseConnection(p);
    await p.goto(path);
    await connected;
    await waitForSseSettle(p);
  };
  const isRefresh = (url: string, headers: Record<string, string>, path: string): boolean =>
    new URL(url).pathname === path && headers['hx-request'] === 'true';

  const ideas = page;
  const home = await context.newPage();
  const todos = await context.newPage();
  await open(ideas, '/ideas');
  await open(home, '/');
  await open(todos, '/todos');

  let todosRefreshes = 0;
  todos.on('request', (r) => {
    if (isRefresh(r.url(), r.headers(), '/todos')) todosRefreshes++;
  });
  const homeRefresh = home.waitForResponse((r) => isRefresh(r.url(), r.request().headers(), '/'));

  appendLine(userFile('ideas.md'), `- [ ] ${title} [status: untriaged] [added: 2026-10-01]`);

  await expect(ideas.locator('.ideas-page .tracker-item', { hasText: title })).toBeVisible({ timeout: 10_000 });
  await homeRefresh;
  // Give /todos as long again to refresh, which it must not.
  await todos.waitForTimeout(1500);
  expect(todosRefreshes).toBe(0);
});

/** Resolves once a live refresh of path has been fetched. */
function refreshed(page: Page, path: string): Promise<unknown> {
  return page.waitForResponse(
    (r) => new URL(r.url()).pathname === path && r.request().method() === 'GET' && r.request().headers()['hx-request'] === 'true',
  );
}

test('external edit while a tracker item is expanded appears without collapsing it', async ({ page }) => {
  const title = uniqueTitle('Keep me open');
  const external = uniqueTitle('Arrives while open');
  const item = await addTask(page, title, { body: 'Body stays visible' });
  await expandTrackerItem(item);

  appendLine(userFile('personal.md'), `- [ ] ${external}`);

  await expect(trackerItem(page, external)).toBeVisible({ timeout: 10_000 });
  await expect(item).not.toHaveClass(/\bminimised\b/);
  await expect(item.locator('.item-toggle')).toHaveAttribute('aria-expanded', 'true');
  await expect(item.locator('.tracker-item-body')).toBeVisible();
});

test('open completion note on /house keeps its text through an SSE refresh', async ({ page }) => {
  const external = uniqueTitle('Added while typing');
  const connected = waitForSseConnection(page);
  await page.goto('/house');
  await connected;
  await waitForSseSettle(page);

  const row = page.locator('tr.house-row-maint', { hasText: 'Clean gutters' });
  await row.getByRole('button', { name: 'done' }).click();
  const note = row.getByLabel('Completion note');
  await note.fill('half typed');

  appendLine(sharedFile('maintenance.md'), `- [ ] ${external} [cadence: 1y]`);

  // The refresh is applied, and the note keeps its popover and its text.
  await expect(page.locator('tr.house-row-maint', { hasText: external })).toBeVisible({ timeout: 10_000 });
  await expect(note).toBeVisible();
  await expect(note).toHaveValue('half typed');
  await expect(note).toBeFocused();
});

test('external edit while a plan item is expanded keeps it expanded on the homepage', async ({ page }) => {
  const title = uniqueTitle('Stay expanded');
  const external = uniqueTitle('Picker gains me');
  const task = await addTask(page, title, { body: 'Detail that should stay visible' });
  await task.getByTitle('Do today').click();
  await expect(trackerItem(page, title).getByTitle('Do today')).toHaveCount(0);

  const connected = waitForSseConnection(page);
  await page.goto('/');
  await connected;
  await waitForSseSettle(page);
  const item = planItem(page, title);
  await expandPlanItem(item);

  const refresh = refreshed(page, '/');
  appendLine(userFile('personal.md'), `- [ ] ${external}`);
  await refresh;

  // The refresh was applied (the new task is in the picker) and the plan
  // row is still open, and not draggable while open.
  await expect(page.locator('.plan-pick-item', { hasText: external })).toHaveCount(1);
  await expect(planItem(page, title)).not.toHaveClass(/\bminimised\b/);
  await expect(planItem(page, title).locator('.plan-item-body')).toBeVisible();
  await expect(planItem(page, title)).toHaveAttribute('draggable', 'false');
});

test('select mode, ticked rows and the bulk bar survive an external edit', async ({ page }) => {
  const first = await addTask(page, uniqueTitle('Select keep A'));
  await addTask(page, uniqueTitle('Select keep B'));
  const title = uniqueTitle('Arrives while selecting');

  await page.getByRole('button', { name: 'select', exact: true }).click();
  await first.locator('.bulk-checkbox').check();
  const bar = page.locator('#bulk-bar');
  await expect(bar).toHaveClass(/\bvisible\b/);
  await expect(page.locator('#bulk-bar-count')).toHaveText('1 selected');

  const refresh = refreshed(page, '/todos');
  appendLine(userFile('personal.md'), `- [ ] ${title} [added: 2026-10-01]`);
  await refresh;

  await expect(trackerItem(page, title)).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('.tracker-page')).toHaveClass(/\bselect-mode\b/);
  await expect(page.locator('#select-toggle')).toHaveText('cancel');
  await expect(first.locator('.bulk-checkbox')).toBeChecked();
  await expect(bar).toHaveClass(/\bvisible\b/);
  await expect(page.locator('#bulk-bar-count')).toHaveText('1 selected');
});
