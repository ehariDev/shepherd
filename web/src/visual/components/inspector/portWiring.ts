/**
 * Bridges the D1 port model into the inspector: which attribute paths are
 * also ports, and how many wires currently land on/leave each one. Reuses
 * `l1.ts`'s `resolvePorts` (the one place the role/path resolution lives)
 * rather than re-deriving it, so the inspector's idea of "this field is
 * wired" can never drift from the validator's.
 */
import { type ResolvedPort, resolvePorts } from '../../l1';
import type { GraphEdge } from '../../types';
import type { ComponentLike } from './schemaShapes';

export interface PortWireIndex {
  /** Dotted schema path (`"targets"`, `"output.metrics"`) -> the port at it.
   *  Only argument-origin ports matter here — an export never doubles as an
   *  attribute, so it can never appear in `def.attributes`. */
  byPath: Map<string, ResolvedPort>;
}

export function buildPortWireIndex(def: ComponentLike | undefined): PortWireIndex {
  const byPath = new Map<string, ResolvedPort>();
  for (const port of resolvePorts(def))
    if (port.origin === 'argument') byPath.set(port.path.join('.'), port);
  return { byPath };
}

/** Wire count per port id, both directions, for one node. A component that
 *  reuses one port id for both an accepts and a produces role (the
 *  `database_observability.*` family, per l1.ts's own comment) will over-count
 *  here — a minor imprecision, not a correctness issue, since the inspector
 *  only uses the count to decide "show this field as wired", not to validate. */
export function wireCountsFor(edges: GraphEdge[], nodeId: string): Map<string, number> {
  const counts = new Map<string, number>();
  const bump = (id: string) => counts.set(id, (counts.get(id) ?? 0) + 1);
  for (const e of edges) {
    if (e.from.node === nodeId) bump(e.from.port);
    if (e.to.node === nodeId) bump(e.to.port);
  }
  return counts;
}

/** The edges landing on one ACCEPTS port of one node — the `to` side only,
 *  since only an argument-origin (accepts) port ever backs an attribute
 *  field — sorted by `order` (missing treated as 0, matching both
 *  renderers' own fallback). Backs the inspector's minimal fan-in reorder
 *  control (W5-08): `WiredRow` shows one row per entry here, in this order. */
export function wireEdgesFor(edges: GraphEdge[], nodeId: string, portId: string): GraphEdge[] {
  return edges
    .filter((e) => e.to.node === nodeId && e.to.port === portId)
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
}
