import type { Page } from '@playwright/test';
import { expect, test, uniqueTitle, waitForSseSettle } from './helpers';

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

  await untriagedCard.locator('.tracker-item-header').click();
  await expect(untriagedCard).not.toHaveClass(/\bminimised\b/);

  // Triage posts via fetch then reloads the page.
  await untriagedCard.getByRole('button', { name: 'park' }).click();
  const parkedCard = ideaSection(page, 'Parked').locator('.tracker-item', { hasText: title });
  await expect(parkedCard).toBeVisible();
  await expect(ideaSection(page, 'Untriaged').locator('.tracker-item', { hasText: title })).toHaveCount(0);
});

test('seeded idea keeps blank lines in its body', async ({ page }) => {
  await page.goto('/ideas');
  const card = ideaSection(page, 'Untriaged').locator('.tracker-item', { hasText: 'Home weather station' });
  await card.locator('.tracker-item-header').click();
  await expect(card.locator('.tracker-item-body')).toContainText('Log readings to SQLite');
});
