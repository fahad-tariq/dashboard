import { appendFileSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { test as base, expect, type Locator, type Page } from '@playwright/test';

/**
 * Every page records the time of its last SSE message or htmx settle in
 * window.__e2eLastActivity, so waitForSseSettle can wait for quiet instead of
 * sleeping a fixed time.
 */
export const test = base.extend({
  page: async ({ page }, use) => {
    await page.addInitScript(() => {
      const w = window as unknown as { __e2eLastActivity: number };
      const touch = (): void => {
        w.__e2eLastActivity = Date.now();
      };
      touch();
      document.addEventListener('htmx:sseMessage', touch);
      document.addEventListener('htmx:beforeRequest', touch);
      document.addEventListener('htmx:afterSettle', touch);
    });
    await use(page);
  },
});

export { expect };

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
 * page container: open <details> close, rows collapse, and a form held by the
 * confirm modal is detached so submitting it does nothing. Wait until the page
 * has been quiet for longer than the debounce plus a round trip. Plan 1 Phase 3
 * stops the app's own writes from broadcasting, after which this mostly no-ops.
 */
export async function waitForSseSettle(page: Page, quietMs = 1200): Promise<void> {
  await page.waitForFunction(
    (q) => Date.now() - (window as unknown as { __e2eLastActivity: number }).__e2eLastActivity > q,
    quietMs,
    { polling: 100 },
  );
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

/** Expands a tracker or idea row through its toggle button. */
export async function expandTrackerItem(item: Locator): Promise<void> {
  const toggle = item.locator('.item-toggle');
  await toggle.click();
  await expect(item).not.toHaveClass(/\bminimised\b/);
  await expect(toggle).toHaveAttribute('aria-expanded', 'true');
}

export async function addTask(
  page: Page,
  title: string,
  opts: { body?: string; tags?: string } = {},
): Promise<Locator> {
  await page.goto('/todos');
  await waitForSseSettle(page);
  await page.locator('details.tracker-add-form > summary', { hasText: 'Add task' }).click();
  const form = page.locator('form[action="/todos/add"]');
  await form.getByLabel('Task title').fill(title);
  if (opts.tags) await form.getByLabel('Tags').fill(opts.tags);
  if (opts.body) await form.getByLabel('Notes').fill(opts.body);
  await form.getByRole('button', { name: 'add task' }).click();
  const item = trackerItem(page, title);
  await expect(item).toBeVisible();
  await waitForSseSettle(page);
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
  await waitForSseSettle(page);
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
  await waitForSseSettle(page);
  return item;
}

/** Expands a plan row on the homepage through its toggle button. */
export async function expandPlanItem(item: Locator): Promise<void> {
  const toggle = item.locator('.plan-item-toggle');
  await toggle.click();
  await expect(item).not.toHaveClass(/\bminimised\b/);
  await expect(toggle).toHaveAttribute('aria-expanded', 'true');
}

export type Theme = 'dark' | 'light';

/** Stores the theme before any page script runs, as a returning visitor would have it. */
export async function useTheme(page: Page, theme: Theme): Promise<void> {
  await page.addInitScript((t) => {
    localStorage.setItem('theme', t);
  }, theme);
}

/** Id of the focused element, or '' when nothing with an id has focus. */
export function activeElementId(page: Page): Promise<string> {
  return page.evaluate(() => document.activeElement?.id ?? '');
}

/** Presses Tab until the element with the given id has focus. */
export async function tabTo(page: Page, id: string, maxPresses = 400): Promise<void> {
  for (let i = 0; i < maxPresses; i++) {
    if ((await activeElementId(page)) === id) return;
    await page.keyboard.press('Tab');
  }
  throw new Error(`#${id} not reached after ${maxPresses} Tab presses`);
}
