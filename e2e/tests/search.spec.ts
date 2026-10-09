import { expect, fixtureIds, test } from './helpers';

test('search dialog opens with "/", finds a task and navigates to it', async ({ page }) => {
  await page.goto('/ideas');
  const overlay = page.locator('#search-overlay');
  const input = page.getByLabel('Search tasks, ideas, and house items');

  await page.keyboard.press('/');
  await expect(overlay).toHaveAttribute('open');
  await expect(input).toBeFocused();

  await input.fill('passport');
  const result = overlay.locator('.search-result', { hasText: 'Renew passport' });
  await expect(result).toBeVisible();
  await expect(result).toHaveAttribute('href', `/todos#item-${fixtureIds.renewPassport}`);

  await input.press('ArrowDown');
  await expect(result).toHaveClass(/search-result-active/);
  await input.press('Enter');

  await expect(page).toHaveURL(new RegExp(`/todos#item-${fixtureIds.renewPassport}$`));
  await expect(page.locator(`#item-${fixtureIds.renewPassport}`)).not.toHaveClass(/\bminimised\b/);
});

test('search shows an empty state, and Escape and Ctrl+K toggle the dialog', async ({ page }) => {
  await page.goto('/');
  const overlay = page.locator('#search-overlay');
  const input = page.getByLabel('Search tasks, ideas, and house items');

  await page.keyboard.press('Control+k');
  await expect(overlay).toHaveAttribute('open');
  await input.fill('zzqx-no-such-thing');
  await expect(overlay.locator('.search-empty')).toHaveText('No results for "zzqx-no-such-thing"');

  await input.press('Escape');
  await expect(overlay).not.toHaveAttribute('open');
  // Focus must leave the closed dialog's input, or the "/" shortcut below is
  // swallowed by the typing guard.
  await expect(input).not.toBeFocused();

  // Results span lists and link by item ID: the seeded idea, family task and
  // house items are searchable too.
  await page.keyboard.press('/');
  await input.fill('weather');
  await expect(overlay.locator('.search-result', { hasText: 'Home weather station' })).toHaveAttribute(
    'href',
    `/ideas/${fixtureIds.homeWeatherStation}`,
  );
  await input.fill('roster');
  await expect(overlay.locator('.search-result', { hasText: 'Organise school pickup roster' })).toHaveAttribute(
    'href',
    `/family#item-${fixtureIds.schoolPickupRoster}`,
  );
  await input.fill('gutters');
  await expect(overlay.locator('.search-result', { hasText: 'Clean gutters' })).toHaveAttribute(
    'href',
    `/house#maint-${fixtureIds.cleanGutters}`,
  );
  await input.fill('fence');
  await expect(overlay.locator('.search-result', { hasText: 'Paint the back fence' })).toHaveAttribute(
    'href',
    `/house#item-${fixtureIds.paintBackFence}`,
  );
});
