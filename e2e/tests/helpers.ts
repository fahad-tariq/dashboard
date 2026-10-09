import { appendFileSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { test as base, expect, type Locator, type Page } from '@playwright/test';

/**
 * Every page, including extra ones a test opens in its context, records the
 * time of its last SSE message or htmx settle in window.__e2eLastActivity, so
 * waitForSseSettle can wait for quiet instead of sleeping a fixed time.
 */
export const test = base.extend({
  context: async ({ context }, use) => {
    await context.addInitScript(() => {
      const w = window as unknown as { __e2eLastActivity: number };
      const touch = (): void => {
        w.__e2eLastActivity = Date.now();
      };
      touch();
      document.addEventListener('htmx:sseMessage', touch);
      document.addEventListener('htmx:beforeRequest', touch);
      document.addEventListener('htmx:afterSettle', touch);
    });
    await use(context);
  },
});

export { expect };

/** Title with a per-run suffix so tests never collide on shared server state. */
export function uniqueTitle(prefix: string): string {
  return `${prefix} ${Date.now().toString(36)}${Math.floor(Math.random() * 1e4)}`;
}

/** Fixed item IDs from e2e/fixtures; every seeded item line ends in `[id: ...]`. */
export const fixtureIds = {
  renewPassport: 'pssp0rt1',
  planWeekendHike: 'h1k3wknd',
  lodgeTaxReturn: 'txrtrn01',
  read12Books: 'bks12gl0',
  // Two open tasks titled "Call the bank".
  callTheBank1: 'bnkc4ll1',
  callTheBank2: 'bnkc4ll2',
  bookDentist: 'dntst001',
  schoolPickupRoster: 'rstr0001',
  paintBackFence: 'fnc3pnt1',
  cleanGutters: 'gttrs001',
  homeWeatherStation: 'wthr5tn1',
  learnToSail: 's41l1ng1',
} as const;

/** The 8-character item ID alphabet: digits and consonants other than y. */
export const itemIdPattern = '[b-df-hj-np-tv-xz0-9]{8}';

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
 * A form submitted through htmx gets the updated page back in the same
 * response and morphs the page's live container ([data-live]); the tab then
 * skips the SSE echo of its own write by revision. Writes from elsewhere
 * (other tabs, the API, an external editor) still send a `changed:<module>`
 * event after a 500ms debounce, which morphs the container again. Morphs keep
 * expanded rows, open <details> and typed text, so this wait is about not
 * racing an in-flight refresh, not about lost state. It returns once the page
 * has been quiet for longer than the debounce plus a round trip.
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
    .locator('.tracker-list > .tracker-item')
    .filter({ has: page.locator('.tracker-item-title', { hasText: title }) });
}

export function trackerSection(page: Page, summary: RegExp): Locator {
  return page.locator('details.tracker-done-section').filter({
    has: page.locator('summary', { hasText: summary }),
  });
}

/**
 * Expands a tracker or idea row through its toggle button. A row keeps its
 * expanded state through morphs, including moves between lists (trash and
 * restore, triage), so it may already be open.
 */
export async function expandTrackerItem(item: Locator): Promise<void> {
  const toggle = item.locator('.item-toggle');
  if ((await toggle.getAttribute('aria-expanded')) !== 'true') await toggle.click();
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
export type Style = 'cards' | 'paper' | 'document';
export const STYLES: Style[] = ['cards', 'paper', 'document'];

/** Stores the theme before any page script runs, as a returning visitor would have it. */
export async function useTheme(page: Page, theme: Theme): Promise<void> {
  await page.addInitScript((t) => {
    localStorage.setItem('theme', t);
  }, theme);
}

/** Stores the style before any page script runs, as the style selector would. */
export async function useStyle(page: Page, style: Style): Promise<void> {
  await page.addInitScript((s) => {
    localStorage.setItem('style', s);
  }, style);
}

/** Id of the focused element, or '' when nothing with an id has focus. */
export function activeElementId(page: Page): Promise<string> {
  return page.evaluate(() => document.activeElement?.id ?? '');
}

/** Presses Tab until the element with the given id has focus. */
export async function tabTo(page: Page, id: string, maxPresses = 400, key: 'Tab' | 'Shift+Tab' = 'Tab'): Promise<void> {
  for (let i = 0; i < maxPresses; i++) {
    if ((await activeElementId(page)) === id) return;
    await page.keyboard.press(key);
  }
  throw new Error(`#${id} not reached after ${maxPresses} ${key} presses`);
}

type MarkedWindow = Window & { __e2eNoReload?: boolean };

/** Marks the current document so expectNoReload can tell it was not replaced. */
export async function markNoReload(page: Page): Promise<void> {
  await page.evaluate(() => {
    (window as MarkedWindow).__e2eNoReload = true;
  });
}

/** Fails if the page navigated or reloaded since markNoReload. */
export async function expectNoReload(page: Page): Promise<void> {
  expect(await page.evaluate(() => (window as MarkedWindow).__e2eNoReload)).toBe(true);
}
