# 2026-09 remediation — review, plan, and what actually happened

> **Archived 2026-09-11.** Every deferred item below has since closed: the `UPGRADING.md`
> 0.9.x → 0.10.0 section exists; the fullstack roles/rollout/wizard-commit/matcher-edit/sandbox-run
> specs are written (`web/tests/fullstack/`); the React 19 / TypeScript 7 / Vite 8 migrations, the
> request-gate authentication and the lazy editor chunk shipped in v0.5.0 (PRs #43, #45, #46, #48,
> #50, #51). Still open, and tracked in `docs/project-status.md` rather than here: the kind
> suite's G10 and previous-version upgrade spec, and the gateway plan's review gates R2/R3/R6. This
> file is the record of how the 2026-09 remediation ran; it is not maintained.

The 2026-09-09 review found gaps across the whole repository — CI gates that ran nowhere, RBAC
holes, stale docs contradicting shipped behavior, unadopted libraries still documented as adopted.
This session (branch `remediation/2026-09`, opened 2026-09-10) fixed what the review found. This
document is the record: the source artifacts, the decisions settled before work started, how the
work actually ran (including the failure modes), what each workstream landed, what is still
deferred, and the evidence for all of it.

- **Review artifact**: <https://claude.ai/code/artifact/5b7bc97a-aa9d-4395-83b6-77807ac5a97e>
- **Remediation plan artifact**: <https://claude.ai/code/artifact/5a0f946c-625b-49a1-a26f-79bda7d34910>
- **Base**: `95f82a0` (main, PR #4 merged — chart 0.9.0 hardening) → **head at the start of the
  docs pass**: `39f724f` (`git log --oneline 95f82a0..39f724f` — 140 commits)

## Decisions settled before work started (D1–D14)

Recorded verbatim from the wave-0 commit message (`git show 0d0669d --no-patch`), settled with the
user on 2026-09-10 before any workstream began:

- **D1** merge PR #5 (grpc bump) + the x/crypto bump onto the remediation base.
- **D2** `@testing-library/react` + `jsdom` for web component tests.
- **D3** service accounts get a configurable role tier, default `editor`, existing rows backfilled
  `editor`.
- **D4** local sign-in throttle via `golang.org/x/time/rate`.
- **D5** the REST shim's pipeline-write/validate/wizard/visual groups align to `org-editor`.
- **D6** `SimulateService` is `org-editor` everywhere — Connect, REST, and the proto comments.
- **D7** OIDC-derived sessions end at `id_token_expires`, not the local session TTL.
- **D8** implement light mode: follow the OS by default, a toggle stores an explicit override.
- **D9** the simulator's bearer token comes from `simulator.token.existingSecret`, then External
  Secrets, then a chart-generated `Secret`, in that order.
- **D10** drop the sandbox's `kube-system` DNS egress; the harness listens on loopback instead.
- **D11** `make e2e` runs on push to `main`, path-filtered — not only on a schedule.
- **D12** attest provenance for release archives and images.
- **D13** the fullstack sandbox-run spec stays local-only (needs the dev stack; no CI runner for it
  yet).
- **D14** app `v0.4.0`, chart `0.10.0`.

Every workstream below cites which D-decision it implements where one applies.

## How the waves actually ran

The plan called for parallel sub-agents in waves, each in its own git worktree, merged back to
`remediation/2026-09` between waves. That is not quite what the git history shows, and the
divergence is worth recording rather than smoothing over — it is exactly the kind of drift this
session exists to close in *other* documents.

**Wave 0** (`0d0669d`, 2026-09-10 10:23) — one commit: the dependency bumps, the
`@testing-library/react`/`jsdom` harness, `scripts/repocheck`'s Ginkgo guards, and the `.PHONY` fix
that had made `make docs` a silent no-op. This is the commit the D1–D14 decisions above are
recorded against.

**Wave 1** (merge commits `1a4a0e8`…`7e191a3`, all at 2026-09-10 11:46) — twelve worktrees merged,
numbered `worktree-wf_d7e92727-399-{1,3,5,9,10,13,14,17,18,19,20,21}`. The numbering is not
consecutive: worktrees 2, 4, 6, 7, 8, 11, 12, 15 and 16 never produced a merge commit. This session
independently hit the same class of problem its own ground rules now guard against — **rule 1**
above required verifying `HEAD` against the required base commit and resetting if it had drifted,
because a worktree spawned before the base was fully in place inherits a stale tree. The gaps in
wave 1's numbering are consistent with the same defect there: some agents in that wave were spawned
against a base that did not yet include wave 0 (or an earlier wave-1 sibling), produced nothing
mergeable, or were discarded by the orchestrator rather than merged. This is inference from the
commit graph, not a first-hand account of wave 1 — but it is why wave 1 is the only round with
gaps, and why the process changed for what came after.

**Between wave 1 and the next round** (`5263fb2` through `8e63a0e`, 2026-09-10 11:49–14:23) — a
run of individually-committed fixes (`S3`–`S20`, `S9a`–`S9c`, `S4-tail`, `W5/W6-D`) landed directly
on the integration branch rather than through another batch of worktree merges. This is where most
of W1's CI work (dependabot, the guards job, govulncheck wiring, SHA-pinning every workflow action,
buildx cache scoping) and W6's page-splitting work actually shipped. Whether this was a deliberate
process change after wave 1's gaps or a continuation of the same session under different bookkeeping
is not recorded in the commit messages; what is recorded is that the round after this one changed
its naming from "wave" to "batch" and its numbering became consecutive again.

**Batch 2** (merge commits `01859c7`…`e479436`, all at 2026-09-10 14:29–14:30) — thirteen
worktrees, `worktree-wf_77a93ffb-91e-{1..13}`, every number present. Followed immediately by
integration fixes the merge exposed: `a0260ec` (regenerated `site/docs/helm-values.html`),
`8ffa82e` ("orchestrator cross-cutting edits after batch 2, and the rebuilt SPA bundle"), then
`d75f025`/`869f8be`/`d478db9`/`84cfc67` (`F1`–`F3`) — the route-guard denial tests, the
`waitForTimeout` migration, and the persona-floor guard, all rewritten against what the merged
branch actually did rather than what each workstream had assumed in isolation. `869f8be`'s own
message — "two consumer-layer breaks the merged branch exposed" — is the honest name for this: work
that was individually green went red only once two workstreams' output met.

**The usage-limit restart**: the last batch-2 fix commit (`84cfc67`) is timestamped 2026-09-10
14:56:22 +0200; the next commits in the branch (batch 3's merges) are timestamped 2026-09-11
09:04:40 +0200 — an eighteen-hour gap with nothing in between. Nothing else in the repository
explains a pause that long inside an otherwise minutes-apart commit cadence; a session-level
interruption (a usage limit reached and a restart the next session) is the only account consistent
with the timestamps.

**Batch 3** (merge commits `c16a710`…`8b545d7`, all at 2026-09-11 09:04) — four worktrees,
`worktree-wf_e358f000-22a-{1..4}`, consecutive again. Followed by a small tail: `b077ae1` (a flaky
wait fixed in a visual-builder spec), `31edb45` (the chart 0.10.0 / app 0.4.0 version bump, D14),
and `39f724f` (the `pnpm lint` script rename) — the last commit before this docs pass (wave 4)
began.

**Wave 4 — this pass**: docs-only, no code changes, three parallel agents (repo-facing docs,
spec/status docs, plans/archive — this document is the plans/archive agent's own record) on
disjoint file territories, run from `39f724f`.

## What each workstream landed

Commit ranges are `git log --oneline 95f82a0..39f724f`, first named commit → last, oldest first;
each range also includes unlabeled fix commits attributable to that workstream by content, not only
the ones carrying its prefix.

**W1 — Build/CI** (`c30fff1`…`ff1deee`, 14 `W1`-prefixed commits plus the CI-specific `S3`, `S4`,
`S5`, `S6`, `S10`, `S11`, `S12`, `S19`, `S20` series — `S9a`–`S9c` and `S4-tail` are UI work,
counted under W6 below): the ten `make guards`
checks with `lint` depending on them; `make vulncheck` (`govulncheck` via `go run`); `test-cover`;
`docs`/`check-docs-*` finally `.PHONY`; `make smoke` rewritten around the bootstrap admin env vars;
`check-raw-sql` widened to any receiver, not only `Pool()`; `build-docs.py --out`/`--check`;
`gofumpt` dropped from `make tools`; `scripts/__pycache__` gitignored. `scripts/repocheck` (a
Ginkgo suite of repository-shape guards) runs in `ci.yml`'s `guards` job alongside `golangci-lint
config verify`; the `build` job runs `make vulncheck`; the `test` job runs `make test-cover` with
coverage upload and an explicit `needs-alloy-binary` run; `test-fullstack` runs `make smoke`; the
frontend gate covers `scripts/build-web.sh`; `cancel-in-progress` is PR-only; buildx cache keys are
scoped per workflow/job/run. `e2e.yml` runs on push to `main`, path-filtered, plus `merge_group`
(D11). `.github/dependabot.yml` (gomod, npm, github-actions, docker, weekly, grouped);
`govulncheck.yml` weekly; every Action SHA-pinned across five workflow files. `release.yml` gained a
`verify` job (lint, guards, build, vet, `go test`, `helm lint`) ahead of release, the
chart-already-published probe before `goreleaser`, provenance attestation (D12), and semver-only
tags.

**W2 — Go core** (`c7ef8cd`…`f21b867`, 12 commits): `gitsync` now runs Stages 1+2 and a Stage 3
merge dry-run on every synced file and fails closed without a validator. `internal/serve.ComputeServed`
is one recompute implementation shared by `mgmtapi` and `agentapi`'s serve paths (hash-identical,
proven). sqlc-generated queries (`MarkServeCacheDirtyByCluster`, `CountOrgContent`,
`ListPipelineNamesReferencingDestination`) replace the last raw SQL sites. `MigrateStatus` writes to
an `io.Writer`. `mgmtapi/rpc_errors.go`'s `mapError` is now used by `rpc_pipeline`, `rpc_admin`,
`rpc_destination`, `toConnectError`, and `oidcSettingsError`. The `exhaustive` linter's
`default-signifies-exhaustive=false` closed four latent switch gaps; 18 stale `nolint` directives
dropped. `RunWorker` moved to `internal/simulate/worker` so `internal/simulate` is DB-free.

**W3 — RBAC** (`552e1f4`…`7b8d6de`, 14 commits): service accounts gained a role tier
(`0018_service_account_role`, column `role` `editor`|`admin` default `editor`, D3) — the interceptor
checks the tier after the org match, and app-admin procedures are refused to every machine caller
unconditionally. Local login is throttled per login and per source IP with `x/time/rate` (429 +
`Retry-After`, D4). OIDC sessions end at `id_token_expires` (D7), with a nonce binding the ID token
to the login. Local team-only members clear the viewer floor in both `authorizeOrgAccess` and
`ResolveOrgRole`/`GetMe` — the two paths had drifted. `auth.RoleSatisfies` and `auth.ResolveOrgRole`
are exported. The REST shim's pipeline-write/validate/wizard/visual/simulate groups require
`org-editor` (D5, D6); `simulate` is `org-editor` on Connect, REST, and in the proto comments.
`spec.md` §7 and §12 rewritten to match.

**W4 — Simulator** (`aacd44c`…`6feb4e6`, 9 commits): the sandboxed Alloy gets a minimal explicit
environment (`PATH`, `HOME`, `TMPDIR`); rune-safe stderr truncation with control-char stripping;
runner tests for argv, kill grace, scratch cleanup, start failure. The simulator's bearer token now
has three sources in order — `simulator.token.existingSecret`, then External Secrets, then a
chart-generated `Secret` — with checksum annotations on both Deployments so a token rotation
restarts the right pods (D9). The chart's `NetworkPolicy` drops the sandbox's `kube-system` DNS
egress; the harness listens on loopback instead (D10). compose stacks share one `SIM_TOKEN` between
Shepherd and the simulator. Chart `0.10.0` / app `0.4.0` (D14) — install pins updated,
`UPGRADING.md`'s `0.9.x → 0.10.0` section **not yet written** (see Deferred, below).

**W5 — Visual builder** (`3e7bfce`…`8463440`, 13 commits, plus `8e63a0e`): secret bindings are
authored as `{"$expr": expr}` values written directly into `props` at the prop's own instance path,
any depth — not into the separate, import-only `bindings[]` array, which cannot represent a nested
path (`web/src/visual/bindings.ts`). Secret-source discovery reads the overlay; a scalar port's
second incoming wire replaces the first immediately, as one undo step, with an Undo toast rather
than a blocking confirm. `edge.order` is stamped at connect time, with a `moveEdge` reorder control
in the inspector. `UpgradeReview` prunes `attr_removed` props on Accept, refuses Accept while a
`component_removed` finding exists, and stopped re-firing `UpgradeCheck` on every mutation. Draft
autosave writes to IndexedDB 500ms after the last graph mutation (not every keystroke) with a
restore-or-discard banner. `reconcile.ts` was extracted from `CanvasPane` and unit-tested on its
own. The corpus is read from `internal/visual/testdata` in place — the `web/src/visual/__fixtures__`
copy is gone, and `make generate-corpus` no longer copies anything there. Diagnostics parity between
the Go and TS renderers is pinned per corpus entry, plus one mixed entry. Dead colour-fallback tables
were deleted; the inspector's node selector narrowed; the Code tab debounces 300ms. The three visual
overlays and `SandboxRunPanel`'s tables moved onto `components/ui`'s `Modal`/`DataTable`; canvas
colours follow the theme toggle (D8, `8e63a0e`).

**W6 — UI shell** (`19fe5f4`…`cc9138d`, 9 `W6`-prefixed commits, plus `8e63a0e`, `177e8da`, and
`S9a`/`S9b`/`S9c`/`S4-tail`): light mode (D8) —
`prefers-color-scheme` by default, a toggle stores the override, `html.light`/`html.dark`, and the
overlay's `color_light` drives wire/category colours. `components/ui` gained `Modal`, `ModalActions`,
`DataTable`, `Field`, `Input`, `Select`, `Textarea`, `Banner`, `Section`, and every list/detail page
now has a `QueryError` branch. `RouteErrorFallback` is the router's `defaultErrorComponent`.
Route-level `requiredRole` reads the route manifest and redirects to `/` with a toast and a
`data-testid="route-denied"` marker — the server remains the actual enforcement point. Breadcrumbs
derive from route labels, not the raw pathname. `GitPage`, `AdminUsersPage`, and `AdminAuthPage`
were each split onto primitives, under 500 lines. `pnpm lint` is now the read-only check CI runs;
`lint:fix` is the separate mutating pass (`39f724f`).

**W7 — Tests** (`edf8dec`…`9502191`, 14 commits, plus the `F1`–`F3` integration fixes and
`b077ae1`): a jsdom component-test harness (`QueryError`, `AdminModal`, `AdminConfirmDialog`,
`WizardStepper`, `WizardStepFields`, `UpgradeReview`, `InspectorPanel`). The dev seed creates
`admin`/`editor`/`viewer` local users on the platform org. Fullstack fixtures gained
`loginAs(page, user, pass)`. New mocked Playwright specs: a route-guard matrix (`orgAdmin`/
`orgEditor`/`reader`/`nobody` × admin routes + `/teams`), the persona-floor guard, a wait-budget
guard for visual specs, and per-persona cases across destinations/dialogs/query-errors/audit/
collector-access/git-page/editor-role/rbac. CLI token tests and SPA handler tests. The gitrepo SSH
host-key callback was fixed (F9-a) with a HOME-isolated test suite, and the e2e SSH GitOps scenario
was un-skipped. `F1`–`F3` (landed after batch 2's merge exposed them, see above) rewrote the
admin/RBAC direct-nav denial tests to the actual route-guard contract, migrated the batch-2
`waitForTimeout` calls off real-time sleeps, and closed the persona-floor guard's coverage gap.
**Not yet written**: fullstack roles/rollout/wizard-commit/matcher-edit/sandbox-run specs (D13 —
local-only, needs the dev stack, explicitly deferred past this docs pass).

**W8 — Docs** (this pass, wave 4, in progress): the workstream this document is part of. Three
agents on disjoint doc territory — see the parent plan artifact for the full step list. Not yet
merged to `remediation/2026-09` as of this document.

## Deferred

Recorded so the next session does not have to re-derive it from the diff:

- **`UPGRADING.md`'s `0.9.x → 0.10.0` section.** The chart moved 0.9.0 → 0.10.0 in this session
  (D14) with install pins updated; the upgrade-notes section for that jump does not exist yet
  (verified: `grep -n '^## ' deploy/helm/shepherd/UPGRADING.md` shows only `0.8.x → 0.9.0`).
- **Fullstack roles/rollout/wizard-commit/matcher-edit/sandbox-run Playwright specs (W7, D13).**
  Explicitly local-only — no CI runner exercises the dev stack these need yet.
- **G10** (installing the k8s-monitoring chart with generated values in the kind suite and watching
  an Alloy register) and the true previous-version Helm upgrade spec in the kind suite — see
  `docs/kind-test-environment-plan.md` §9 item 4, no longer blocked on a released prior chart
  version (`v0.3.5` is chart 0.9.0) but still unwritten.
- **`docs/gateway-tier-plan.md`'s open review gates** — R2 (beacon data review) and R3 (receiver
  tier containment) are unsigned; R6 (agent-actor review) is only partly resolved. This
  remediation's W3 extended one table that plan's W10 created (`service_accounts` gained a `role`
  tier, migration 0018, alongside W10's existing `capability` column) but did not touch that plan's
  own workstreams otherwise; its status header contradicted its own ledger regardless, and was
  corrected by W8-11 in this same wave.
- **The visual builder's relabel-trace "live samples" source** (§6.3 of
  `docs/visual-builder-design-VB1.md`) was found, in this docs pass, to have never been built — no
  config knob, no paste-JSON UI, no live Prometheus query. Recorded as a design-doc correction, not
  a code defect to fix, because nothing regressed: it was never shipped.

## Verification evidence

Dated **2026-09-11**, run by the orchestrator on the merged branch (this docs-only worktree does
not run Docker, per its own ground rules, so these are cited rather than re-run here):

| Check | Result |
|---|---|
| `go build`, `go vet` | OK |
| `golangci-lint` | 0 issues |
| `make lint` (all ten guards + config verify) | OK |
| `go test ./scripts/repocheck` | OK |
| `go test ./...` | 41 packages OK. One failure (`agentapi`) attributed to a Postgres testcontainer dying while `make smoke` built images concurrently in the same run — a resource-contention flake, not a code defect; rerun was in progress at time of writing |
| `pnpm typecheck`, `biome check` | OK |
| Vitest | 564/564 |
| Mocked Playwright | 254/254 |
| `make smoke` | PASSED |
| `make e2e` | **gate running** at time of writing, not yet reported |
| `make e2e-sim` | **pending** |
| `make e2e-k8s` | **pending** |

This session independently confirmed the `go build ./...` line above from a clean checkout of
`39f724f` in this docs worktree (no Docker involved) as a sanity check before starting the docs
pass — it built clean.

## Where the rest of the record lives

- `docs/project-status.md` — the live ledger; W8-20 re-baselines it against this session.
- `docs/gateway-tier-plan.md`, `docs/kind-test-environment-plan.md`,
  `docs/visual-builder-design-VB1.md` — corrected in this same wave (W8-11/12/13) against the code
  this session shipped.
- `docs/archive/README.md` — the archive index, rewritten in this wave (W8-15) to drop 13
  unresolvable commit SHAs left over from a pre-reset history.
- `CHANGELOG.md` — user-facing entries for what actually ships, in this project's own
  Shipped/RPC-only/Built-not-wired taxonomy.
