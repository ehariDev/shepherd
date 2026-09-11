import { useEffect, useState } from 'react';

/**
 * Trailing-debounced view of `value`: updates `delayMs` after the LAST
 * change, not on every change. `BottomDrawer`'s Code tab (W5-10) is the
 * motivating caller — `renderTS(doc, schema)` re-walks the whole graph on
 * every store mutation, including one per keystroke in the inspector, even
 * while the drawer is collapsed or showing a different tab; debouncing the
 * `doc` it renders from keeps that off the hot path while the user is still
 * typing, without changing what eventually gets shown.
 */
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(timer);
  }, [value, delayMs]);
  return debounced;
}
