import { renderToString } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { RouteErrorFallback } from './RouteErrorFallback';

describe('RouteErrorFallback', () => {
  it('renders an alert carrying the error message, with no router context required', () => {
    const html = renderToString(<RouteErrorFallback error={new Error('boom')} />);
    expect(html).toContain('role="alert"');
    expect(html).toContain('data-testid="route-error"');
    expect(html).toContain('boom');
  });

  it('offers a retry action and a way back to the overview', () => {
    const noop = () => {
      /* not exercised — renderToString doesn't click anything */
    };
    const html = renderToString(<RouteErrorFallback error={new Error('boom')} reset={noop} />);
    expect(html).toContain('Try again');
    expect(html).toContain('Go to overview');
  });
});
