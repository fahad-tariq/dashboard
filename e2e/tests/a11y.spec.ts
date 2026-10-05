import AxeBuilder from '@axe-core/playwright';
import type { Page } from '@playwright/test';
import {
  activeElementId,
  addTask,
  expandPlanItem,
  expandTrackerItem,
  expect,
  planFromPicker,
  planItem,
  tabTo,
  test,
  trackerItem,
  type Theme,
  uniqueTitle,
  useTheme,
  waitForSseSettle,
} from './helpers';

const THEMES: Theme[] = ['dark', 'light'];
const WCAG_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];

const MAIN_PAGES = [
  '/',
  '/todos',
  '/family',
  '/goals',
  '/ideas',
  '/house',
  '/digest',
  '/plan/calendar',
  '/plan/calendar?view=month',
  '/account',
];

async function gotoInTheme(page: Page, path: string, theme: Theme): Promise<void> {
  await page.goto(path);
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
  await waitForSseSettle(page);
}

/**
 * Runs axe and fails on serious or critical violations. Every violation,
 * whatever its impact, is logged and annotated so CI output shows the rule
 * and the offending selectors.
 */
async function expectNoSeriousViolations(page: Page, label: string): Promise<void> {
  // Entry animations fade in from opacity 0, and a scan mid-fade measures
  // blended colours (seen on the confirm modal title). Infinite animations,
  // such as loading pulses, never finish and are skipped.
  await page.evaluate(() =>
    Promise.all(
      document
        .getAnimations()
        .filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
        .map((a) => a.finished.catch(() => undefined)),
    ),
  );
  const { violations } = await new AxeBuilder({ page }).withTags(WCAG_TAGS).analyze();
  const describe = (v: (typeof violations)[number]): string =>
    `[${v.impact ?? 'unknown'}] ${v.id}: ${v.help} -> ${v.nodes.map((n) => n.target.map(String).join(' ')).join(' | ')}`;
  if (violations.length > 0) {
    const lines = violations.map(describe);
    console.log(`axe ${label}\n  ${lines.join('\n  ')}`);
    for (const line of lines) test.info().annotations.push({ type: `axe ${label}`, description: line });
  }
  const blocking = violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map(describe);
  expect(blocking, `serious or critical axe violations on ${label}`).toEqual([]);
}

/** Visible controls smaller than min x min CSS px, described for the failure message. */
function undersizedTargets(page: Page, min: number): Promise<string[]> {
  return page.evaluate((m) => {
    // Inline text links are exempt under WCAG 2.5.8, so only controls are checked.
    const selector = 'button, [role="button"], summary, select, input[type="checkbox"], input[type="radio"]';
    const out: string[] = [];
    for (const el of Array.from(document.querySelectorAll<HTMLElement>(selector))) {
      const rect = el.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) continue;
      if (getComputedStyle(el).visibility === 'hidden') continue;
      if (rect.width + 0.5 >= m && rect.height + 0.5 >= m) continue;
      const id = el.id ? `#${el.id}` : '';
      const cls = el.className && typeof el.className === 'string' ? `.${el.className.trim().split(/\s+/).join('.')}` : '';
      const name = el.getAttribute('aria-label') ?? el.textContent?.trim().slice(0, 30) ?? '';
      out.push(`${el.tagName.toLowerCase()}${id}${cls} "${name}" ${rect.width.toFixed(1)}x${rect.height.toFixed(1)}`);
    }
    return out;
  }, min);
}

/** Running animations or transitions longer than 10ms, described for the failure message. */
function longAnimations(page: Page): Promise<string[]> {
  return page.evaluate(() =>
    document
      .getAnimations()
      .filter((a) => a.playState === 'running' && Number(a.effect?.getTiming().duration ?? 0) > 10)
      .map((a) => {
        const named = a as unknown as { animationName?: string; transitionProperty?: string };
        const target = (a.effect as KeyframeEffect | null)?.target;
        const where = target ? `${target.tagName.toLowerCase()}.${Array.from(target.classList).join('.')}` : '?';
        return `${named.animationName ?? named.transitionProperty ?? a.constructor.name} on ${where} (${String(a.effect?.getTiming().duration)}ms)`;
      }),
  );
}

for (const theme of THEMES) {
  test.describe(`axe, ${theme} theme`, () => {
    test.beforeEach(async ({ page }) => {
      await useTheme(page, theme);
    });

    for (const path of MAIN_PAGES) {
      test(`page ${path}`, async ({ page }) => {
        await gotoInTheme(page, path, theme);
        await expectNoSeriousViolations(page, `${path} (${theme})`);
      });
    }

    test('idea detail page', async ({ page }) => {
      await gotoInTheme(page, '/ideas', theme);
      const href = await page.evaluate(() => {
        const link = Array.from(document.querySelectorAll<HTMLAnchorElement>('main a[href^="/ideas/"]')).find((a) =>
          /^\/ideas\/[a-z0-9][a-z0-9-]*$/.test(a.getAttribute('href') ?? ''),
        );
        return link?.getAttribute('href') ?? null;
      });
      expect(href, 'an idea detail link on /ideas').not.toBeNull();
      await gotoInTheme(page, href as string, theme);
      await expectNoSeriousViolations(page, `${href} (${theme})`);
    });

    test('tracker item expanded', async ({ page }) => {
      await gotoInTheme(page, '/todos', theme);
      const item = trackerItem(page, 'Plan weekend hike');
      await expandTrackerItem(item);
      const toggle = item.locator('.item-toggle');
      await expect(page.locator(`[id="${await toggle.getAttribute('aria-controls')}"]`)).toBeVisible();
      await expectNoSeriousViolations(page, `/todos item expanded (${theme})`);
    });

    test('plan item expanded', async ({ page }) => {
      const title = uniqueTitle(`Expanded plan ${theme}`);
      await addTask(page, title, { body: 'Detail for the scan', tags: 'a11y' });
      const item = await planFromPicker(page, title);
      await expandPlanItem(item);
      await expect(item.locator('.plan-item-detail')).toBeVisible();
      await expectNoSeriousViolations(page, `/ plan item expanded (${theme})`);
    });

    test('select mode with the bulk bar', async ({ page }) => {
      await gotoInTheme(page, '/todos', theme);
      await page.locator('#select-toggle').click();
      await page.getByRole('checkbox', { name: 'Select Plan weekend hike' }).check();
      await expect(page.locator('#bulk-bar')).toBeVisible();
      await expectNoSeriousViolations(page, `/todos select mode (${theme})`);
    });

    test('confirm modal open', async ({ page }) => {
      await gotoInTheme(page, '/todos', theme);
      const item = trackerItem(page, 'Plan weekend hike');
      await expandTrackerItem(item);
      await item.getByRole('button', { name: 'trash' }).click();
      const modal = page.locator('#confirm-modal');
      await expect(modal).toHaveClass(/\bvisible\b/);
      await expectNoSeriousViolations(page, `/todos confirm modal (${theme})`);
      await modal.getByRole('button', { name: 'Cancel' }).click();
      await expect(modal).not.toHaveClass(/\bvisible\b/);
    });

    test('search overlay with results', async ({ page }) => {
      await gotoInTheme(page, '/todos', theme);
      await page.keyboard.press('/');
      const overlay = page.locator('#search-overlay');
      await expect(overlay).toHaveClass(/\bvisible\b/);
      await page.getByLabel('Search tasks, ideas, and house items').fill('passport');
      await expect(overlay.locator('.search-result').first()).toBeVisible();
      await expectNoSeriousViolations(page, `search overlay (${theme})`);
    });

    test('shortcut help', async ({ page }) => {
      await gotoInTheme(page, '/todos', theme);
      await page.keyboard.press('?');
      await expect(page.locator('#shortcut-help')).toHaveClass(/\bvisible\b/);
      await expectNoSeriousViolations(page, `shortcut help (${theme})`);
    });

    test('mobile nav open', async ({ page }) => {
      await page.setViewportSize({ width: 375, height: 800 });
      await gotoInTheme(page, '/todos', theme);
      const hamburger = page.locator('#nav-hamburger');
      await hamburger.click();
      await expect(page.locator('#nav-links')).toHaveClass(/\bnav-links-open\b/);
      await expect(hamburger).toHaveAttribute('aria-expanded', 'true');
      await expectNoSeriousViolations(page, `mobile nav (${theme})`);
    });

    test('nav "more" menu open', async ({ page }) => {
      await gotoInTheme(page, '/todos', theme);
      await page.getByRole('button', { name: 'more' }).click();
      await expect(page.locator('#nav-more-menu')).toBeVisible();
      await expectNoSeriousViolations(page, `more menu (${theme})`);
    });

    test('toast visible', async ({ page }) => {
      await gotoInTheme(page, '/todos', theme);
      await page.evaluate(() => (window as unknown as { showToast: (msg: string) => void }).showToast('Saved'));
      await expect(page.locator('#toast')).toBeVisible();
      await expect(page.locator('#toast .toast-text')).toHaveText('Saved');
      await expect(page.locator('#announcer')).toHaveText('Saved');
      await expectNoSeriousViolations(page, `toast (${theme})`);
    });
  });
}

test.describe('reflow at 320px', () => {
  test.use({ viewport: { width: 320, height: 640 } });

  for (const path of MAIN_PAGES) {
    test(`no horizontal page scroll on ${path}`, async ({ page }) => {
      await page.goto(path);
      await waitForSseSettle(page);
      // The house table is wide by design; it scrolls inside .house-table-wrap,
      // so its cells are excluded from the offender list but the page itself
      // must still not scroll.
      const { scrollWidth, clientWidth, offenders } = await page.evaluate(() => {
        const root = document.documentElement;
        const limit = root.clientWidth + 1;
        const found: string[] = [];
        for (const el of Array.from(document.body.querySelectorAll<HTMLElement>('*'))) {
          if (el.closest('.house-table-wrap') && !el.classList.contains('house-table-wrap')) continue;
          const rect = el.getBoundingClientRect();
          if (rect.width > 0 && rect.right > limit) {
            found.push(`${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : ''}.${Array.from(el.classList).join('.')} right=${Math.round(rect.right)}`);
          }
          if (found.length >= 10) break;
        }
        return { scrollWidth: root.scrollWidth, clientWidth: root.clientWidth, offenders: found };
      });
      expect(scrollWidth, `page scrolls horizontally; widest elements: ${offenders.join(', ')}`).toBeLessThanOrEqual(
        clientWidth + 1,
      );
      if (path === '/house') {
        const overflowX = await page.locator('.house-table-wrap').first().evaluate((el) => getComputedStyle(el).overflowX);
        expect(['auto', 'scroll']).toContain(overflowX);
      }
    });
  }
});

test.describe('reduced motion', () => {
  test.use({ reducedMotion: 'reduce' });

  test('completing a planned task runs no animation', async ({ page }) => {
    const title = uniqueTitle('Calm completion');
    await addTask(page, title);
    const item = await planFromPicker(page, title);
    expect(await page.evaluate(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches)).toBe(true);
    expect(await longAnimations(page), 'animations on page load').toEqual([]);

    // Trigger the completion celebration without submitting, so its animation
    // can be inspected before the form navigates away.
    await item
      .locator('form[action$="/complete"]')
      .evaluate((form) =>
        (window as unknown as { celebrateComplete: (f: Element) => boolean }).celebrateComplete(form),
      );
    await expect(item).toHaveClass(/plan-item-completing/);
    expect(await longAnimations(page), 'animations during the completion celebration').toEqual([]);

    await item.getByRole('button', { name: `done ${title}` }).click();
    await expect(planItem(page, title)).toHaveClass(/plan-item-done/);
    expect(await longAnimations(page), 'animations after completion').toEqual([]);
  });
});

test('skip link moves focus to main', async ({ page }) => {
  await page.goto('/todos');
  await page.keyboard.press('Tab');
  await expect(page.locator('a.skip-link')).toBeFocused();
  await expect(page.locator('a.skip-link')).toHaveAttribute('href', '#main');
  await page.keyboard.press('Enter');
  await expect(page.locator('#main')).toBeFocused();
});

test('keyboard only: expand, reorder and complete a plan item', async ({ page }) => {
  test.setTimeout(120_000);
  const first = uniqueTitle('Keyboard first');
  const second = uniqueTitle('Keyboard second');
  await addTask(page, first);
  await addTask(page, second);
  const firstId = await (await planFromPicker(page, first)).getAttribute('id');
  const secondId = await (await planFromPicker(page, second)).getAttribute('id');
  const personalTitles = () =>
    page.locator('.plan-today-tasks .plan-item[data-list="todos"] .plan-item-title').allTextContents();

  await page.reload();
  await waitForSseSettle(page);

  // Move whichever of the two is not already at the top of the personal list.
  const [title, rowId] = (await personalTitles()).indexOf(second) > 0 ? [second, secondId] : [first, firstId];
  expect(rowId).toMatch(/^plan-todos-[a-z0-9-]+$/);
  const byId = (suffix: string) => page.locator(`[id="${rowId}${suffix}"]`);

  // Start from the skip link rather than tabbing through the nav.
  await page.keyboard.press('Tab');
  await expect(page.locator('a.skip-link')).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('#main')).toBeFocused();

  await tabTo(page, `${rowId}-toggle`);
  await page.keyboard.press('Enter');
  await expect(byId('-toggle')).toHaveAttribute('aria-expanded', 'true');
  await expect(byId('-detail')).toBeVisible();

  await tabTo(page, `${rowId}-up`);
  await expect(byId('-up')).toHaveAccessibleName(`Move ${title} up`);
  for (let i = 0; i < 50; i++) {
    const before = await personalTitles();
    const index = before.indexOf(title);
    expect(index).toBeGreaterThanOrEqual(0);
    if (index === 0) break;

    await Promise.all([
      page.waitForResponse((r) => new URL(r.url()).pathname === '/plan/reorder' && r.ok()),
      page.keyboard.press('Enter'),
    ]);
    // `index` is the 0-based old position, so it is also the 1-based new one.
    await expect(page.locator('#announcer')).toHaveText(new RegExp(`Moved .+ to position ${index} of \\d+`));
    await expect(page.locator('#announcer')).toContainText(title);
    expect((await personalTitles()).indexOf(title)).toBe(index - 1);
    // Focus returns to the pressed button and survives the SSE refresh that
    // the write triggers.
    await expect.poll(() => activeElementId(page)).toBe(`${rowId}-up`);
    await waitForSseSettle(page);
    expect(await activeElementId(page)).toBe(`${rowId}-up`);
  }
  expect((await personalTitles())[0]).toBe(title);
  await expect(page.locator('#announcer')).toHaveText(/position 1 of \d+/);
  await expect(byId('-toggle')).toHaveAttribute('aria-expanded', 'true');

  await tabTo(page, `${rowId}-done`, 10);
  await expect(byId('-done')).toHaveAccessibleName(`done ${title}`);
  await page.keyboard.press('Enter');
  await expect(planItem(page, title)).toHaveClass(/plan-item-done/);
});

for (const [label, hasTouch, min] of [
  ['fine pointer', false, 24],
  ['coarse pointer', true, 44],
] as const) {
  test.describe(`target size, ${label}`, () => {
    test.use({ hasTouch });

    for (const path of ['/', '/todos']) {
      test(`controls on ${path} are at least ${min}px`, async ({ page }) => {
        await page.goto(path);
        await waitForSseSettle(page);
        const coarse = await page.evaluate(() => window.matchMedia('(pointer: coarse)').matches);
        expect(coarse, 'pointer: coarse emulation').toBe(hasTouch);
        expect(await undersizedTargets(page, min)).toEqual([]);
      });
    }
  });
}
