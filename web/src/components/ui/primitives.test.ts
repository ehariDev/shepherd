import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

/*
 * S3/S4: every page-level table repeated the same
 * `<thead className='bg-card text-muted'>` markup, and every page-level
 * form input repeated the same border/background/padding class literal.
 * This is the drift guard: once a page migrates onto components/ui/DataTable
 * (S3) or components/ui/Field+Input+Select+Textarea (S4), its source can no
 * longer contain the literal it used to hand-roll.
 *
 * GitPage.tsx, AdminUsersPage.tsx and AdminAuthPage.tsx are excused by name —
 * they are split onto the primitives in S9a-c, outside this workstream.
 */

const PAGES_DIR = join(__dirname, '..', '..', 'pages');

const EXCUSED_UNTIL_S9 = new Set(['GitPage.tsx', 'AdminUsersPage.tsx', 'AdminAuthPage.tsx']);

// The nine small table pages S3 migrates onto DataTable.
const S3_TABLE_PAGES = [
  'DestinationsPage.tsx',
  'CollectorDetailPage.tsx',
  'CollectorsPage.tsx',
  'TeamsPage.tsx',
  'PipelinesPage.tsx',
  'AdminClustersPage.tsx',
  'AuditPage.tsx',
  'AdminTokensPage.tsx',
  'AdminOrgsPage.tsx',
];

// S4 additionally covers PipelineEditorPage, which has no table but does
// repeat the input literal.
const S4_INPUT_PAGES = [...S3_TABLE_PAGES, 'PipelineEditorPage.tsx'];

const THEAD_LITERAL = "<thead className='bg-card text-muted'>";
const INPUT_LITERAL = 'border-border-strong bg-card px-3 py-1.5 text-sm';

function readPage(name: string): string {
  return readFileSync(join(PAGES_DIR, name), 'utf8');
}

describe('DataTable migration (S3)', () => {
  it.each(S3_TABLE_PAGES)('%s does not hand-roll a table thead', (name) => {
    expect(readPage(name)).not.toContain(THEAD_LITERAL);
  });
});

describe('Field/Input/Select migration (S4)', () => {
  it.each(S4_INPUT_PAGES)('%s does not hand-roll the input border/padding literal', (name) => {
    expect(readPage(name)).not.toContain(INPUT_LITERAL);
  });

  it.each([...EXCUSED_UNTIL_S9])('%s is excused until S9 lands', (name) => {
    // Documents the exemption rather than asserting anything about content —
    // if one of these pages stops needing the exemption it should move up
    // into the enforced list above, not silently vanish from this test.
    expect(EXCUSED_UNTIL_S9.has(name)).toBe(true);
  });
});
