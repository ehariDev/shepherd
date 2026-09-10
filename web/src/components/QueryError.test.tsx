// @vitest-environment jsdom
import { Code, ConnectError } from '@connectrpc/connect';
import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import * as transport from '@/api/transport';
import { QueryError } from './QueryError';

// Delegates to the real toApiError by default; individual tests override it
// with mockReturnValueOnce to exercise QueryError's own fallback text without
// depending on whether toApiError can actually produce that input (it
// currently cannot — see the "Please retry." test below).
vi.mock('@/api/transport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/transport')>();
  return { ...actual, toApiError: vi.fn(actual.toApiError) };
});

afterEach(() => {
  vi.mocked(transport.toApiError).mockClear();
});

describe('QueryError', () => {
  it('renders a permission-denied message for permission_denied errors, not the generic retry copy', () => {
    render(
      <QueryError error={new ConnectError('nope', Code.PermissionDenied)} noun='audit entries' />,
    );
    const alert = screen.getByRole('alert');
    expect(alert.textContent).toBe(
      'You do not have permission to view audit entries in this organisation.',
    );
  });

  it('renders a permission-denied message for unauthenticated errors too', () => {
    render(
      <QueryError error={new ConnectError('nope', Code.Unauthenticated)} noun='credentials' />,
    );
    expect(screen.getByRole('alert').textContent).toContain('do not have permission');
  });

  it('renders a generic failure message with the underlying error for other errors', () => {
    render(
      <QueryError
        error={new ConnectError('destination not found', Code.NotFound)}
        noun='destinations'
      />,
    );
    expect(screen.getByRole('alert').textContent).toBe(
      'Failed to load destinations. destination not found',
    );
  });

  it('falls back to "Please retry." when toApiError reports no message', () => {
    // NOTE: toApiError (ConnectError.from under the hood) currently never
    // actually produces an empty message in practice — every code, even with
    // an empty rawMessage, formats as "[code] ..." — so this fallback branch
    // (QueryError.tsx's `err.message || 'Please retry.'`) is presently dead
    // code. Isolate it here via a mocked toApiError rather than asserting an
    // unreachable real-world input.
    vi.mocked(transport.toApiError).mockReturnValueOnce({ code: 'internal', message: '' });
    render(<QueryError error={new Error('irrelevant, toApiError is mocked')} noun='pipelines' />);
    expect(screen.getByRole('alert').textContent).toBe('Failed to load pipelines. Please retry.');
  });
});
