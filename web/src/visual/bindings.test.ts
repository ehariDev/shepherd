import { describe, expect, it } from 'vitest';
import {
  buildBindingExpr,
  deleteAtPath,
  EXPR_KEY,
  exprOf,
  isExprValue,
  SECRET_EXPORT_FIELD,
  secretSourceNodes,
  setAtPath,
} from './bindings';
import type { ComponentDef, GraphDocument, GraphNode, SchemaPayload } from './types';

const node = (id: string, component: string, extra: Partial<GraphNode> = {}): GraphNode => ({
  id,
  component,
  label: id,
  position: { x: 0, y: 0 },
  props: {},
  disabled: false,
  notes: '',
  ...extra,
});

const doc = (nodes: GraphNode[]): Pick<GraphDocument, 'nodes'> => ({ nodes });

const minimalComponent = (extra: Partial<ComponentDef> = {}): ComponentDef => ({
  stability: 'ga',
  doc: '',
  attributes: [],
  blocks: [],
  inputs: [],
  outputs: [],
  default_snippet: '',
  ...extra,
});

describe('setAtPath / deleteAtPath', () => {
  it('sets a top-level scalar', () => {
    expect(setAtPath({ a: 1 }, ['b'], 2)).toEqual({ a: 1, b: 2 });
  });

  it('sets a value nested inside a repeatable block instance, creating intermediate containers', () => {
    const props = {
      endpoint: [{ url: 'http://x', basic_auth: { username: 'u' } }],
    };
    const next = setAtPath(props, ['endpoint', '0', 'basic_auth', 'password'], {
      [EXPR_KEY]: 'local.file.c.content',
    });
    expect(next).toEqual({
      endpoint: [
        {
          url: 'http://x',
          basic_auth: { username: 'u', password: { [EXPR_KEY]: 'local.file.c.content' } },
        },
      ],
    });
    // Original is untouched (pure function).
    expect(props.endpoint[0].basic_auth).toEqual({ username: 'u' });
  });

  it('creates missing block-instance containers along the way', () => {
    const next = setAtPath({}, ['endpoint', '0', 'basic_auth', 'password'], { [EXPR_KEY]: 'x' });
    expect(next).toEqual({ endpoint: [{ basic_auth: { password: { [EXPR_KEY]: 'x' } } }] });
  });

  it('deletes a nested value, leaving siblings and other instances untouched', () => {
    const props = {
      endpoint: [
        { url: 'a', basic_auth: { username: 'u', password: { [EXPR_KEY]: 'x' } } },
        { url: 'b', basic_auth: { username: 'v', password: { [EXPR_KEY]: 'y' } } },
      ],
    };
    const next = deleteAtPath(props, ['endpoint', '0', 'basic_auth', 'password']);
    expect(next).toEqual({
      endpoint: [
        { url: 'a', basic_auth: { username: 'u' } },
        { url: 'b', basic_auth: { username: 'v', password: { [EXPR_KEY]: 'y' } } },
      ],
    });
  });

  it('deleting a path that does not resolve is a no-op', () => {
    const props = { a: 1 };
    expect(deleteAtPath(props, ['nope', '0', 'x'])).toEqual({ a: 1 });
  });
});

describe('isExprValue / exprOf', () => {
  it('recognizes a $expr value and extracts it', () => {
    expect(isExprValue({ [EXPR_KEY]: 'x' })).toBe(true);
    expect(exprOf({ [EXPR_KEY]: 'x.y' })).toBe('x.y');
  });

  it('rejects a literal, a multi-key object, or a non-string $expr', () => {
    expect(isExprValue('plain string')).toBe(false);
    expect(isExprValue({ [EXPR_KEY]: 'x', other: 1 })).toBe(false);
    expect(isExprValue({ [EXPR_KEY]: 5 })).toBe(false);
    expect(exprOf('plain string')).toBeUndefined();
  });
});

describe('secretSourceNodes', () => {
  const schema: SchemaPayload = {
    _meta: { alloy_version: 'x', components_total: 2 },
    wire_types: {},
    components: {
      'remote.kubernetes.secret': minimalComponent({ sim_secret_source: { mode: 'literal' } }),
      'discovery.kubernetes': minimalComponent(),
    },
  };

  it('returns only nodes whose component declares sim_secret_source', () => {
    const d = doc([node('n1', 'remote.kubernetes.secret'), node('n2', 'discovery.kubernetes')]);
    expect(secretSourceNodes(d, schema)).toEqual([
      { id: 'n1', label: 'n1', component: 'remote.kubernetes.secret', mode: 'literal' },
    ]);
  });

  it('excludes a disabled node and returns [] for a null schema', () => {
    const d = doc([node('n1', 'remote.kubernetes.secret', { disabled: true })]);
    expect(secretSourceNodes(d, schema)).toEqual([]);
    expect(secretSourceNodes(d, null)).toEqual([]);
  });
});

describe('buildBindingExpr', () => {
  it('indexes a map-shaped export by key', () => {
    expect(SECRET_EXPORT_FIELD['remote.kubernetes.secret'].map).toBe(true);
    expect(
      buildBindingExpr({ component: 'remote.kubernetes.secret', label: 'creds' }, 'password'),
    ).toBe('remote.kubernetes.secret.creds.data["password"]');
  });

  it('returns the whole export for a scalar-shaped source, ignoring any key', () => {
    expect(buildBindingExpr({ component: 'local.file', label: 'c' })).toBe('local.file.c.content');
  });

  it('returns null for a map-shaped source given a blank key, and for an unknown component', () => {
    expect(
      buildBindingExpr({ component: 'remote.kubernetes.secret', label: 'creds' }, '  '),
    ).toBeNull();
    expect(buildBindingExpr({ component: 'not.a.secret.source', label: 'x' })).toBeNull();
  });
});
