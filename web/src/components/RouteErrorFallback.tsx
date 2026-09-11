import { toApiError } from '@/api/transport';

/**
 * The router-wide error boundary fallback.
 *
 * TanStack Router resolves error boundaries per LEAF match, not per ancestor:
 * a rootRoute `errorComponent` never actually sees a page crash, because the
 * nearest matched route below it always catches the error first. So this is
 * wired as `defaultErrorComponent` on `createRouter` (router.tsx), which every
 * route without its own `errorComponent` falls back to — Shell stays mounted
 * (Sign out, the nav, the org switcher all keep working) because the boundary
 * that actually catches sits below it, at the failed leaf.
 *
 * Kept prop-compatible with TanStack's `ErrorComponentProps` (error, info?,
 * reset) but deliberately renders no router hooks or `<Link>` of its own, so
 * it stays testable with a bare `renderToString` — no RouterProvider needed.
 */
export function RouteErrorFallback({
  error,
  reset,
}: {
  error: unknown;
  info?: { componentStack: string };
  reset?: () => void;
}) {
  const message = toApiError(error).message || String(error);
  return (
    <div
      role='alert'
      data-testid='route-error'
      className='m-6 rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-400'
    >
      <p className='font-medium'>Something went wrong.</p>
      <p className='mt-1 text-red-300'>{message}</p>
      <div className='mt-3 flex gap-2'>
        <button
          type='button'
          onClick={() => reset?.()}
          className='rounded-md border border-red-500/30 px-3 py-1.5 text-xs text-red-300 hover:bg-red-500/20'
        >
          Try again
        </button>
        <a
          href='/'
          className='rounded-md border border-red-500/30 px-3 py-1.5 text-xs text-red-300 hover:bg-red-500/20'
        >
          Go to overview
        </a>
      </div>
    </div>
  );
}
