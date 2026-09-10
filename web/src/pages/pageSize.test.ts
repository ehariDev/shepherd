import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

/*
 * S9a-c: GitPage.tsx, AdminUsersPage.tsx and AdminAuthPage.tsx grew past the
 * point one file can reasonably hold a whole feature area -- 796, 668 and
 * 654 lines, respectively -- because every dialog, form and table stayed
 * inline instead of moving onto the shared primitives S2-S4 introduced
 * (Modal, DataTable, Field/Input/Select/Textarea, Banner, QueryError). This
 * is the guard against that happening again: no file under src/pages or
 * src/components may grow past 500 lines.
 */

const MAX_LINES = 500;
const SRC_DIR = join(__dirname, '..');
const ROOTS = ['pages', 'components'];
const CODE_FILE = /\.(tsx?|jsx?)$/;
const TEST_FILE = /\.(test|spec)\.(tsx?|jsx?)$/;

function collectCodeFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...collectCodeFiles(full));
    } else if (CODE_FILE.test(entry.name) && !TEST_FILE.test(entry.name)) {
      out.push(full);
    }
  }
  return out;
}

function lineCount(path: string): number {
  return readFileSync(path, 'utf8').split('\n').length;
}

describe('file size (S9a-c)', () => {
  it('no file under src/pages or src/components exceeds 500 lines', () => {
    const oversized = ROOTS.flatMap((root) => collectCodeFiles(join(SRC_DIR, root)))
      .map((path) => ({ path: path.slice(SRC_DIR.length + 1), lines: lineCount(path) }))
      .filter(({ lines }) => lines > MAX_LINES);
    expect(oversized).toEqual([]);
  });
});
