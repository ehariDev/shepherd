import { CheckCircle2 } from 'lucide-react';
import { SsoBanner } from '@/components/admin/SsoBanner';
import type { TestOidcSettingsResponse } from '@/gen/shepherd/mgmt/v1/admin_pb';

/** Admin → Single sign-on → discovery test result (S9c). */
export function SsoTestReport({
  result,
  requested,
}: {
  result: TestOidcSettingsResponse;
  requested: string[];
}) {
  if (!result.ok) {
    return (
      <SsoBanner tone='error' testId='sso-test-result'>
        {result.message}
      </SsoBanner>
    );
  }
  return (
    <div
      data-testid='sso-test-result'
      className='rounded-md border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm space-y-1'
    >
      <p className='flex items-center gap-1.5 font-medium text-emerald-400'>
        <CheckCircle2 size={14} /> Discovery succeeded
      </p>
      <p className='text-xs text-muted-2'>
        This checks the issuer only. The client ID, secret, and redirect URL are not exercised until
        someone actually signs in.
      </p>
      <dl className='grid grid-cols-[max-content_1fr] gap-x-3 gap-y-0.5 text-xs text-muted'>
        <dt>Issuer</dt>
        <dd className='break-all'>{result.issuer}</dd>
        <dt>Authorize</dt>
        <dd className='break-all'>{result.authorizationEndpoint}</dd>
        <dt>Token</dt>
        <dd className='break-all'>{result.tokenEndpoint}</dd>
        <dt>JWKS</dt>
        <dd className='break-all'>{result.jwksUri}</dd>
      </dl>
      {result.issuerMismatch && <p className='text-xs text-amber-400'>{result.issuerMismatch}</p>}
      {!result.supportsPkce && (
        <p className='text-xs text-amber-400'>
          This provider does not advertise PKCE (S256). Shepherd always sends a PKCE challenge, so
          sign-in may fail.
        </p>
      )}
      {result.missingScopes.length > 0 && (
        <p className='text-xs text-amber-400'>
          Not advertised as supported: {result.missingScopes.join(', ')}. Many providers
          under-report this, so it is worth checking rather than trusting.
        </p>
      )}
      {requested.length > 0 && result.supportedScopes.length === 0 && (
        <p className='text-xs text-muted-2'>
          This provider does not publish a scope list, so the requested scopes could not be checked.
        </p>
      )}
    </div>
  );
}
