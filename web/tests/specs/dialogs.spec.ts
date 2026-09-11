import { basicScenario } from '../fixtures/factories';
import { appAdmin, orgAdmin } from '../fixtures/personas';
import { expect, test } from '../fixtures/test';

/*
 * S2: DestinationsPage used to hand-roll its own overlay (a bare
 * fixed-inset-0 div with no role) instead of the shared dialog shell every
 * other create form uses. This is the kill-switch: a real role=dialog,
 * discoverable by its accessible name, not just "some element became
 * visible".
 */

test('new destination opens a labelled dialog', async ({ page, api }) => {
  await api.loginAs(appAdmin);
  const s = basicScenario();
  api.seed({ orgs: [s.org], destinations: [] });
  await page.goto('/destinations');

  await page.getByRole('button', { name: /new destination/i }).click();
  await expect(page.getByRole('dialog', { name: 'New destination' })).toBeVisible();
});

test('Escape closes the new destination dialog', async ({ page, api }) => {
  await api.loginAs(appAdmin);
  const s = basicScenario();
  api.seed({ orgs: [s.org], destinations: [] });
  await page.goto('/destinations');

  await page.getByRole('button', { name: /new destination/i }).click();
  const dialog = page.getByRole('dialog', { name: 'New destination' });
  await expect(dialog).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
});

// W7-08: real, positive coverage for a non-appAdmin persona — an org admin
// gets the same labelled dialog shell an app admin does (useCanAdminister
// permits app admin OR org role "admin", DestinationsPage.tsx).
test('an org admin also opens a labelled new destination dialog', async ({ page, api }) => {
  await api.loginAs(orgAdmin);
  const s = basicScenario();
  api.seed({ orgs: [s.org], destinations: [] });
  await page.goto('/destinations');

  await page.getByRole('button', { name: /new destination/i }).click();
  await expect(page.getByRole('dialog', { name: 'New destination' })).toBeVisible();
});
