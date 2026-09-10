/**
 * Shared state for the admin → users forms (S9b), split out of
 * AdminUsersPage.tsx so CreateUserModal/EditUserModal/ResetPasswordModal can
 * share it without importing from the page module.
 */

/** The three org roles, in rank order. Matches org_members.role's CHECK. */
export const ORG_ROLES = ['admin', 'editor', 'viewer'];

export interface CreateUserFormState {
  login: string;
  email: string;
  displayName: string;
  password: string;
  isAppAdmin: boolean;
  mustChangePassword: boolean;
}

export const emptyCreateUserForm: CreateUserFormState = {
  login: '',
  email: '',
  displayName: '',
  password: '',
  isAppAdmin: false,
  mustChangePassword: true,
};
