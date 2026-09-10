/**
 * W6-S8: the topbar breadcrumb is built from routeManifest's route labels
 * (breadcrumb.ts's buildCrumbs), not the raw pathname — '/admin/users' used
 * to render literally as "admin / users".
 */
import { appAdmin } from '../fixtures/personas';
import { expect, test } from '../fixtures/test';

test('breadcrumb reads Admin / Users on /admin/users, not the raw pathname', async ({
  page,
  api,
}) => {
  await api.loginAs(appAdmin);
  await page.goto('/admin/users');

  const breadcrumb = page.getByRole('navigation', { name: 'breadcrumb' });
  await expect(breadcrumb).toContainText('Admin');
  await expect(breadcrumb).toContainText('Users');
  // The raw-pathname rendering this replaced joined segments with ' / ' and
  // left them lowercase — assert it's actually gone, not just that the new
  // text happens to also be present somewhere.
  await expect(breadcrumb).not.toContainText('admin / users');
});

test('breadcrumb reads Overview on /, the text other specs locate by', async ({ page, api }) => {
  await api.loginAs(appAdmin);
  await page.goto('/');

  await expect(page.getByRole('navigation', { name: 'breadcrumb' })).toHaveText('Overview');
});

test('breadcrumb reads Collectors / Collector on a collector detail page', async ({
  page,
  api,
}) => {
  await api.loginAs(appAdmin);
  await page.goto('/collectors/col-0001');

  const breadcrumb = page.getByRole('navigation', { name: 'breadcrumb' });
  await expect(breadcrumb).toContainText('Collectors');
  await expect(breadcrumb).toContainText('Collector');
});
