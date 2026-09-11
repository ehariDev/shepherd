import { describe, expect, it } from 'vitest';
import type { UpgradeItem } from '../api/client';
import type { GraphDocument, GraphNode } from './types';
import { hasBlockingItems, pruneRemovedAttrs } from './upgradeOps';

function node(id: string, props: Record<string, unknown> = {}): GraphNode {
  return {
    id,
    component: 'test.component',
    label: id,
    position: { x: 0, y: 0 },
    props,
    disabled: false,
    notes: '',
  };
}

function doc(nodes: GraphNode[]): GraphDocument {
  return {
    kind: 'alloy-graph/v1',
    schema_version: 'alloy-v1.12.0',
    nodes,
    edges: [],
    bindings: [],
    viewport: { x: 0, y: 0, zoom: 1 },
    meta: { created_with: 'test' },
  };
}

function item(overrides: Partial<UpgradeItem>): UpgradeItem {
  return {
    node_id: 'n1',
    node_label: 'n1',
    component: 'test.component',
    class: 'attr_removed',
    detail: 'gone',
    ...overrides,
  };
}

describe('pruneRemovedAttrs', () => {
  it('drops only the named top-level prop on the named node', () => {
    const d = doc([node('n1', { role: 'pod', gone: 'x' }), node('n2', { gone: 'y' })]);
    const result = pruneRemovedAttrs(d, [item({ node_id: 'n1', detail: 'gone' })]);
    expect(result.nodes.find((n) => n.id === 'n1')?.props).toEqual({ role: 'pod' });
    // n2 untouched — same object reference, since it was not in the removal set.
    expect(result.nodes.find((n) => n.id === 'n2')).toBe(d.nodes[1]);
  });

  it('ignores items that are not attr_removed', () => {
    const d = doc([node('n1', { role: 'pod' })]);
    const result = pruneRemovedAttrs(d, [
      item({ node_id: 'n1', class: 'stability_changed', detail: 'role' }),
    ]);
    expect(result.nodes[0].props).toEqual({ role: 'pod' });
  });

  it('ignores an attr_removed item with no detail', () => {
    const d = doc([node('n1', { role: 'pod' })]);
    const result = pruneRemovedAttrs(d, [item({ node_id: 'n1', detail: undefined })]);
    expect(result.nodes[0].props).toEqual({ role: 'pod' });
  });

  it('is a no-op (returns the same document reference) when there is nothing to prune', () => {
    const d = doc([node('n1', { role: 'pod' })]);
    const result = pruneRemovedAttrs(d, []);
    expect(result).toBe(d);
  });

  it('prunes multiple removed props on the same node from separate items', () => {
    const d = doc([node('n1', { a: 1, b: 2, c: 3 })]);
    const result = pruneRemovedAttrs(d, [
      item({ node_id: 'n1', detail: 'a' }),
      item({ node_id: 'n1', detail: 'c' }),
    ]);
    expect(result.nodes[0].props).toEqual({ b: 2 });
  });
});

describe('hasBlockingItems', () => {
  it('is true when a component_removed item is present', () => {
    expect(hasBlockingItems([item({ class: 'component_removed', detail: undefined })])).toBe(true);
  });

  it('is false when every item is a non-blocking class', () => {
    expect(
      hasBlockingItems([
        item({ class: 'attr_removed', detail: 'gone' }),
        item({ class: 'migration_available', detail: 'x' }),
      ]),
    ).toBe(false);
  });

  it('is false for an empty item list', () => {
    expect(hasBlockingItems([])).toBe(false);
  });
});
