/**
 * waitForTimeout budget (W7-13).
 *
 * `page.waitForTimeout` is a real-time sleep with no relationship to what
 * the page is actually doing — it is either too short (flaky) or padded
 * well past what's needed (slow), and it says nothing about *why* the test
 * is waiting. It has two legitimate uses in this suite: the visual canvas
 * specs, where a drag/highlight/layout animation genuinely has no other
 * observable signal, and nothing else — every other wait belongs on a
 * locator assertion (which polls) or a faked page.clock (which is
 * deterministic).
 *
 * This guard source-scans tests/specs/*.spec.ts (excluding this file
 * itself) so it also catches a wait added to a *new* non-visual spec, not
 * just the six migrated here.
 */
import { readdirSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';

const SPECS_DIR = path.dirname(fileURLToPath(import.meta.url));
const SELF = path.basename(fileURLToPath(import.meta.url));
const WAIT_BUDGET = 25;
const WAIT_RE = /waitForTimeout\(/g;

function specFiles(): string[] {
  return readdirSync(SPECS_DIR).filter((f) => f.endsWith('.spec.ts') && f !== SELF);
}

test('only visual specs use waitForTimeout, and the total stays under budget', () => {
  const offenders: string[] = [];
  let total = 0;

  for (const file of specFiles()) {
    const source = readFileSync(path.join(SPECS_DIR, file), 'utf8');
    const isVisual = file.startsWith('visual-');
    const lines = source.split('\n');
    lines.forEach((lineText, i) => {
      const hits = lineText.match(WAIT_RE);
      if (!hits) return;
      total += hits.length;
      if (!isVisual) offenders.push(`${file}:${i + 1}`);
    });
  }

  expect(offenders, 'waitForTimeout outside a visual-*.spec.ts file').toEqual([]);
  expect(total, 'total waitForTimeout calls across tests/specs').toBeLessThanOrEqual(WAIT_BUDGET);
});
