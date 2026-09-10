/**
 * Mocked direct-navigation denial matrix (persona x route) — the
 * behavioural red run for W6-S7's role-level route guard
 * (routeManifest.requiredRole + a RequireRole component that redirects
 * before a denied page ever renders).
 *
 * Contract (mirrors internal/mgmtapi/rpc_interceptor.go):
 *   - admin/* requires app-admin. None of orgAdmin, orgEditor, reader or
 *     nobody carry isAppAdmin, so all four are denied on every admin/*
 *     route below.
 *   - /teams carries no requiredRole beyond being a member of the selected
 *     org (routeManifest.ts S7 only assigns 'app-admin' to admin/* and
 *     'org-admin' to /git and /audit — /teams is unlisted, i.e. the reader
 *     floor). orgAdmin, orgEditor and reader all belong to org-0001 and are
 *     let through; nobody belongs to no org at all and is denied.
 * A denial is either a router-level redirect to '/' or a visible
 * [data-testid="route-denied"] element, and — the part a redirect alone
 * cannot prove — the page's privileged RPC for that route must never have
 * fired, i.e. the denied page's data never left the guard to render.
 *
 * This is the RED run for W6-S7, which has not landed on this branch: every
 * denial case below fails today (routeManifest carries no requiredRole and
 * there is no RequireRole component, so every persona reaches every route
 * and its RPC fires). Wrapped in test.describe.skip for that reason —
 * unskip when W6-S7 merges, and reconcile the /teams assumption above if
 * W6-S7 chose a different tier for it.
 */
import type { Page } from '@playwright/test';
import { org } from '../fixtures/factories';
import { nobody, orgAdmin, orgEditor, reader } from '../fixtures/personas';
import { expect, test } from '../fixtures/test';

const ORG = org({ id: 'org-0001' });

// Local, minimal view of the `api` fixture — just the one method this file
// needs. `ApiFixture` in fixtures/test.ts is not exported (that file is
// outside this workstream's territory), so this is declared rather than
// imported.
interface CallsFixture {
  calls: (pattern: string) => Array<{ method: string; path: string; body: unknown }>;
}

async function expectDenied(page: Page, api: CallsFixture, route: string, rpcPattern: string) {
  await page.goto(route);
  await expect(async () => {
    const onRoot = new URL(page.url()).pathname === '/';
    const hasDeniedBanner = await page
      .getByTestId('route-denied')
      .isVisible()
      .catch(() => false);
    expect(onRoot || hasDeniedBanner).toBe(true);
  }).toPass({ timeout: 5000 });
  expect(api.calls(rpcPattern)).toHaveLength(0);
}

async function expectAllowed(page: Page, route: string) {
  await expect(async () => {
    expect(new URL(page.url()).pathname).toBe(route);
  }).toPass({ timeout: 5000 });
  await expect(page.getByTestId('route-denied')).toHaveCount(0);
}

test.describe
  .skip('route-guard: persona x route denial matrix (unskip when W6-S7 merges)', () => {
    test('orgAdmin is denied /admin/orgs', async ({ page, api }) => {
      await api.loginAs(orgAdmin);
      api.seed({ orgs: [ORG] });
      await expectDenied(page, api, '/admin/orgs', 'AdminService/ListOrgs');
    });

    test('orgAdmin is denied /admin/users', async ({ page, api }) => {
      await api.loginAs(orgAdmin);
      api.seed({ orgs: [ORG] });
      await expectDenied(page, api, '/admin/users', 'UserService/ListUsers');
    });

    test('orgAdmin is denied /admin/auth', async ({ page, api }) => {
      await api.loginAs(orgAdmin);
      api.seed({ orgs: [ORG] });
      await expectDenied(page, api, '/admin/auth', 'AdminService/GetOidcSettings');
    });

    test('orgAdmin is allowed /teams', async ({ page, api }) => {
      await api.loginAs(orgAdmin);
      api.seed({ orgs: [ORG] });
      await page.goto('/teams');
      await expectAllowed(page, '/teams');
    });

    test('orgEditor is denied /admin/orgs', async ({ page, api }) => {
      await api.loginAs(orgEditor);
      api.seed({ orgs: [ORG] });
      await expectDenied(page, api, '/admin/orgs', 'AdminService/ListOrgs');
    });

    test('orgEditor is denied /admin/users', async ({ page, api }) => {
      await api.loginAs(orgEditor);
      api.seed({ orgs: [ORG] });
      await expectDenied(page, api, '/admin/users', 'UserService/ListUsers');
    });

    test('orgEditor is denied /admin/auth', async ({ page, api }) => {
      await api.loginAs(orgEditor);
      api.seed({ orgs: [ORG] });
      await expectDenied(page, api, '/admin/auth', 'AdminService/GetOidcSettings');
    });

    test('orgEditor is allowed /teams', async ({ page, api }) => {
      await api.loginAs(orgEditor);
      api.seed({ orgs: [ORG] });
      await page.goto('/teams');
      await expectAllowed(page, '/teams');
    });

    test('reader is denied /admin/orgs', async ({ page, api }) => {
      await api.loginAs(reader);
      api.seed({ orgs: [ORG] });
      await expectDenied(page, api, '/admin/orgs', 'AdminService/ListOrgs');
    });

    test('reader is denied /admin/users', async ({ page, api }) => {
      await api.loginAs(reader);
      api.seed({ orgs: [ORG] });
      await expectDenied(page, api, '/admin/users', 'UserService/ListUsers');
    });

    test('reader is denied /admin/auth', async ({ page, api }) => {
      await api.loginAs(reader);
      api.seed({ orgs: [ORG] });
      await expectDenied(page, api, '/admin/auth', 'AdminService/GetOidcSettings');
    });

    test('reader is allowed /teams', async ({ page, api }) => {
      await api.loginAs(reader);
      api.seed({ orgs: [ORG] });
      await page.goto('/teams');
      await expectAllowed(page, '/teams');
    });

    test('nobody is denied /admin/orgs', async ({ page, api }) => {
      await api.loginAs(nobody);
      api.seed({});
      await expectDenied(page, api, '/admin/orgs', 'AdminService/ListOrgs');
    });

    test('nobody is denied /admin/users', async ({ page, api }) => {
      await api.loginAs(nobody);
      api.seed({});
      await expectDenied(page, api, '/admin/users', 'UserService/ListUsers');
    });

    test('nobody is denied /admin/auth', async ({ page, api }) => {
      await api.loginAs(nobody);
      api.seed({});
      await expectDenied(page, api, '/admin/auth', 'AdminService/GetOidcSettings');
    });

    test('nobody is denied /teams', async ({ page, api }) => {
      await api.loginAs(nobody);
      api.seed({});
      await expectDenied(page, api, '/teams', 'TeamService/ListTeams');
    });
  });
