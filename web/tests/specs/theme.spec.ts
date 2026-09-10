import { basicScenario } from '../fixtures/factories';
import { appAdmin } from '../fixtures/personas';
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
