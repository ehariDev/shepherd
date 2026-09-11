# Proof: local admin LogValue redaction

## Red run
Logging a local admin configuration could expose the configured password hash.

## Green run
`LocalAdminConfig.LogValue` emits `[REDACTED]`; the named test confirms the hash value is absent.

*Note added 2026-09-11: `LocalAdminConfig` no longer exists under that name — local admin
provisioning is now `auth.UserStore.BootstrapAdmin` (`internal/auth/localusers.go`), part of full
local user management (migration 0015; see `docs/archive/README.md`'s correction on SCIM). This
proof is kept as the historical record of the redaction property, not as a pointer to current code.*
