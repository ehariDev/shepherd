// Source scan, same pattern as web/tests/fullstack/protected-routes.spec.ts:
// router.tsx is the only source that cannot drift from the routes the app
// actually serves, so its wiring is pinned by reading the file rather than
// mounting a full RouterProvider tree.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const source = readFileSync(new URL('./router.tsx', import.meta.url), 'utf8');

describe('router.tsx error-boundary wiring', () => {
  it('wires RouteErrorFallback as the router-wide defaultErrorComponent', () => {
    expect(source).toMatch(/defaultErrorComponent:\s*RouteErrorFallback/);
  });

  it('imports RouteErrorFallback from the shared component, not an inline element', () => {
    expect(source).toMatch(
      /import\s*\{\s*RouteErrorFallback\s*\}\s*from\s*'@\/components\/RouteErrorFallback'/,
    );
  });

  it('no longer renders the old inline red-text error div on rootRoute', () => {
    // TanStack Router resolves error boundaries per leaf match: a rootRoute
    // errorComponent never actually sees a page crash, so it must be gone,
    // not just replaced with a nicer inline element.
    expect(source).not.toContain("color: 'red'");
    expect(source).not.toContain('Router Error:');
    expect(source).not.toContain('errorComponent:');
  });
});
