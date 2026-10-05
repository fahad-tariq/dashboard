import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import type { Page } from '@playwright/test';
import {
  expandTrackerItem,
  expect,
  STYLES,
  test,
  trackerItem,
  type Theme,
  useStyle,
  useTheme,
  waitForSseSettle,
} from './helpers';

/**
 * Full-page screenshots of every main page and the key interactive states,
 * for design review. Nothing is compared: CI uploads e2e/screenshots as an
 * artefact so before and after sets can be put side by side.
 */

const OUT_DIR = join(__dirname, '..', 'screenshots');
const THEMES: Theme[] = ['light', 'dark'];
const WIDTHS = [
  { label: 'desktop', width: 1280, height: 900 },
  { label: 'phone', width: 375, height: 812 },
] as const;
const PAGES = [
  ['home', '/'],
  ['todos', '/todos'],
  ['family', '/family'],
  ['goals', '/goals'],
  ['ideas', '/ideas'],
  ['house', '/house'],
  ['digest', '/digest'],
  ['calendar-week', '/plan/calendar'],
  ['calendar-month', '/plan/calendar?view=month'],
  ['account', '/account'],
] as const;

async function shoot(page: Page, name: string): Promise<void> {
  // Let entry animations finish so the capture shows the resting state.
  await page.evaluate(() =>
    Promise.all(
      document
        .getAnimations()
        .filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
        .map((a) => a.finished.catch(() => undefined)),
    ),
  );
  mkdirSync(OUT_DIR, { recursive: true });
  await page.screenshot({ path: join(OUT_DIR, `${name}.png`), fullPage: true });
}

// Screenshots only need the page at rest, not a full SSE round trip.
async function open(page: Page, path: string): Promise<void> {
  await page.goto(path);
  await waitForSseSettle(page, 500);
}

for (const style of STYLES) for (const theme of THEMES) {
  for (const size of WIDTHS) {
    test.describe(`screenshots, ${style}, ${theme}, ${size.label}`, () => {
      test.use({ viewport: { width: size.width, height: size.height } });
      test.beforeEach(async ({ page }) => {
        await useTheme(page, theme);
        await useStyle(page, style);
      });
      const prefix = `${style}-${theme}-${size.label}`;

      test('main pages', async ({ page }) => {
        test.setTimeout(120_000);
        for (const [label, path] of PAGES) {
          await open(page, path);
          await shoot(page, `${prefix}-${label}`);
        }
      });

      test('interactive states', async ({ page }) => {
        test.setTimeout(60_000);
        await open(page, '/todos');
        const item = trackerItem(page, 'Plan weekend hike');
        await expandTrackerItem(item);
        await shoot(page, `${prefix}-state-item-expanded`);

        await page.locator('#select-toggle').click();
        await page.getByRole('checkbox', { name: 'Select Plan weekend hike' }).check();
        await expect(page.locator('#bulk-bar')).toBeVisible();
        await shoot(page, `${prefix}-state-select-mode`);

        await open(page, '/todos');
        const hike = trackerItem(page, 'Plan weekend hike');
        await expandTrackerItem(hike);
        // Phones fold the row actions behind "more".
        const more = hike.locator('summary.item-actions-toggle');
        if (await more.isVisible()) await more.click();
        await hike.getByRole('button', { name: 'move to family' }).click();
        const modal = page.locator('#confirm-modal');
        await expect(modal).toHaveAttribute('open');
        await shoot(page, `${prefix}-state-dialog`);
        await modal.getByRole('button', { name: 'Cancel' }).click();

        await page.evaluate(() => (window as unknown as { showToast: (msg: string) => void }).showToast('Saved'));
        await expect(page.locator('#toast')).toBeVisible();
        await shoot(page, `${prefix}-state-toast`);
      });
    });
  }
}
