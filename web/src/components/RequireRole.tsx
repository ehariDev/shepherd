import { useNavigate } from '@tanstack/react-router';
import { type ReactNode, useEffect, useRef } from 'react';
import { toast } from 'sonner';
import { useMe } from '@/hooks/useMe';
import { useOrg } from '@/hooks/useOrg';
import { type RequiredRole, roleSatisfied } from '@/routes/routeManifest';

/**
 * Client-side gate for routeManifest's `requiredRole`. The server remains
 * the real enforcement (internal/mgmtapi/rpc_interceptor.go) — this exists
 * so a denied direct navigation never mounts the page component at all, i.e.
 * its privileged RPC never fires, instead of letting the page render and
 * then fail loudly (or, worse, render successfully because a mock or a
 * client-only check was the only thing standing in the way).
 *
 * Renders nothing until useMe resolves — no flash of a page the viewer
 * isn't allowed to see. Once resolved, a denial shows a
 * `data-testid="route-denied"` element, raises a toast, and redirects to
 * '/', in that order within the same effect.
 */
export function RequireRole({
  requiredRole,
  children,
}: {
  requiredRole: RequiredRole;
  children: ReactNode;
}) {
  const { data: me, isLoading } = useMe();
  const { orgId } = useOrg();
  const navigate = useNavigate();
  const redirected = useRef(false);

  const allowed = !isLoading && roleSatisfied(me, orgId, requiredRole);

  useEffect(() => {
    if (isLoading || allowed || redirected.current) return;
    redirected.current = true;
    toast.error('You do not have permission to view that page.');
    navigate({ to: '/' });
  }, [isLoading, allowed, navigate]);

  if (isLoading) return null;
  if (!allowed) {
    return (
      <div role='alert' data-testid='route-denied' className='sr-only'>
        You do not have permission to view this page.
      </div>
    );
  }
  return <>{children}</>;
}
