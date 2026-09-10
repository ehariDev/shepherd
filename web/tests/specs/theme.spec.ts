import { basicScenario } from '../fixtures/factories';
import { appAdmin } from '../fixtures/personas';
import { schemaFixture } from '../fixtures/schema-fixture';
import { expect, test } from '../fixtures/test';

// D8 (light mode, W6-S1). Contract: with no stored choice,
// prefers-color-scheme decides (pure CSS, no login/JS needed — exercised on
// the unauthenticated /login route below); the toggle writes an explicit
// "light"|"dark" to localStorage['theme'] and sets html.light/html.dark,
// which overrides the system preference and survives a reload.

async function bodyBackground(page: import('@playwright/test').Page): Promise<string> {
  return page.evaluate(() => getComputedStyle(document.body).backgroundColor);
}

test.describe('system preference decides with no stored choice', () => {
  test('emulated light and dark color schemes render different body backgrounds', async ({
    page,
    api,
  }) => {
    // No loginAs(): default mocked state has me:null, so /login renders with
    // no Shell (and no JS theme logic) involved — this is pure index.css.
    await page.emulateMedia({ colorScheme: 'dark' });
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: /sign in to shepherd/i })).toBeVisible();
    const darkBg = await bodyBackground(page);

    await page.emulateMedia({ colorScheme: 'light' });
    // Force a fresh evaluation of the media query: a navigation is not
    // required for emulateMedia to take effect, but re-asserting visibility
    // proves the page is still alive.
    await expect(page.getByRole('heading', { name: /sign in to shepherd/i })).toBeVisible();
    const lightBg = await bodyBackground(page);

    expect(lightBg).not.toEqual(darkBg);
    // Nothing was ever stored — the switch is driven purely by the media
    // query, not by any leftover localStorage state from a prior test.
    const stored = await page.evaluate(() => localStorage.getItem('theme'));
    expect(stored).toBeNull();
  });
});

test.describe('explicit toggle overrides the system preference and persists', () => {
  test('clicking the toggle sets an explicit theme that survives reload and beats the OS preference', async ({
    page,
    api,
  }) => {
    await api.loginAs(appAdmin);
    const s = basicScenario();
    api.seed({ orgs: [s.org] });

    // Start dark (matches playwright.config.ts's project default), with no
    // stored choice yet.
    await page.emulateMedia({ colorScheme: 'dark' });
    await page.goto('/');
    const html = page.locator('html');
    await expect(html).not.toHaveClass(/light/);
    const beforeBg = await bodyBackground(page);

    const themeBtn = page.getByRole('button', { name: /toggle theme/i });
    await expect(themeBtn).toBeVisible();
    await themeBtn.click();

    await expect(html).toHaveClass(/light/);
    await expect(html).not.toHaveClass(/dark/);
    const afterBg = await bodyBackground(page);
    expect(afterBg).not.toEqual(beforeBg);
    expect(await page.evaluate(() => localStorage.getItem('theme'))).toBe('light');

    // Reload with the OS still reporting dark: the explicit 'light' choice
    // must win over prefers-color-scheme, and survive the reload.
    await page.reload();
    await expect(html).toHaveClass(/light/);
    await expect(html).not.toHaveClass(/dark/);
    expect(await bodyBackground(page)).toEqual(afterBg);
  });
});

// W5/W6-D: PipelineNode and CanvasPane read wire/category colours through
// schemaAdapter's theme-aware getThemedWireColor/getThemedCategoryColor
// (added by W6-S1) instead of the dark-only getWireColor/getCategoryColor —
// so the canvas itself, not just chrome around it, follows the toggle.
test.describe('visual canvas colours follow the theme', () => {
  test("a placed node's port wire colour changes with the theme toggle", async ({ page, api }) => {
    await api.loginAs(appAdmin);
    const s = basicScenario();
    api.seed({ orgs: [s.org], schema: schemaFixture });

    await page.emulateMedia({ colorScheme: 'dark' });
    await page.goto('/pipelines/visual/new');
    await page.waitForSelector('[data-testid="visual-builder"]', { timeout: 10_000 });
    await page.waitForSelector('[data-testid="palette-search"]', { timeout: 8_000 });

    // prometheus.scrape's `targets` port (a data-kind argument, D1's one
    // "accepts" exception — see PipelineNode's own comment) uses the
    // 'targets' wire type, which carries a distinct overlay `color_light`
    // (internal/schema/artifacts/overlay.json) — the port handle's colour is
    // set unconditionally (unlike the node's category-colour left border,
    // which a fresh unconnected node's own required-argument errors
    // override with a fixed red), so it's the reliable signal here.
    await page.click('[data-component="prometheus.scrape"]');
    const node = page.locator('[data-testid="pipeline-node"]').first();
    await expect(node).toBeVisible();
    const handle = node.locator('.react-flow__handle').first();
    await expect(handle).toBeVisible();

    const darkHandle = await handle.evaluate((el) => getComputedStyle(el).backgroundColor);

    const themeBtn = page.getByRole('button', { name: /toggle theme/i });
    await themeBtn.click();
    await expect(page.locator('html')).toHaveClass(/light/);

    // The toggle only changes Shell's own local state — this node is
    // `memo`-wrapped and receives no new props from that, so the colour
    // functions themselves being theme-aware isn't enough on its own; wait
    // for the value to actually change rather than reading once.
    await expect
      .poll(async () => handle.evaluate((el) => getComputedStyle(el).backgroundColor))
      .not.toBe(darkHandle);
  });
});
