import { describe, expect, it } from 'vitest';
import { routeManifest } from '@/routes/routeManifest';
import { buildCrumbs } from './breadcrumb';

const noName = () => undefined;

describe('buildCrumbs', () => {
  it('/admin/users -> Admin, Users (admin has no route of its own)', () => {
    expect(buildCrumbs('/admin/users', routeManifest, noName)).toEqual([
      { label: 'Admin' },
      { label: 'Users' },
    ]);
  });

  it('/ -> Overview, the text the manifest locator relies on', () => {
    expect(buildCrumbs('/', routeManifest, noName)).toEqual([{ label: 'Overview' }]);
  });

  it('/collectors/col-0001 -> Collectors, Collector (generic — no resolver hit)', () => {
    expect(buildCrumbs('/collectors/col-0001', routeManifest, noName)).toEqual([
      { label: 'Collectors' },
      { label: 'Collector' },
    ]);
  });

  it('/pipelines/new -> Pipelines, New pipeline', () => {
    expect(buildCrumbs('/pipelines/new', routeManifest, noName)).toEqual([
      { label: 'Pipelines' },
      { label: 'New pipeline' },
    ]);
  });

  it('/pipelines/$id/visual -> Pipelines, Pipeline, Visual builder', () => {
    expect(buildCrumbs('/pipelines/pip-0001/visual', routeManifest, noName)).toEqual([
      { label: 'Pipelines' },
      { label: 'Pipeline' },
      { label: 'Visual builder' },
    ]);
  });

  it('calls the resolver with the matched route and its params, and uses a resolved name over the generic label', () => {
    const seen: Array<{ path: string; params: Record<string, string> }> = [];
    const resolveName = (route: { path: string }, params: Record<string, string>) => {
      seen.push({ path: route.path, params });
      return route.path === '/pipelines/$id' ? 'checkout-pipeline' : undefined;
    };
    const crumbs = buildCrumbs('/pipelines/pip-0001', routeManifest, resolveName);
    expect(crumbs).toEqual([{ label: 'Pipelines' }, { label: 'checkout-pipeline' }]);
    expect(seen).toContainEqual({ path: '/pipelines/$id', params: { id: 'pip-0001' } });
  });

  it('/git -> Git sync (single segment, matches its own route directly)', () => {
    expect(buildCrumbs('/git', routeManifest, noName)).toEqual([{ label: 'Git sync' }]);
  });
});
