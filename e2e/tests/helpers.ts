import { appendFileSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test, type Locator, type Page } from '@playwright/test';

export { expect, test };

/** Title with a per-run suffix so tests never collide on shared server state. */
export function uniqueTitle(prefix: string): string {
  return `${prefix} ${Date.now().toString(36)}${Math.floor(Math.random() * 1e4)}`;
}

export function dataDir(): string {
  const dir = process.env.E2E_DATA_DIR;
  if (!dir) throw new Error('E2E_DATA_DIR is not set; run the suite via e2e/run.sh');
  return dir;
}

/** Per-user files live under USER_DATA_DIR/<id>/; the auto-created admin is user 1. */
export function userFile(name: 'personal.md' | 'ideas.md'): string {
  return join(dataDir(), 'users', '1', name);
}

export function sharedFile(name: 'family.md' | 'maintenance.md' | 'house-projects.md'): string {
  return join(dataDir(), name);
}

/** Appends a line to a markdown file on disk, as an external editor would. */
export function appendLine(path: string, line: string): void {
  const current = readFileSync(path, 'utf8');
  const prefix = current.endsWith('\n') ? '' : '\n';
  appendFileSync(path, `${prefix}${line}\n`);
}

/**
 * Every write to a watched markdown file (including the app's own) triggers an
 * SSE `file-changed` event after a 500ms debounce, which outerHTML-swaps the
 * page container. The homepage and house page do not suppress that swap, so
 * transient UI state (an expanded row) is lost if the swap lands after we set
 * it. Waiting out the debounce plus a round trip avoids racing it.
 */
export async function waitForSseSettle(page: Page): Promise<void> {
  await page.waitForTimeout(1500);
}

/** Resolves once the htmx SSE stream for the current page has connected. */
export function waitForSseConnection(page: Page): Promise<unknown> {
  return page.waitForResponse((r) => new URL(r.url()).pathname === '/events');
}

export function trackerItem(page: Page, title: string): Locator {
  return page
    .locator('.tracker-page > .tracker-item')
    .filter({ has: page.locator('.tracker-item-title', { hasText: title }) });
}

export function trackerSection(page: Page, summary: RegExp): Locator {
  return page.locator('details.tracker-done-section').filter({
    has: page.locator('summary', { hasText: summary }),
  });
}

export async function expandTrackerItem(item: Locator): Promise<void> {
  await item.locator('.tracker-item-header').click();
  await expect(item).not.toHaveClass(/\bminimised\b/);
}

export async function addTask(
  page: Page,
  title: string,
  opts: { body?: string; tags?: string } = {},
): Promise<Locator> {
  await page.goto('/todos');
  await page.locator('details.tracker-add-form > summary', { hasText: 'Add task' }).click();
  const form = page.locator('form[action="/todos/add"]');
  await form.getByLabel('Task title').fill(title);
  if (opts.tags) await form.getByLabel('Tags').fill(opts.tags);
  if (opts.body) await form.getByLabel('Notes').fill(opts.body);
  await form.getByRole('button', { name: 'add task' }).click();
  const item = trackerItem(page, title);
  await expect(item).toBeVisible();
  return item;
}

export function planItem(page: Page, title: string): Locator {
  return page
    .locator('.plan-today-tasks .plan-item')
    .filter({ has: page.locator('.plan-item-title', { hasText: title }) });
}

/** Plans a task for today via the homepage task picker. */
export async function planFromPicker(page: Page, title: string): Promise<Locator> {
  await page.goto('/');
  const picker = page.locator('details.plan-picker');
  if ((await picker.getAttribute('open')) === null) {
    await picker.locator('summary').click();
  }
  await page.getByLabel('Filter tasks').fill(title);
  const pick = page.locator('.plan-pick-item').filter({ hasText: title });
  await expect(pick).toHaveCount(1);
  await pick.getByRole('button', { name: 'today' }).click();
  const item = planItem(page, title);
  await expect(item).toBeVisible();
  return item;
}
