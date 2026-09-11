import { basicScenario } from '../fixtures/factories';
import { appAdmin, orgAdmin, orgEditor, reader } from '../fixtures/personas';
import { expect, test } from '../fixtures/test';

/*
 * Two sibling specs were removed rather than fixed. 'destinations page renders
 * heading' only asserted the heading, which every test below already does and
 * the fullstack walkthrough covers for all routes. 'destination list requires
 * its GET handler' was labelled a kill-switch probe but asserted the heading
 * too — it could not fail if the handler disappeared. The first test below is
 * the real kill-switch: it reads rows that only exist if the list is fetched.
 */

test('seeded destinations render name, type, and URL host', async ({ page, api }) => {
  await api.loginAs(appAdmin);
  const s = basicScenario();
  api.seed({ orgs: [s.org], destinations: s.destinations });
  await page.goto('/destinations');

  await expect(page.getByText('prom-prod')).toBeVisible();
  await expect(page.getByRole('cell', { name: 'prometheus', exact: true })).toBeVisible();
  await expect(page.getByText(/prometheus\.example\.com/).first()).toBeVisible();
});

test('destination create dialog reports invalid URL', async ({ page, api }) => {
  await api.loginAs(appAdmin);
  const s = basicScenario();
  api.seed({ orgs: [s.org], destinations: [] });
  await page.goto('/destinations');

  await page.getByRole('button', { name: /new|create|add destination/i }).click();
  // A malformed URL, not an empty one: empty fields are caught by the inputs'
  // own `required` before submit ever runs, so they exercise the browser rather
  // than this dialog's validation.
  await page.getByLabel(/name/i).fill('bad-dest');
  await page.getByLabel(/url/i).fill('not-a-url');
  await page.getByRole('button', { name: /save|create/i }).click();

  // Assert the message itself. This used to match /valid URL|URL/i, whose second
  // branch matches the dialog's own "URL" field LABEL — so it passed whether or
  // not any validation rendered, and would not have caught the message going away.
  await expect(page.getByText(/Enter a valid URL/i)).toBeVisible();
});

test('created destination appears in the list', async ({ page, api }) => {
  await api.loginAs(appAdmin);
  const s = basicScenario();
  api.seed({ orgs: [s.org], destinations: [] });
  await page.goto('/destinations');

  await page.getByRole('button', { name: /new|create|add destination/i }).click();
  await page.getByLabel(/name/i).fill('new-destination');
  await page.getByLabel(/url/i).fill('https://new.example.com');
  await page.getByRole('button', { name: /save|create/i }).click();

  await expect(page.getByText('new-destination')).toBeVisible();
});

// /destinations carries no requiredRole (routeManifest.ts) — any
// authenticated org member reaches it — but write access is gated by
// useCanAdminister (appAdmin or org role "admin"), so an org admin sees
// the same write affordance an app admin does, while an editor and a
// reader see the list but not the button (W7-08: real, positive coverage
// for the roles this route actually ships, not just appAdmin).
test('an org admin, not just an app admin, can create a destination', async ({ page, api }) => {
  await api.loginAs(orgAdmin);
  const s = basicScenario();
  api.seed({ orgs: [s.org], destinations: [] });
  await page.goto('/destinations');

  await expect(page.getByRole('button', { name: /new|create|add destination/i })).toBeVisible();
  await page.getByRole('button', { name: /new|create|add destination/i }).click();
  await page.getByLabel(/name/i).fill('org-admin-destination');
  await page.getByLabel(/url/i).fill('https://org-admin.example.com');
  await page.getByRole('button', { name: /save|create/i }).click();

  await expect(page.getByText('org-admin-destination')).toBeVisible();
});

test('orgEditor sees the destinations list but not the New destination button', async ({
  page,
  api,
}) => {
  await api.loginAs(orgEditor);
  const s = basicScenario();
  api.seed({ orgs: [s.org], destinations: s.destinations });
  await page.goto('/destinations');

  // Positive control first: the page actually rendered seeded data, so the
  // absence check below is not vacuous.
  await expect(page.getByText('prom-prod')).toBeVisible();
  await expect(page.getByRole('button', { name: /new|create|add destination/i })).toHaveCount(0);
});

test('reader sees the destinations list but not the New destination button', async ({
  page,
  api,
}) => {
  await api.loginAs(reader);
  const s = basicScenario();
  api.seed({ orgs: [s.org], destinations: s.destinations });
  await page.goto('/destinations');

  await expect(page.getByText('prom-prod')).toBeVisible();
  await expect(page.getByRole('button', { name: /new|create|add destination/i })).toHaveCount(0);
});
