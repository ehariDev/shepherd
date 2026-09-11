# Proof: local admin boot validation

## Red run
With local admin enabled and no password hash, configuration loading fails with the required `password_hash` validation error.

## Green run
The validation is implemented in `config.Load`; the configuration package tests pass.

*Note added 2026-09-11: the config-level `password_hash` field this proof validated no longer
exists (`grep -n password_hash internal/config/config.go` — no hits). Local admin provisioning is
now `auth.UserStore.BootstrapAdmin` (`internal/auth/localusers.go`), which reads
`SHEPHERD_BOOTSTRAP_ADMIN_LOGIN`/`SHEPHERD_BOOTSTRAP_ADMIN_PASSWORD` at first-start time instead of
a static config field — part of full local user management (migration 0015). This proof is kept as
the historical record of "boot must fail loudly rather than silently accept no password", not as a
pointer to current code.*
