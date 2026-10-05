import type { Page, Request } from '@playwright/test';
import {
  activeElementId,
  addTask,
  expandPlanItem,
  expandTrackerItem,
  expect,
  expectNoReload,
  markNoReload,
  planFromPicker,
  planItem,
  test,
  trackerItem,
  trackerSection,
  uniqueTitle,
  waitForSseSettle,
} from './helpers';

/** Records the page's requests, except the SSE stream and the footer's MCP poll. */
function recordRequests(page: Page): Request[] {
  const seen: Request[] = [];
  page.on('request', (r) => {
    const path = new URL(r.url()).pathname;
    if (path !== '/events' && !path.startsWith('/mcp/')) seen.push(r);
  });
  return seen;
}

test('one mutation is one request: no redirect GET and no SSE echo refresh', async ({ page }) => {
  const title = uniqueTitle('Single request');
  await addTask(page, title);
  await waitForSseSettle(page);

  const requests = recordRequests(page);
  await trackerItem(page, title).getByRole('button', { name: `Complete ${title}` }).click();
  await expect(trackerItem(page, title)).toHaveCount(0);
  // Longer than the 500ms event debounce plus a refresh round trip.
  await page.waitForTimeout(1500);

  const described = requests.map((r) => `${r.method()} ${new URL(r.url()).pathname}`);
  expect(described.filter((d) => d.startsWith('POST ')), described.join(', ')).toHaveLength(1);
  expect(described.filter((d) => d.startsWith('GET ')), described.join(', ')).toEqual([]);
});

test('tracker actions update in place: priority, edit, complete, uncomplete, trash, undo, restore', async ({ page }) => {
  const title = uniqueTitle('In-place actions');
  const item = await addTask(page, title);
  await markNoReload(page);
  await expandTrackerItem(item);

  // Priority: the row stays expanded through the morph.
  await item.getByLabel('Priority', { exact: true }).selectOption('high');
  await item.getByRole('button', { name: 'apply' }).click();
  await expect(item.locator('.tracker-item-meta .badge-priority-high')).toBeVisible();
  await expect(item).not.toHaveClass(/\bminimised\b/);

  // Edit the notes.
  await item.locator('details.tracker-notes-edit > summary').click();
  await item.locator('.tracker-notes-form').getByLabel('Notes').fill('Edited in place');
  await item.locator('.tracker-notes-form').getByRole('button', { name: 'save' }).click();
  await expect(item.locator('.tracker-item-body')).toHaveText('Edited in place');
  await expect(page.locator('#toast .toast-text')).toHaveText('Changes saved.');

  // Complete, then reopen from Done.
  await item.getByRole('button', { name: `Complete ${title}` }).click();
  await expect(trackerItem(page, title)).toHaveCount(0);
  const done = trackerSection(page, /^Done \(\d+\)/);
  await done.locator('summary').click();
  await done.getByRole('button', { name: `Mark ${title} not done` }).click();
  await expect(trackerItem(page, title)).toBeVisible();

  // Trash, undo from the toast.
  await expandTrackerItem(trackerItem(page, title));
  await trackerItem(page, title).getByRole('button', { name: 'trash' }).click();
  await expect(trackerItem(page, title)).toHaveCount(0);
  await page.locator('#toast').getByRole('button', { name: 'undo' }).click();
  await expect(trackerItem(page, title)).toBeVisible();
  await expect(page.locator('#toast .toast-text')).toHaveText('Item restored from trash.');

  // Trash again and restore from Recently Deleted.
  await expandTrackerItem(trackerItem(page, title));
  await trackerItem(page, title).getByRole('button', { name: 'trash' }).click();
  await expect(trackerItem(page, title)).toHaveCount(0);
  const deleted = trackerSection(page, /^Recently Deleted \(\d+\)/);
  await deleted.locator('summary').click();
  await deleted.getByRole('button', { name: `Restore ${title}` }).click();
  await expect(trackerItem(page, title)).toBeVisible();

  await expectNoReload(page);
});

test('plan actions update in place: set, complete and drop', async ({ page }) => {
  const doneTitle = uniqueTitle('Plan complete');
  const dropTitle = uniqueTitle('Plan drop');
  await addTask(page, doneTitle);
  await addTask(page, dropTitle);
  await planFromPicker(page, doneTitle);
  await markNoReload(page);
  await planFromPicker(page, dropTitle);

  await planItem(page, doneTitle).getByRole('button', { name: `done ${doneTitle}` }).click();
  await expect(planItem(page, doneTitle)).toHaveClass(/plan-item-done/);

  await expandPlanItem(planItem(page, dropTitle));
  await planItem(page, dropTitle).getByRole('button', { name: `drop ${dropTitle}` }).click();
  await expect(planItem(page, dropTitle)).toHaveCount(0);
  await expect(page.locator('.plan-pick-item', { hasText: dropTitle })).toHaveCount(1);
});

test('undo toast stays at least 10s, pauses on hover, and "u" undoes', async ({ page }) => {
  test.setTimeout(90_000);
  const title = uniqueTitle('Toast timing');
  const item = await addTask(page, title);
  const toast = page.locator('#toast');
  const restore = async () => {
    const deleted = trackerSection(page, /^Recently Deleted \(\d+\)/);
    if (!(await deleted.evaluate((d) => (d as HTMLDetailsElement).open))) await deleted.locator('summary').click();
    await deleted.getByRole('button', { name: `Restore ${title}` }).click();
    await expect(trackerItem(page, title)).toBeVisible();
    await expandTrackerItem(trackerItem(page, title));
  };

  // Left alone, it stays at least 10s, then goes.
  await page.mouse.move(0, 0);
  await expandTrackerItem(item);
  await item.getByRole('button', { name: 'trash' }).click();
  await expect(toast.getByRole('button', { name: 'undo' })).toBeVisible();
  await page.waitForTimeout(9_000);
  await expect(toast).toBeVisible();
  await expect(toast).toBeHidden({ timeout: 5_000 });

  // Hovered from the start, it stays well past 10s; leaving starts a fresh 10s.
  await restore();
  await trackerItem(page, title).getByRole('button', { name: 'trash' }).click();
  await expect(toast).toBeVisible();
  await toast.hover();
  await page.waitForTimeout(12_000);
  await expect(toast).toBeVisible();
  await page.mouse.move(0, 0);
  await expect(toast).toBeHidden({ timeout: 12_000 });

  // Keyboard: "u" undoes while the toast offers it.
  await expect(trackerItem(page, title)).toHaveCount(0);
  await restore();
  await trackerItem(page, title).getByRole('button', { name: 'trash' }).click();
  await expect(toast.getByRole('button', { name: 'undo' })).toBeVisible();
  await expect(page.locator('#announcer')).toContainText('Press U to undo');
  await page.keyboard.press('u');
  await expect(trackerItem(page, title)).toBeVisible();
});

test('keyboard: completing a row moves focus to the next row, an emptied list to its page heading', async ({ page }) => {
  const first = uniqueTitle('Focus first');
  const second = uniqueTitle('Focus second');
  await addTask(page, first);
  await addTask(page, second);

  const firstId = await trackerItem(page, first).getAttribute('id');
  // The row focus should land on: the next open row, or the previous one if
  // this row is last.
  const expected = await page.evaluate((id) => {
    const rows = Array.from(document.querySelectorAll('.tracker-page > .tracker-item[data-row]'));
    const i = rows.findIndex((r) => r.id === id);
    return (rows[i + 1] ?? rows[i - 1]).id;
  }, firstId);

  await trackerItem(page, first).getByRole('button', { name: `Complete ${first}` }).focus();
  await page.keyboard.press('Enter');
  await expect(trackerItem(page, first)).toHaveCount(0);
  await expect.poll(() => activeElementId(page)).toBe(`${expected}-toggle`);

  // An idea moved out of the Dropped section, its only card, leaves the
  // section empty, so focus goes to the page heading.
  const idea = uniqueTitle('Focus idea');
  await page.goto('/ideas');
  await waitForSseSettle(page);
  await page.locator('details.tracker-add-form > summary', { hasText: 'Add idea' }).click();
  const form = page.locator('form[action="/ideas/add"]');
  await form.getByLabel('Idea title').fill(idea);
  await form.getByRole('button', { name: 'add idea' }).click();
  const card = () => page.locator('.ideas-page .tracker-item', { hasText: idea });
  await expect(card()).toBeVisible();
  await expandTrackerItem(card());
  await card().getByRole('button', { name: 'drop' }).click();
  await expect(page.locator('#ideas-dropped').locator('.tracker-item', { hasText: idea })).toBeVisible();
  await waitForSseSettle(page);

  test.skip((await page.locator('#ideas-dropped .tracker-item').count()) !== 1, 'Dropped holds other ideas');
  await expandTrackerItem(card());
  await card().getByRole('button', { name: 'park' }).focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('#ideas-dropped')).toHaveCount(0);
  await expect(page.locator('#page-heading')).toBeFocused();
});

test('search is a combobox: arrows set the active option, Escape returns focus', async ({ page }) => {
  await page.goto('/todos');
  await waitForSseSettle(page);
  const opener = page.locator('#select-toggle');
  await opener.focus();
  await page.keyboard.press('/');

  const overlay = page.locator('#search-overlay');
  const input = page.getByRole('combobox', { name: 'Search tasks, ideas, and house items' });
  await expect(overlay).toHaveAttribute('open');
  await expect(input).toBeFocused();
  await input.fill('passport');
  // Scoped: the page's own <select>s hold options too.
  const option = page.getByRole('listbox', { name: 'Search results' }).getByRole('option').first();
  await expect(option).toBeVisible();
  await expect(input).toHaveAttribute('aria-expanded', 'true');

  await input.press('ArrowDown');
  const optionId = await option.getAttribute('id');
  expect(optionId).toBe('search-result-0');
  await expect(input).toHaveAttribute('aria-activedescendant', optionId as string);
  await expect(option).toHaveAttribute('aria-selected', 'true');

  await input.press('Escape');
  await expect(overlay).not.toHaveAttribute('open');
  await expect(opener).toBeFocused();
});

test('a failed idea triage shows an error toast and leaves the card', async ({ page }) => {
  await page.goto('/ideas');
  await waitForSseSettle(page);
  await markNoReload(page);
  await page.route('**/ideas/*/triage', (route) =>
    route.fulfill({ status: 400, contentType: 'text/plain', body: 'Item not found\n' }),
  );

  const card = page.locator('#ideas-untriaged .tracker-item', { hasText: 'Home weather station' });
  await expandTrackerItem(card);
  await card.getByRole('button', { name: 'park' }).click();

  const toast = page.locator('#toast');
  await expect(toast).toHaveClass(/toast-error/);
  await expect(toast.locator('.toast-text')).toHaveText('Item not found');
  await expect(card).toBeVisible();
  await expect(card).not.toHaveClass(/idea-transitioning/);
  await expect(card.getByRole('button', { name: 'park' })).toBeEnabled();
  await expectNoReload(page);
});

test('text typed in a field without focus survives another row\'s action', async ({ page }) => {
  const keep = uniqueTitle('Keep my typing');
  const done = uniqueTitle('Complete me instead');
  await addTask(page, keep);
  await addTask(page, done);
  await markNoReload(page);

  const keepRow = trackerItem(page, keep);
  await expandTrackerItem(keepRow);
  const step = keepRow.getByRole('textbox', { name: 'Add a sub-step' });
  await step.fill('half-typed step');

  // Completing another row moves focus away and morphs the whole list.
  await trackerItem(page, done).getByRole('button', { name: `Complete ${done}` }).click();
  await expect(trackerItem(page, done)).toHaveCount(0);

  await expect(step).toHaveValue('half-typed step');
  await expectNoReload(page);
});
