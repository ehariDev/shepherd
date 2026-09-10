/**
 * Shared form state for the admin → single sign-on page (S9c), split out of
 * AdminAuthPage.tsx so the section components can share it.
 */

/** Local form state. Mirrors UpdateOidcSettingsRequest, with the list fields
 *  held as the newline-separated text the textareas actually edit. */
export interface SsoFormState {
  enabled: boolean;
  provider: string;
  displayName: string;
  issuer: string;
  clientId: string;
  clientSecret: string;
  redirectUrl: string;
  scopes: string;
  subjectClaim: string;
  emailClaim: string;
  nameClaim: string;
  groupsClaim: string;
  appAdminGroups: string;
  useGraphGroups: boolean;
  graphBaseUrl: string;
}

export const EMPTY_SSO_FORM: SsoFormState = {
  enabled: false,
  provider: 'generic',
  displayName: '',
  issuer: '',
  clientId: '',
  clientSecret: '',
  redirectUrl: '',
  scopes: '',
  subjectClaim: '',
  emailClaim: '',
  nameClaim: '',
  groupsClaim: '',
  appAdminGroups: '',
  useGraphGroups: false,
  graphBaseUrl: '',
};

export const toLines = (values: string[]) => values.join('\n');
export const fromLines = (text: string) =>
  text
    .split(/[\n,]/)
    .map((value) => value.trim())
    .filter(Boolean);

/** The only callback path the server serves (internal/auth.CallbackPath). */
export const SSO_CALLBACK_PATH = '/auth/callback';
