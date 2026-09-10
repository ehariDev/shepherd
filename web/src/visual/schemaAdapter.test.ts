import { describe, expect, it } from 'vitest';
import { schemaFixture } from '../../tests/fixtures/schema-fixture';
import {
  DEFAULT_CATEGORY_COLOR,
  DEFAULT_WIRE_COLOR,
  getCategoryColor,
  getWireColor,
  portHandleId,
} from './schemaAdapter';

describe('portHandleId (A1)', () => {
  it('prefers `prop` when present', () => {
    expect(portHandleId({ prop: 'targets', export: 'other' }, 3)).toBe('targets');
  });
  it('falls back to `export` when `prop` is absent', () => {
    expect(portHandleId({ export: 'metrics' }, 2)).toBe('metrics');
  });
  it('falls back to a positional id when neither `prop` nor `export` is present', () => {
    expect(portHandleId({}, 0)).toBe('p0');
    expect(portHandleId({}, 4)).toBe('p4');
  });
});

describe('getWireColor / getCategoryColor (A4)', () => {
  it('reads the wire color from the schema payload when present', () => {
    // schemaFixture is now sliced from the shipped artifact + overlay, so the
    // expected value is the overlay's own loki.logs color rather than a colour
    // the fixture invented.
    expect(getWireColor(schemaFixture, 'loki.logs')).toBe('#22c55e');
  });
  it('returns the default for a wire type the schema does not define', () => {
    expect(
      getWireColor({ wire_types: {}, components: {}, _meta: schemaFixture._meta }, 'targets'),
    ).toBe(DEFAULT_WIRE_COLOR);
  });
  it('falls back to the default hex when schema is null', () => {
    expect(getWireColor(null, 'targets')).toBe(DEFAULT_WIRE_COLOR);
  });
  it('falls back to the default category color when the payload carries no categories', () => {
    const withoutCategories = { ...schemaFixture, categories: undefined };
    expect(getCategoryColor(withoutCategories, 'sources')).toBe(DEFAULT_CATEGORY_COLOR);
    expect(getCategoryColor(withoutCategories, 'destinations')).toBe(DEFAULT_CATEGORY_COLOR);
  });
  it('reads the category color from the schema payload when the overlay serves one (backend half of A4)', () => {
    const withCategories = {
      ...schemaFixture,
      categories: { sources: { color: '#123456', label: 'Sources' } },
    };
    expect(getCategoryColor(withCategories, 'sources')).toBe('#123456');
  });
});
