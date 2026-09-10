import { describe, expect, it } from 'vitest';
import type { GraphEdge } from '../../types';
import { wireCountsFor, wireEdgesFor } from './portWiring';

const edge = (id: string, toNode: string, toPort: string, order?: number): GraphEdge => ({
  id,
  from: { node: 'src', port: 'out' },
  to: { node: toNode, port: toPort },
  order,
});

describe('wireEdgesFor (W5-08)', () => {
  it('returns only this node+port’s incoming edges, sorted by order', () => {
    const edges = [
      edge('e2', 'n', 'in', 1),
      edge('e0', 'n', 'in', 0),
      edge('other', 'n', 'elsewhere', 0),
    ];
    expect(wireEdgesFor(edges, 'n', 'in').map((e) => e.id)).toEqual(['e0', 'e2']);
  });

  it('treats a missing order as 0 for sort purposes', () => {
    const edges = [edge('withOrder', 'n', 'in', 1), edge('noOrder', 'n', 'in')];
    expect(wireEdgesFor(edges, 'n', 'in').map((e) => e.id)).toEqual(['noOrder', 'withOrder']);
  });

  it('is empty when nothing targets this node+port', () => {
    expect(wireEdgesFor([edge('e', 'n', 'in')], 'n', 'other')).toEqual([]);
  });
});

// Not previously covered by a dedicated unit test (Playwright-only until
// now) — added alongside wireEdgesFor since both live in this module.
describe('wireCountsFor', () => {
  it('counts both directions for one node', () => {
    const edges: GraphEdge[] = [
      { id: 'e1', from: { node: 'n', port: 'out' }, to: { node: 'x', port: 'in' } },
      { id: 'e2', from: { node: 'y', port: 'out' }, to: { node: 'n', port: 'in' } },
    ];
    expect(wireCountsFor(edges, 'n')).toEqual(
      new Map([
        ['out', 1],
        ['in', 1],
      ]),
    );
  });
});
