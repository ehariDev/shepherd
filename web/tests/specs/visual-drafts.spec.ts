// visual-drafts.spec.ts — W5-05: IndexedDB draft autosave and the
// restore-or-discard banner (design §4.4).
import { expect, type Page } from '@playwright/test';
import { basicScenario } from '../fixtures/factories';
import { appAdmin } from '../fixtures/personas';
import { schemaFixture } from '../fixtures/schema-fixture';
import { test } from '../fixtures/test';

/**
 * Polls for the debounced IndexedDB autosave (subscribeDraftAutosave,
 * draft.ts, production delayMs default 500ms) actually landing for
 * `pipelineId`, instead of a flat page.waitForTimeout comfortably past the
 * debounce (W7-13: waitForTimeout is real-time and either flaky or padded).
 * expect.poll moves on the moment the write lands and still tolerates a
 * loaded CI/dev machine delaying well past 500ms. Reads idb-keyval's
 * default store directly (DB 'keyval-store', object store 'keyval') since
 * that's the actual thing under test — the store instance itself, not a
 * proxy for it.
 */
async function waitForDraftSaved(page: Page, pipelineId: string): Promise<void> {
  await expect
    .poll(
      () =>
        page.evaluate(
          (key) =>
            new Promise<boolean>((resolve) => {
              const openReq = indexedDB.open('keyval-store');
              openReq.onerror = () => resolve(false);
              openReq.onsuccess = () => {
                const db = openReq.result;
                if (!db.objectStoreNames.contains('keyval')) {
                  resolve(false);
                  return;
                }
                const getReq = db.transaction('keyval', 'readonly').objectStore('keyval').get(key);
                getReq.onsuccess = () => resolve(getReq.result !== undefined);
                getReq.onerror = () => resolve(false);
              };
            }),
          `vb:draft:${pipelineId}`,
        ),
      { timeout: 5000 },
    )
    .toBe(true);
}

test.describe('visual builder drafts', () => {
  test.beforeEach(async ({ page, api }) => {
    await api.loginAs(appAdmin);
    const s = basicScenario();
    api.seed({ orgs: [s.org], schema: schemaFixture });
    await page.goto('/pipelines/visual/new');
    await page.waitForSelector('[data-testid="visual-builder"]', { timeout: 10_000 });
    await page.waitForSelector('[data-testid="palette-search"]', { timeout: 8_000 });
  });

  test('a draft survives a reload and is offered for restore', async ({ page }) => {
    await page.click('[data-testid="palette-item-prometheus.remote_write"]');
    await expect(page.locator('.react-flow__node')).toHaveCount(1);
    await waitForDraftSaved(page, 'new');

    // The existing beforeunload handler (VisualBuilderPage.tsx) turns
    // page.reload() into a confirm dialog since the graph is non-empty;
    // accept it so the reload actually proceeds.
    page.on('dialog', (d) => {
      void d.accept();
    });
    await page.reload();
    await page.waitForSelector('[data-testid="visual-builder"]', { timeout: 10_000 });

    // A fresh mount starts from an empty doc; the autosaved draft (1 node)
    // differs from it, so the restore banner offers it.
    await expect(page.getByTestId('draft-restore-banner')).toBeVisible();
    await expect(page.locator('.react-flow__node')).toHaveCount(0);

    await page.getByTestId('draft-restore').click();
    await expect(page.getByTestId('draft-restore-banner')).not.toBeVisible();
    await expect(page.locator('.react-flow__node')).toHaveCount(1);
  });

  test('a draft can be discarded, and no longer offers itself after', async ({ page }) => {
    await page.click('[data-testid="palette-item-prometheus.remote_write"]');
    await expect(page.locator('.react-flow__node')).toHaveCount(1);
    await waitForDraftSaved(page, 'new');

    page.on('dialog', (d) => {
      void d.accept();
    });
    await page.reload();
    await page.waitForSelector('[data-testid="visual-builder"]', { timeout: 10_000 });
    await expect(page.getByTestId('draft-restore-banner')).toBeVisible();

    await page.getByTestId('draft-discard').click();
    await expect(page.getByTestId('draft-restore-banner')).not.toBeVisible();
    await expect(page.locator('.react-flow__node')).toHaveCount(0);

    // Reloading again (nothing to warn about — the canvas is empty) must not
    // resurrect the discarded draft.
    await page.reload();
    await page.waitForSelector('[data-testid="visual-builder"]', { timeout: 10_000 });
    await expect(page.getByTestId('draft-restore-banner')).not.toBeVisible();
  });

  test('no banner appears for a route with no draft', async ({ page }) => {
    await expect(page.getByTestId('draft-restore-banner')).not.toBeVisible();
  });

  test("the 'new' draft is cleared once the create actually succeeds", async ({ page, api }) => {
    api.seed({
      visualRenderResult: { content: '// generated\n', node_map: {}, diagnostics: [] },
    });
    await page.click('[data-testid="palette-item-prometheus.remote_write"]');
    await expect(page.locator('.react-flow__node')).toHaveCount(1);
    await waitForDraftSaved(page, 'new');

    await page.locator('[data-testid="toolbar-name"]').fill('checkout-metrics');
    const input = page.locator('[data-testid="matcher-input"]');
    await input.fill('cluster="prod-eu-1"');
    await input.press('Enter');
    await page.locator('[data-testid="toolbar-save"]').click();
    await expect(page).toHaveURL(/\/pipelines\/pip-\d+$/, { timeout: 5_000 });

    // A crash mid-authoring of a NEXT new pipeline must not resurrect the
    // just-saved one's draft — it was cleared on the successful create.
    await page.goto('/pipelines/visual/new');
    await page.waitForSelector('[data-testid="visual-builder"]', { timeout: 10_000 });
    await expect(page.getByTestId('draft-restore-banner')).not.toBeVisible();
  });
});
