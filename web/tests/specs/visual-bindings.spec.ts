// visual-bindings.spec.ts — W5-02
// SecretField's binding picker: wiring an existing remote.kubernetes.secret
// node into prometheus.remote_write's endpoint.basic_auth.password, and the
// unified "Bound" display covering the new props-based binding channel.
import { expect, type Page } from '@playwright/test';
import { basicScenario } from '../fixtures/factories';
import { appAdmin } from '../fixtures/personas';
import { schemaFixture } from '../fixtures/schema-fixture';
import { test } from '../fixtures/test';

/**
 * Waits out React Flow's fitView pan/zoom transition (`.react-flow__viewport`'s
 * inline `transform`) instead of a flat page.waitForTimeout (W7-13):
 * the follow-up click below uses `{ force: true }`, which skips
 * Playwright's own actionability "stable target" check, so this is the
 * only thing standing between the click and a moving target. Polls until
 * the transform stops changing across two consecutive reads rather than
 * blocking for a fixed real-time window regardless of how long the
 * transition actually takes.
 */
async function waitForViewportSettled(page: Page): Promise<void> {
  const viewport = page.locator('.react-flow__viewport');
  let last: string | null = null;
  await expect
    .poll(
      async () => {
        const current = await viewport.getAttribute('style');
        const settled = last !== null && current === last;
        last = current;
        return settled;
      },
      { timeout: 2000 },
    )
    .toBe(true);
}

test.describe('visual bindings', () => {
  test.beforeEach(async ({ page, api }) => {
    await api.loginAs(appAdmin);
    const s = basicScenario();
    api.seed({ orgs: [s.org], schema: schemaFixture });
    await page.goto('/pipelines/visual/new');
    await page.waitForSelector('[data-testid="visual-builder"]', { timeout: 10_000 });
    await page.waitForSelector('[data-testid="palette-search"]', { timeout: 8_000 });
  });

  test('binds an existing secret-source node into a nested secret field, and the code tab shows the reference', async ({
    page,
  }) => {
    // Place the secret source and the destination.
    await page.click('[data-component="remote.kubernetes.secret"]');
    await page.click('[data-component="prometheus.remote_write"]');
    await expect(page.locator('[data-testid="pipeline-node"]')).toHaveCount(2);

    // Select the remote_write node (it was placed second).
    await page.locator('.react-flow__controls-fitview').click();
    await waitForViewportSettled(page);
    const remoteWriteId = await page
      .locator('[data-testid="pipeline-node"]')
      .last()
      .getAttribute('data-node-id');
    await page.click(`[data-node-id="${remoteWriteId}"]`, { force: true });
    await expect(page.locator('[data-testid="inspector"]')).toContainText(
      'prometheus.remote_write',
      {
        timeout: 5_000,
      },
    );

    // Open an endpoint instance, then its basic_auth block, to reach `password`.
    await page.click('[data-testid="block-add-endpoint"]');
    await page.click('[data-testid="block-add-endpoint.basic_auth"]');

    const picker = page.locator('[data-testid="attr-binding-source-password"]');
    await expect(picker).toBeVisible({ timeout: 5_000 });

    await picker.selectOption({ label: 'secret (remote.kubernetes.secret)' });
    await page.fill('[data-testid="attr-binding-key-password"]', 'password');
    await page.click('[data-testid="attr-binding-bind-password"]');

    await expect(page.locator('[data-testid="attr-secret-password"]')).toContainText(
      'remote.kubernetes.secret.secret.data["password"]',
    );

    await page.click('[data-testid="drawer-toggle"]');
    await page.click('[data-testid="drawer-tab-code"]');
    await expect(page.locator('[data-testid="code-tab-content"]')).toContainText(
      'remote.kubernetes.secret.secret.data["password"]',
      { timeout: 5_000 },
    );
  });

  test('unbind clears the field back to the picker', async ({ page }) => {
    await page.click('[data-component="remote.kubernetes.secret"]');
    await page.click('[data-component="prometheus.remote_write"]');
    await page.locator('.react-flow__controls-fitview').click();
    await waitForViewportSettled(page);
    const remoteWriteId = await page
      .locator('[data-testid="pipeline-node"]')
      .last()
      .getAttribute('data-node-id');
    await page.click(`[data-node-id="${remoteWriteId}"]`, { force: true });
    await page.click('[data-testid="block-add-endpoint"]');
    await page.click('[data-testid="block-add-endpoint.basic_auth"]');

    await page
      .locator('[data-testid="attr-binding-source-password"]')
      .selectOption({ label: 'secret (remote.kubernetes.secret)' });
    await page.fill('[data-testid="attr-binding-key-password"]', 'password');
    await page.click('[data-testid="attr-binding-bind-password"]');
    await expect(page.locator('[data-testid="attr-secret-password"]')).toContainText('Bound');

    await page.click('[data-testid="attr-binding-unbind-password"]');

    await expect(page.locator('[data-testid="attr-binding-source-password"]')).toBeVisible();
    await expect(page.locator('[data-testid="attr-secret-password"]')).not.toContainText('Bound');
  });
});
