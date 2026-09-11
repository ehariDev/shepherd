import type { Page } from '@playwright/test';
import { basicScenario } from '../fixtures/factories';
import { appAdmin } from '../fixtures/personas';
import { expect, test } from '../fixtures/test';

// CodeMirror's completionKeymap binds Ctrl-Space on every platform — it does NOT
// bind Cmd-Space, which on macOS is Spotlight. Playwright's `ControlOrMeta`
// resolves to Meta there, so the previous `ControlOrMeta+Space` opened nothing
// locally while passing on CI's Linux runner; the in-block test then skipped
// itself and reported green on both. Always `Control+Space` here.
async function openEditor(page: Page) {
  await page.goto('/pipelines/new');
  // Wait for the CodeMirror editor to be available
  await page.waitForSelector('.cm-editor', { timeout: 5000 });
  const editor = page.locator('.cm-content');
  await editor.click();
  return editor;
}

// Named test 1: top-level completions appear
test('top-level completion shows component options', async ({ page, api }) => {
  await api.loginAs(appAdmin);
  api.seed({ orgs: [basicScenario().org] });
  const ed = await openEditor(page);
  // Type a component prefix to trigger autocomplete
  await ed.pressSequentially('prom');
  // No arbitrary wait for the activateOnTyping debounce: Control+Space
  // invokes CodeMirror's startCompletion command directly, bypassing the
  // typing debounce, and the toBeVisible below already polls for the
  // tooltip.
  await page.keyboard.press('Control+Space');
  const tooltip = page.locator('.cm-tooltip-autocomplete');
  await expect(tooltip).toBeVisible({ timeout: 5000 });
  await expect(tooltip).toContainText('prometheus');
});

// Named test 2: in-block attribute completion
test('in-block attribute completion appears after entering block', async ({ page, api }) => {
  await api.loginAs(appAdmin);
  api.seed({ orgs: [basicScenario().org] });
  const ed = await openEditor(page);
  await ed.pressSequentially('prometheus.scrape "test" {');
  await ed.press('Enter');
  // See the top-level completion test: the explicit trigger below bypasses
  // the typing debounce, so no arbitrary wait is needed here either.
  await page.keyboard.press('Control+Space');
  const tooltip = page.locator('.cm-tooltip-autocomplete');
  await expect(tooltip).toBeVisible({ timeout: 5000 });
  await expect(tooltip).toContainText(/targets|forward_to/);
});

// Named test 3: enum completion after =
test('enum completion appears after = for attribute with values', async ({ page, api }) => {
  await api.loginAs(appAdmin);
  api.seed({ orgs: [basicScenario().org] });
  const ed = await openEditor(page);
  await ed.pressSequentially('prometheus.scrape "test" {');
  await ed.press('Enter');
  await ed.pressSequentially('  scheme = ');
  // See the top-level completion test: the explicit trigger below bypasses
  // the typing debounce, so no arbitrary wait is needed here either.
  await page.keyboard.press('Control+Space');
  const tooltip = page.locator('.cm-tooltip-autocomplete');
  await expect(tooltip).toBeVisible({ timeout: 5000 });
  await expect(tooltip).toContainText(/http|https/);
});

// Named test 4: no completion inside comment context
test('no completion inside a comment', async ({ page, api }) => {
  await api.loginAs(appAdmin);
  api.seed({ orgs: [basicScenario().org] });
  const ed = await openEditor(page);
  await ed.pressSequentially('// this is a comment');
  // Fake the clock from here so "wait past the point where a tooltip would
  // have rendered" is a deterministic, instant clock advance rather than a
  // real sleep — this is a negative control, so unlike the other cases in
  // this file it cannot rely on a polling locator assertion alone (there is
  // nothing to poll for; the assertion is that nothing ever appears).
  await page.clock.install();
  await page.keyboard.press('Control+Space');
  await page.clock.runFor(500);
  // Deterministic: AlloyEditor registers alloyCompletionSource as the SOLE
  // completion source (autocompletion({ override: [...] })), and the source
  // returns null in comment context — so no autocomplete tooltip may exist.
  // Red run: remove the comment check from alloyCompletionSource → the tooltip
  // appears with component names and this fails.
  await expect(page.locator('.cm-tooltip-autocomplete')).toHaveCount(0);
});
