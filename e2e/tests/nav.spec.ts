import { expect, test } from './helpers';

test('"g" shortcuts come from the nav, including links behind "more"', async ({ page }) => {
  await page.goto('/');
  await page.keyboard.press('g');
  await page.keyboard.press('d');
  await expect(page).toHaveURL(/\/digest$/);

  await page.keyboard.press('g');
  await page.keyboard.press('c');
  await expect(page).toHaveURL(/\/plan\/calendar$/);
  await expect(page.locator('#nav-more-menu a[href="/plan/calendar"]')).toHaveAttribute('aria-current', 'page');
});

test('nav marks the current section by path prefix', async ({ page }) => {
  await page.goto('/ideas');
  const ideas = page.locator('#nav-links a[href="/ideas"]');
  await expect(ideas).toHaveAttribute('aria-current', 'page');
  await expect(page.locator('#nav-links a[href="/"]')).not.toHaveAttribute('aria-current', 'page');

  const href = await page.evaluate(() => {
    const link = Array.from(document.querySelectorAll<HTMLAnchorElement>('main a[href^="/ideas/"]')).find((a) =>
      /^\/ideas\/[a-z0-9][a-z0-9-]*$/.test(a.getAttribute('href') ?? ''),
    );
    return link?.getAttribute('href') ?? null;
  });
  expect(href).not.toBeNull();
  await page.goto(href as string);
  await expect(ideas).toHaveAttribute('aria-current', 'page');
});

test('"more" opens and closes on Escape, returning focus', async ({ page }) => {
  await page.goto('/todos');
  const more = page.getByRole('button', { name: 'more' });
  const menu = page.locator('#nav-more-menu');

  await expect(menu).toBeHidden();
  await more.click();
  await expect(more).toHaveAttribute('aria-expanded', 'true');
  await expect(menu.getByRole('link', { name: 'digest' })).toBeVisible();

  await menu.getByRole('link', { name: 'digest' }).focus();
  await page.keyboard.press('Escape');
  await expect(menu).toBeHidden();
  await expect(more).toHaveAttribute('aria-expanded', 'false');
  await expect(more).toBeFocused();

  await more.click();
  await page.locator('main').click({ position: { x: 5, y: 5 } });
  await expect(menu).toBeHidden();
});

test('mobile menu lists "more" links inline and closes on Escape', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 700 });
  await page.goto('/todos');
  const hamburger = page.locator('#nav-hamburger');

  await hamburger.click();
  await expect(page.locator('#nav-links')).toHaveClass(/\bnav-links-open\b/);
  await expect(page.getByRole('button', { name: 'more' })).toBeHidden();
  await expect(page.locator('#nav-more-menu a[href="/digest"]')).toBeVisible();
  const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
  expect(scrollWidth).toBeLessThanOrEqual(320);

  await page.keyboard.press('Escape');
  await expect(page.locator('#nav-links')).not.toHaveClass(/\bnav-links-open\b/);
  await expect(hamburger).toHaveAttribute('aria-expanded', 'false');
  await expect(hamburger).toBeFocused();
});
