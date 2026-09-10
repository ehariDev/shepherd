/**
 * Shared types/constants for the Git sync forms (S9a). Pulled out of
 * GitPage.tsx so CredentialForm/RepoLinkForm/TestCredentialDialog can share
 * them without importing from the page module.
 */

// CREDENTIAL_KINDS mirrors docs/git-provider-design.md §3.2's six auth
// strategies. Order matters here: it's also the <select> option order.
export const CREDENTIAL_KINDS = ['pat', 'basic', 'ssh', 'ado_sp', 'github_app', 'none'] as const;
export type CredentialKind = (typeof CREDENTIAL_KINDS)[number];

export const KIND_LABELS: Record<CredentialKind, string> = {
  pat: 'Personal access token',
  basic: 'Basic auth (username + password)',
  ssh: 'SSH deploy key',
  ado_sp: 'Azure DevOps service principal',
  github_app: 'GitHub App',
  none: 'None (public repository)',
};

export interface CredentialFormState {
  name: string;
  kind: CredentialKind;
  username: string;
  adoOrgUrl: string;
  entraTenantId: string;
  clientId: string;
  appId: string;
  installationId: string;
  apiBaseUrl: string;
  clientSecret: string;
  secret2: string;
  sshKnownHosts: string;
  caCert: string;
  tlsInsecureSkipVerify: boolean;
}

export const emptyCredentialForm: CredentialFormState = {
  name: '',
  kind: 'pat',
  username: '',
  adoOrgUrl: '',
  entraTenantId: '',
  clientId: '',
  appId: '',
  installationId: '',
  apiBaseUrl: '',
  clientSecret: '',
  secret2: '',
  sshKnownHosts: '',
  caCert: '',
  tlsInsecureSkipVerify: false,
};

export interface RepoLinkFormState {
  repoUrl: string;
  branch: string;
  path: string;
  collectorId: string;
  credentialId: string;
}

export const emptyLinkForm: RepoLinkFormState = {
  repoUrl: '',
  branch: 'main',
  path: '/',
  collectorId: '',
  credentialId: '',
};
