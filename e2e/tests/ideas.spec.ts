import type { Page } from '@playwright/test';
import { expandTrackerItem, expect, expectNoReload, markNoReload, test, uniqueTitle, waitForSseSettle } from './helpers';

function ideaSection(page: Page, heading: string) {
  return page.locator('.ideas-page section').filter({
    has: page.getByRole('heading', { level: 2, name: new RegExp(`^${heading} \\(\\d+\\)$`) }),
  });
}

test('add an idea and triage it', async ({ page }) => {
  const title = uniqueTitle('Compost bin sensor');

  await page.goto('/ideas');
  await waitForSseSettle(page);
  await page.locator('details.tracker-add-form > summary', { hasText: 'Add idea' }).click();
  const form = page.locator('form[action="/ideas/add"]');
  await form.getByLabel('Idea title').fill(title);
  await form.getByLabel('Tags').fill('electronics');
  await form.getByLabel('Description').fill('Track temperature inside the bin.');
  await form.getByRole('button', { name: 'add idea' }).click();

  // New ideas land in Untriaged.
  const untriagedCard = ideaSection(page, 'Untriaged').locator('.tracker-item', { hasText: title });
  await expect(untriagedCard).toBeVisible();
  await waitForSseSettle(page);

  await expandTrackerItem(untriagedCard);

  // Triage posts through htmx and morphs the page; it does not reload.
  await markNoReload(page);
  await untriagedCard.getByRole('button', { name: 'park' }).click();
  const parkedCard = ideaSection(page, 'Parked').locator('.tracker-item', { hasText: title });
  await expect(parkedCard).toBeVisible();
  await expect(ideaSection(page, 'Untriaged').locator('.tracker-item', { hasText: title })).toHaveCount(0);
  await expectNoReload(page);
});

test('seeded idea keeps blank lines in its body', async ({ page }) => {
  await page.goto('/ideas');
  const card = ideaSection(page, 'Untriaged').locator('.tracker-item', { hasText: 'Home weather station' });
  await expandTrackerItem(card);
  await expect(card.locator('.tracker-item-body')).toContainText('Log readings to SQLite');
});

test('trash from the idea page goes to the list and offers undo', async ({ page }) => {
  const title = uniqueTitle('Rain gauge');

  await page.goto('/ideas');
  await waitForSseSettle(page);
  await page.locator('details.tracker-add-form > summary', { hasText: 'Add idea' }).click();
  const form = page.locator('form[action="/ideas/add"]');
  await form.getByLabel('Idea title').fill(title);
  await form.getByRole('button', { name: 'add idea' }).click();
  const card = ideaSection(page, 'Untriaged').locator('.tracker-item', { hasText: title });
  await expect(card).toBeVisible();
  await waitForSseSettle(page);

  await card.getByRole('link', { name: title }).click();
  await expect(page.getByRole('heading', { level: 1, name: title })).toBeVisible();

  // No confirm dialog: the toast offers undo instead.
  await page.getByRole('button', { name: 'move to trash' }).click();
  await expect(page).toHaveURL(/\/ideas$/);
  await expect(page.locator('dialog[open]')).toHaveCount(0);
  await expect(ideaSection(page, 'Untriaged').locator('.tracker-item', { hasText: title })).toHaveCount(0);

  await page.locator('#toast').getByRole('button', { name: 'undo' }).click();
  await expect(ideaSection(page, 'Untriaged').locator('.tracker-item', { hasText: title })).toBeVisible();
});
