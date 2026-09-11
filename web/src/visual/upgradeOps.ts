/**
 * upgradeOps.ts — W5-04: pure helpers UpgradeReview applies against an
 * UpgradeCheck result.
 *
 * Deliberately NOT web/src/api/client.ts (stampSchemaVersion lives there,
 * outside this workstream's territory, and stays as-is): this module only
 * turns an UpgradeItem[] into document edits, with no wire/transport
 * concerns.
 */
import type { UpgradeItem } from '../api/client';
import type { GraphDocument } from './types';

/**
 * Removes every prop an `attr_removed` diff item names, on the node it
 * names. upgrade.go only ever emits a flat top-level prop name in `detail`
 * for this class (blocks are explicitly excluded from the diff), so no path
 * parsing is needed — a bare `delete props[detail]` is exact.
 *
 * Returns the SAME `doc` reference when nothing was actually removed (no
 * matching items, or every named prop was already absent), so a caller can
 * skip a wasted store update; otherwise returns a new document with new node
 * objects only for the nodes that lost a prop — every other node keeps its
 * reference, matching the reconciler's identity contract (see reconcile.ts).
 */
export function pruneRemovedAttrs(doc: GraphDocument, items: UpgradeItem[]): GraphDocument {
  const removedByNode = new Map<string, Set<string>>();
  for (const item of items) {
    if (item.class !== 'attr_removed' || !item.detail) continue;
    let set = removedByNode.get(item.node_id);
    if (!set) {
      set = new Set();
      removedByNode.set(item.node_id, set);
    }
    set.add(item.detail);
  }
  if (removedByNode.size === 0) return doc;

  let anyChanged = false;
  const nodes = doc.nodes.map((n) => {
    const removed = removedByNode.get(n.id);
    if (!removed) return n;
    let nodeChanged = false;
    const props = { ...n.props };
    for (const key of removed) {
      if (key in props) {
        delete props[key];
        nodeChanged = true;
      }
    }
    if (!nodeChanged) return n;
    anyChanged = true;
    return { ...n, props };
  });

  return anyChanged ? { ...doc, nodes } : doc;
}

/**
 * True while the upgrade would leave the graph referencing a component that
 * no longer exists in the target schema — there is nothing a rendered save
 * could mean for that node, so Accept must be blocked until the graph is
 * fixed (the node removed, or re-pointed at a surviving component).
 */
export function hasBlockingItems(items: UpgradeItem[]): boolean {
  return items.some((item) => item.class === 'component_removed');
}
