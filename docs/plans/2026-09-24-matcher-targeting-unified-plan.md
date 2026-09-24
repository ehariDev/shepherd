# Matcher targeting — unified design & implementation plan

**Status:** Planned — unreleased — **HOLD, see status block below**
**Supersedes:** `2026-09-24-matcher-impact-preview.md` and `shepherd-matcher-targeting-redesign-backlog.md` (both retired — see §0 note in each; do not implement from either, this document is the merge of both, corrected)
**Location once merged:** `docs/plans/2026-09-24-matcher-targeting-unified-plan.md` (archive to `docs/archive/plans/` after release, per `AGENTS.md`'s docs map)
**Spec section affected:** `docs/spec.md` §6.1 (Merge engine — Matchers)
**Verified against:** `main` @ [`7189a14`](https://github.com/ehariDev/shepherd/blob/main) (`7189a148d5a14dfa1c6dba17bddd50f65f45bf79`), 2026-09-24. All file:line citations below point at `main`, not any feature branch — re-verify line numbers if `main` has moved since this hash.
**Owner persona for this plan:** Senior Observability Architect review, pre-implementation

> **Status: PLAN ONLY — HOLD.** No code has been written for this work, no branch created, and **none
> should be until PR [#12](https://github.com/ehariDev/shepherd/pull/12) and PR
> [#11](https://github.com/ehariDev/shepherd/pull/11) are both merged to `main`.** Both PRs touch the
> exact files this plan touches (`rpc_pipeline.go`, `merge.go`, `PipelineEditorPage.tsx`). As of
> 2026-09-24: **#12** `feat/label-matching-pr11` is `OPEN`, `mergeable: MERGEABLE`. **#11**
> `fix/pr144-remaining-findings` is `OPEN`, `mergeable: CONFLICTING` (branched from #12, so it can't
> cleanly merge before #12 does). Re-check both before creating a branch:
> `gh pr view 12 --repo ehariDev/shepherd --json state,mergedAt` /
> `gh pr view 11 --repo ehariDev/shepherd --json state,mergedAt`.

> **2026-09-24 status update — hold explicitly overridden by repo owner.** Re-checked: #12 and #11 are
> still both `OPEN` (unmerged) as of this update. Owner decision: branch off PR #12's head
> (`feat/label-matching-pr11`, `MERGEABLE` into `main`) rather than wait for the merge — avoids #11's
> conflicts by simply not building on #11. Working branch: `feat/matcher-targeting-unified-plan`
> (created from `feat/label-matching-pr11` @ `4913e7d`). This branch will need to be rebased once #12
> (and, separately, #11 if/when it's un-conflicted) actually merge to `main`.
>
> Phase 0 gates resolved:
> - **0.2 (decision-maker):** repo owner (ehariDev) is the sign-off owner for the Phase 2 proto ask and
>   the "merges to main" call. No separate approver.
> - **0.3 (grandfather audit):** no real production fleet exists yet. Ran the equivalent query against
>   the WSL systemd install's dev DB (org `wsl-lab`, port 5433):
>   `select count(*) from pipelines where source in ('ui','wizard','visual') and
>   jsonb_array_length(matchers) = 0 and enabled = true;` → **0**. No grandfathering concern, no owner
>   notification needed before Phase 3's `EnablePipeline` gate ships.

---

## 0. Why this document exists, and what it corrects

`2026-09-24-matcher-impact-preview.md` and `shepherd-matcher-targeting-redesign-backlog.md` were written
the same day, cover overlapping ground (matcher validation, preview, targeting UX), and disagree on
pieces of it. A senior-architect pass across both, checked directly against `main` @ `7189a14`, found:

1. **Both plans share a factual error.** Each says `PipelineService.PreviewMatches` has no real callers,
   based on grepping only `web/src`. It has four: the MCP tools `preview_matches`
   (`internal/mcp/tools.go:67-70,225-236`) and `propose_pipeline_revision`
   (`internal/mcp/propose.go:123-151`), the REST endpoint
   `GET /api/orgs/{org}/pipelines/{id}/preview-matches`
   (`internal/mgmtapi/pipelines.go:311-314`, `internal/mgmtapi/router.go:178`), and the fullstack test
   `matcher-edit.spec.ts` (`docs/frontend-testing.md:44`). Extending the RPC in place (rather than adding
   a second one) is still correct — see §4 decision 2 — but the two response-shape changes below (§6.2)
   are contract changes for those real callers, not free additions.
2. **A second, previously unreported bug: `PreviewMatches` is broken for git-sourced pipelines today.**
   `internal/mgmtapi/rpc_pipeline.go:817-823` builds `merge.Pipeline{ID, Name, Matchers, Source}` —
   it never sets `RepoLinkCollectorID`. Since `merge.MatchesPipeline` (`internal/merge/merge.go:110-113`)
   matches a git pipeline by `p.RepoLinkCollectorID == cl.CollectorID`, an empty `RepoLinkCollectorID`
   means **`PreviewMatches` on any git-sourced pipeline today returns zero collectors, always**, silently.
   Neither source document's "preserve the `source == "git"` special case" instruction catches this —
   it would preserve the bug, not fix it. Folded into §5 Phase 1.
3. **The two plans set three different rules for "matcher set is empty" and disagree on which diff
   engine to reuse.** See §3 for the reconciled decisions.
4. **Both plans' core insight is correct and is the organizing idea for this document:** the collector
   ↔ pipeline matching decision is computed independently in five-plus places
   (`internal/serve/compute.go`, `internal/mgmtapi/rpc_reconcile.go`'s `reconcileServed`,
   `previewMatchedCollectors`, `stage3Check`, the wizard preview, `merge.DiffMatches`, and
   `shepherd admin audit-matcher-impact`), and `reconcileServed`'s bug (§1 items below) is a direct
   consequence of that duplication, not an isolated mistake. Patching `reconcileServed` alone (as the
   impact-preview plan's narrowed Phase 1 does) fixes the one instance that's been caught; it does not
   prevent the next one. This document's Phase 1 fixes the immediate bug *and* removes the duplication
   that produced it.

Everything the two source plans got right (both did substantial, verified fact-checking against `main`)
is carried forward here without re-litigating it. This document does not re-derive what they already
confirmed correct; it corrects the seven items above and the conflicts in §3, then gives one sequenced
plan.

---

## 1. Problem statement (reconciled)

Shepherd routes pipeline configuration to collectors via Alertmanager-style label matchers
(`internal/merge.MatchesPipeline`). Three problems compound today:

1. **No live feedback while editing.** `PreviewMatches` re-loads a pipeline's *already-saved* matchers
   by ID; nothing lets an author preview an in-progress, unsaved edit. `PipelineEditorPage.tsx` does
   client-side regex shape-checking only (`web/src/visual/matcher.ts`) — no match-count feedback at all.
2. **The matching decision is computed in more than one place, and they have drifted apart.**
   `internal/merge.BuildCollectorLabels` (`internal/merge/merge.go:73-97`) is the correct, gated,
   documented way to build a `CollectorLabels`. Five call sites use it correctly
   (`internal/serve/compute.go:105`, `rpc_pipeline.go:1189`, `rpc_pipeline.go:1279`, `rpc_wizard.go:152`,
   `internal/gitsync/reconciler.go:507`). One does not: `internal/mgmtapi/rpc_reconcile.go:90-93`
   (`reconcileServed`) builds `merge.CollectorLabels{Labels: {"role":…, "cluster":…}}` directly, with no
   admin labels and no local attributes. For any org with `allow_label_matching` or
   `allow_local_attribute_matching` on, `reconcileServed`'s desired-state view is wrong: it omits
   pipelines that genuinely match via a label or attribute, which surfaces as a false "collector running
   a pipeline its desired state doesn't include" drift finding on the reconciliation page. And
   `PreviewMatches` itself has the git-sourced bug in item 2 of §0 above — a sixth divergence, not caught
   by the five-call-site framing either source plan used.
3. **Two independent gaps, not one, in what's actually enforced:**
   - **Matcher syntax** *is* validated server-side today, for every source, via `validateSaveInput`
     (`internal/mgmtapi/rpc_pipeline.go:427-460`, `labels.ParseMatcher`). This is not the gap.
   - **Non-emptiness is not.** `len(matchers) == 0` is accepted with `200 OK` by `CreatePipeline` and
     `UpdatePipeline`; it only becomes inert later, at merge time
     (`internal/merge/merge.go:114-116`, "match nothing" safety default) — and produces **no recorded
     reason anywhere**, not even the assembled config's header comment. A pipeline with zero matchers is
     silently invisible everywhere: to its author, to `Assemble`'s exclusion list, to the reconciliation
     view.
4. **Targeting UX is inconsistent across the three editing surfaces** (raw editor, Visual Builder,
   wizard) — only the Visual Builder's `Toolbar.tsx` has both a working non-empty gate and syntax
   validation; `PipelineEditorPage.tsx` has neither client-side; the wizard's matcher list is read-only.
   Git-sourced pipelines render their real, disabled matchers next to a "read only" banner, which is
   vestigial (they don't participate in matching) but not literally empty as originally believed.

---

## 2. Goals / non-goals

**Goals**
- One shared targeting evaluator used by every consumer that answers "does this pipeline match this
  collector, and would it survive enforcement" — preview, reconciliation, serving, wizard, diff, and the
  audit CLI all call the same code path. No second implementation to drift.
- Live, debounced preview of an *unsaved* matcher/content draft, showing a three-way diff
  (newly matched / still matched / no longer matched) against what's currently saved and enabled.
- Server-side non-emptiness enforcement at the point that actually matters (enabling a pipeline — see §3
  decision 2 for why not at every save), with the reason recorded and visible, not just silently inert.
- One shared `<MatcherEditor>` component across the raw editor, Visual Builder, and wizard.
- Preview and reconciliation stay cheap at fleet scale: one batched collector/attribute fetch per call,
  not N+1, with a server-side cap and rate awareness.

**Non-goals (this iteration — call out as fast-follows)**
- Full per-collector merge dry-run on every keystroke — only the newly-matched delta, optionally
  (§6 Phase 3, task 3.6).
- A combined `local_attributes` per-key union across concurrent instances — `docs/spec.md` §6.1 already
  documents single-most-recent-instance as a deliberate simplification (2026-09-18). Out of scope.
- Any change to `allow_label_matching` / `allow_local_attribute_matching` gating semantics or their
  rollout tooling (`shepherd admin audit-matcher-impact`) — that's a separate, already-closed initiative.
- Re-opening `orgs.allow_label_matching`'s default-off posture.

---

## 3. Fixed decisions (do not relitigate without asking — this section is what replaces the two plans'
conflicting rules)

1. **One evaluator, one call site per consumer.** Introduce
   `internal/merge.Evaluate(p Pipeline, cl CollectorLabels, opts ...AssembleOption) EvalResult` wrapping
   the existing `MatchesPipeline` + the role/signal-enforcement logic `enforce.go` and
   `reconcileServed` both currently reimplement separately. `EvalResult` carries `Matched bool`,
   `Served bool` (matched **and** survives enforcement), and `Reason` (one of: `""` for served,
   `"unparsable_matcher"`, `"role_signal_mismatch"`, `"zero_matchers"`, `"signals_undetermined"`).
   `reconcileServed`, `previewMatchedCollectors`, the new preview handler, and `Assemble`'s exclusion
   computation all call this one function. This is the structural fix for §0 item 4 — it is not optional
   scope, it is the reason `reconcileServed` drifted in the first place.
2. **Zero-matchers is enforced at *enable*, not at every save.** The redesign plan's B1 (reject empty
   matchers in `CreatePipeline`/`UpdatePipeline`) breaks `RestoreRevision`, which also calls
   `validateSaveInput` (`rpc_pipeline.go:994`) — rolling back to a pre-existing zero-matcher revision
   during an incident would start failing. It also blocks content-only edits to a pipeline the org has
   deliberately grandfathered as zero-matcher (decision 4 below). Correct gate: block **`EnablePipeline`**
   (not `Create`/`Update`/`Restore`) when `source ∈ {ui, wizard, visual}` and matchers are empty. A
   disabled pipeline may be saved and edited freely with zero matchers (it's inert either way); a
   pipeline cannot be turned on in a state that will silently match nothing.
3. **Zero *matched collectors* (as opposed to zero matchers) is a warning, not a block.** Pre-provisioning
   a pipeline for a fleet segment that isn't enrolled yet is legitimate. The preview panel shows a
   static, always-visible warning when the draft matches 0 collectors; it does not disable Save or
   Enable. This is a different case from decision 2 and must use different, clearly distinguishable
   copy — see decision 8.
4. **Existing zero-matcher rows are grandfathered, not migrated or backfilled.** Decision 2 only gates the
   `EnablePipeline` transition going forward. A row that is already enabled with zero matchers keeps
   running (or rather, keeps matching nothing) exactly as today — no startup scan, no forced disable.
   Before implementation starts, run the fleet audit in §7 open question 1 to know the actual blast
   radius of this decision.
5. **The three-way preview diff (`NEWLY_MATCHED`/`STILL_MATCHED`/`NO_LONGER_MATCHED`) is computed by the
   `PreviewMatches` handler directly, calling the evaluator from decision 1 twice** (once against the
   saved-and-enabled baseline, once against the draft) **— it does not go through
   `merge.DiffMatches`.** `DiffMatches(pipelines, before, after CollectorLabels)` (`internal/merge/
   matchdrift.go:21-40`) compares **one collector's label set, before/after, against many pipelines** —
   it's shaped for "an admin label changed, which pipelines flip for this collector," which is what it's
   already wired for (`rpc_fleet.go`'s `emitMatchDrift`, `agentapi/service.go`'s
   `emitLocalAttrsMatchDrift`). Previewing a pipeline edit is the transposed shape — **one pipeline,
   many collectors, before/after matcher sets** — and does not fit `DiffMatches`'s signature without
   forcing an awkward per-collector call in a loop. The redesign plan's C2 (diff-before-save on an
   already-enabled pipeline) **is** the same shape as `DiffMatches` (one pipeline's old/new matcher set
   against the fleet, expressed as iterating collectors) and should reuse it as originally proposed —
   the two workstreams were conflated in the source plans; they use different tools:
   - **Live keystroke preview (impact-preview plan's Phase 3, redesign plan's C1):** new matchers vs.
     draft, per collector, via the evaluator directly.
   - **Diff-before-save confirm dialog (redesign plan's C2):** old saved matchers vs. new draft matchers,
     for one pipeline, via `merge.DiffMatches` called once per candidate collector, or (preferred,
     smaller diff) a thin `DiffMatchesForPipeline(pipeline, oldMatchers, newMatchers, collectors []CollectorLabels) []DiffEntry` helper added next to `DiffMatches` that reuses its per-pipeline comparison logic transposed. Decide the exact shape in Phase 2 (§5, task 2.1) rather than here — it's an implementation
     detail once the semantic split above is agreed.
6. **`PreviewMatches` derives `source` from the loaded/draft pipeline server-side; it is never a
   client-supplied field.** Both source plans' proto sketch put `source` on the request. A git pipeline's
   match behavior is fully determined by `RepoLinkCollectorID`, which the server already has once a
   pipeline exists, or which an unsaved draft simply doesn't have (drafts are always `ui`/`wizard`/
   `visual` — nothing lets you draft a git-sourced pipeline in the editor). Dropping the field removes a
   class of "client lies about its own source" input validation entirely.
7. **The request's draft field is a message, not a bare `repeated string`, so "not sent" and "sent but
   empty" are distinguishable.** `repeated string matchers = 3` can't express "mid-edit, author hasn't
   typed a matcher yet" vs. "use the saved matchers" — both serialize to an absent/empty repeated field
   in proto3. Use `optional MatcherDraft draft = 3;` with `message MatcherDraft { repeated string
   matchers = 1; }` — presence of `draft` means "evaluate this draft, even if its matchers list is
   empty"; absence means "use the saved pipeline's matchers." This directly enables the empty-draft test
   both source plans flagged as a gap (impact-preview plan's own §11 item 4, redesign plan's §11 item 3)
   to actually be expressible in the contract, not just in a handwritten Ginkgo fixture that bypasses the
   wire format.
8. **One shared empty-state string module, `web/src/matcherMessages.ts`,** used by `<MatcherEditor>`
   (decision 3's "0 matched collectors" warning), the `EnablePipeline` failure toast (decision 2's
   block), and the exclusion panel (§5 Phase 5, Workstream E). This directly closes the gap the
   impact-preview plan's §8 item 4 flagged (does the preview panel's zero-state and the Save-disabled
   state say the same thing) by construction rather than by convention.
9. **`ValidateMatchers` is replaced by `CompileMatchers`.** Both source plans proposed
   `internal/merge.ValidateMatchers([]string) error`, parsing-only. Because the evaluator (decision 1)
   and the preview handler both need the *parsed* `labels.Matcher` values, not just a yes/no, add
   `internal/merge.CompileMatchers(raw []string) ([]*labels.Matcher, error)` instead — parses once,
   returns the compiled matchers, wrapping the first error exactly as today's inline loop does
   (`fmt.Errorf("matcher %q is not valid: %w", m, perr)`, matching `rpc_pipeline.go:449-452`'s existing
   shape so callers' `InvalidArgument` mapping doesn't change). `validateSaveInput` and
   `MatchesPipeline` both call it; `MatchesPipeline` stops re-parsing the same strings once per
   collector per call.
10. **RBAC on the extended `PreviewMatches`: unchanged role (`RoleOrgReader`,
    `rpc_interceptor.go:103`), but the handler gains the team-ownership filter it's missing today.**
    `previewMatchedCollectors` calls `ListCollectorsByOrg` (`rpc_pipeline.go:1272`) — org-wide, no team
    scoping — while `loadPipeline` (used by every other pipeline RPC) enforces per-pipeline ownership via
    `authorizeOwnership` (`rpc_pipeline.go:312-...`, G11 per its own comment). An org-reader on Team A can
    today preview-match a Team B pipeline's matchers against the *entire org's* collector fleet, which is
    an existing information-disclosure gap this work touches directly (the new draft-preview path adds a
    second way to hit it — see §4). Decision: keep `RoleOrgReader` as the floor, but scope the collector
    set returned to what the caller's role/team allows before it flows into the evaluator, and treat
    closing the *existing* gap in `previewMatchedCollectors` as in-scope (§5 Phase 1, task 1.4) since
    Phase 1 already touches every call site that builds a `CollectorLabels`.
11. **`fleet.proto`'s stale `Collector.labels` comment gets corrected as part of Phase 4** (folds the
    redesign plan's decision 6 in) — it currently reads "independent of Alloy attributes and pipeline
    matching," which has been false since `BuildCollectorLabels` shipped.
12. **Who decides "go":** neither source plan named an owner for the proto-shape sign-off or the
    "when does this merge to `main`" call. Both self-reviews flagged this. This document does not invent
    an org chart — it makes the gap explicit here instead of leaving it implicit, and the Phase 0 task in
    §5 is to get that name on record before Phase 2's proto ask-first request goes out. Do not proceed
    past Phase 1 without it.

---

## 4. Security considerations

1. **Enumeration oracle, existing and expanded — team-scoping action RETRACTED, 2026-09-24 (see Phase 1
   task 1.5).** Once `PreviewMatches` accepts an arbitrary draft matcher set (this plan) from an
   `org-reader`, a reader can iterate label values across the whole org's fleet — `cluster=~".+"` then
   binary-search on value. This is real and unaffected by the note below: it stays exactly as risky as
   `org-reader` visibility into the org's fleet already is everywhere else in the app (the fleet page,
   `stage3Check`, `recomputeOrgCaches` all show the same org-wide collector set to any org-reader). What
   is **not** real: "even for collectors their team doesn't own" — investigated in task 1.5, no
   team-to-collector ownership model exists, and no other read path in the app is team-scoped, so there is
   no narrower set to scope down to. Do not add team-scoping to Phase 3's draft-preview path on this
   item's authority; it was written on the same mistaken premise task 1.5 corrected.
2. **Draft path skips the org-ownership check `loadPipeline` normally provides.** The `id=""` /
   draft-only call has no pipeline row to check ownership against. It must still validate the caller's
   `org_id` against their session (not fall back to a zero UUID the way today's handler does on a bad
   `org_id`, `rpc_pipeline.go:808-811` — that fallback is acceptable for the “preview my own already-
   loaded pipeline” path but not for a caller-supplied blank-slate org scope).
3. **Agent-reported `local_attributes` remain lower-precedence than admin labels** (already true,
   `BuildCollectorLabels`'s doc comment, `merge.go:83-89` — a compromised agent token cannot shadow an
   admin-set label). This plan does not change that precedence. It does surface, for the first time, a
   per-collector "which label source did this match on" breakdown in the preview panel (§6.2's
   `MatchedCollector.matched_on`) — this is a *defensive* addition: it lets an author notice a
   surprising `local_attributes`-sourced match before saving, rather than discovering it via drift later.
4. **Resource limits on the evaluator.** `CompileMatchers` runs `labels.ParseMatcher`/RE2 compilation
   once per call (decision 9 removes the per-collector re-parse), but a pathological client can still
   submit many matchers with expensive regex bodies. Cap matcher count (suggest 20) and per-matcher
   string length (suggest 256 bytes) in the same validation pass that already rejects bad syntax — RE2
   has no catastrophic-backtracking class of attack, but compile cost and match cost both still scale
   with input size and fleet size multiplicatively.
5. **No rate limit exists on `PreviewMatches` today**, and this plan makes it more attractive to call in
   a tight loop (a debounced client is still a client; a second tab or a buggy retry bypasses the
   debounce). §6.4's per-org token-bucket limiter closes this — flagged as open but unresolved by the
   redesign plan's own §8 item 2.
6. **Audit trail.** `internal/mcp/propose.go:752-774` already writes an audit row when a *machine*
   identity calls `PreviewMatches`, deliberately skipping human keystroke-level calls (its own comment:
   "an audit trail nobody can read is its own failure"). Preserve exactly this behavior for the extended
   draft-preview path — do not start auditing human preview keystrokes, do continue auditing machine
   (service-account / MCP) draft-preview calls, and additionally record the before/after match-count
   delta on `EnablePipeline`/`UpdatePipeline` writes themselves (not just previews) via the existing
   `auditLogDetail` mechanism, since a save's actual effect is the thing worth being able to reconstruct
   later, not just a preview someone looked at and didn't act on.
7. **Truncation must not hide the dangerous direction.** A capped response (§6.2, cap 200) must sort
   `NO_LONGER_MATCHED` entries first and report per-status counts independent of the cap — silently
   dropping the collectors that are about to *lose* config is the failure mode a preview feature exists
   to prevent (impact-preview plan §3 already makes this point; it's restated here because it's a
   security-adjacent availability concern, not just a UX one — a collector silently losing its
   remote_write pipeline is a monitoring gap, and monitoring gaps hide incidents).

---

## 5. API contract (Phase 2 — single ask-first proto request, replacing both plans' separate proto asks)

`AGENTS.md`'s "Ask first" list includes `proto/` changes (confirmed verbatim, `AGENTS.md:69`). Bundle
every proto change this plan needs into **one** sign-off request, not the three the two source plans
would have produced separately (extended `PreviewMatches`, exclusion-reason field on `GetPipeline`,
`ListAttributes` admin-key union).

```protobuf
// --- pipeline.proto ---

message MatcherDraft {              // NEW — decision 7
  repeated string matchers = 1;
}

message PreviewMatchesRequest {
  string org_id = 1;
  string id = 2;                    // may be "" for an unsaved draft
  optional MatcherDraft draft = 3;  // NEW — presence, not emptiness, selects draft mode (decision 7)
  // source is intentionally NOT a field here — derived server-side (decision 6)
}

enum MatchStatus {
  MATCH_STATUS_UNSPECIFIED = 0;
  MATCH_STATUS_NEWLY_MATCHED = 1;
  MATCH_STATUS_STILL_MATCHED = 2;
  MATCH_STATUS_NO_LONGER_MATCHED = 3;
}

enum LabelSource {                  // NEW — decision 1 / §4 item 3, "which source matched"
  LABEL_SOURCE_UNSPECIFIED = 0;
  LABEL_SOURCE_BUILTIN = 1;         // cluster / role
  LABEL_SOURCE_ADMIN_LABEL = 2;
  LABEL_SOURCE_LOCAL_ATTRIBUTE = 3;
}

message MatchedCollector {
  string cluster = 1;
  string role = 2;
  string id = 3;
  MatchStatus status = 4;           // NEW
  repeated LabelSource matched_on = 5;  // NEW — which source(s) the matching label(s) came from
}

message PreviewMatchesResponse {
  repeated MatchedCollector collectors = 1;
  int32 total_collectors = 2;       // NEW
  int32 newly_matched_count = 3;    // NEW — decision applies independent of truncation, §4 item 7
  int32 no_longer_matched_count = 4; // NEW
  bool truncated = 5;               // NEW
}

// --- fleet.proto ---
// Collector.labels comment corrected (decision 11):
// was: "UI-managed grouping labels, independent of Alloy attributes and pipeline matching."
// now: "UI-managed grouping labels. Participate in pipeline matching when the org has
//       allow_label_matching enabled (see internal/merge.BuildCollectorLabels)."

// ListAttributesResponse gains admin-label keys, gated per §4 decision 5 of the original
// redesign backlog (unchanged by this document): union collectors.labels keys into the
// existing local_attributes/cluster/role response when allow_label_matching is on for the org.

// --- GetPipeline / a new exclusion surface (Workstream E, Phase 5) ---
message Exclusion {                 // NEW
  string reason = 1;   // one of: "unparsable_matcher" | "role_signal_mismatch" | "zero_matchers"
  string detail = 2;   // human-readable, e.g. which matcher failed to parse
}
// Added as a field on GetPipelineResponse (or a new lightweight RPC — decide in Phase 2 review;
// GetPipeline is the more natural home since exclusions are per-pipeline, per-collector-set state
// already computed at request time).
```

Verify: `make generate` regenerates `gen/shepherd/mgmt/v1` and `web/src/gen/shepherd/mgmt/v1`;
`go build ./...`; `pnpm -C web typecheck`.

---

## 6. Phased implementation plan (tasks for the next agent)

Each task lists files, steps, a concrete verification command, and dependencies. Tasks marked
**ASK FIRST** require explicit sign-off before proceeding past drafting, per `AGENTS.md`.

### Phase 0 — Gates and planning artifacts
- **0.1 — Re-check the hold.** `gh pr view 12 --repo ehariDev/shepherd --json state,mergedAt` and
  `gh pr view 11 --repo ehariDev/shepherd --json state,mergedAt`. Do not proceed past this task until
  both show `state: MERGED`.
- **0.2 — Name the decision-maker (decision 12).** Get an explicit name/role on record for (a) the
  Phase 2 proto sign-off and (b) "who says this merges to `main`." Put it in this file's status block
  once known.
- **0.3 — Run the grandfather audit (decision 4 / §7 open question 1).** Query production (or the
  closest available environment) for `count(*) from pipelines where source in ('ui','wizard','visual')
  and jsonb_array_length(matchers) = 0 and enabled = true`. If the count is non-trivial, decide whether
  those owners need a one-time notification before Phase 3 ships the `EnablePipeline` gate (which does
  not affect already-enabled rows, but the number informs whether Workstream E's exclusion surface
  should be fast-tracked ahead of the gate so those owners can *see* the problem before anyone tightens
  anything further).
- **0.4** Commit this file at `docs/plans/2026-09-24-matcher-targeting-unified-plan.md`; add one line to
  `docs/project-status.md`'s open-work ledger (`AGENTS.md:7`: "the single live ledger... do not start a
  second one"); delete/archive the two superseded planning docs' ledger lines if they added any.
  *Depends on:* 0.1, 0.2.

### Phase 1 — Fix the matching engine and its duplication (independently shippable, ship first)
This is the highest-leverage phase: it fixes a live, silent bug (`reconcileServed`) and a second,
newly-found one (`PreviewMatches` + git pipelines), and removes the structural cause of both before
anything is built on top of the duplicated logic.

- **1.1 — Add `internal/merge.CompileMatchers`** (decision 9). New function next to `MatchesPipeline` in
  `internal/merge/merge.go`. Signature: `CompileMatchers(raw []string) ([]*labels.Matcher, error)`.
  Update `MatchesPipeline` to accept pre-compiled matchers where a caller already has them (add an
  internal `matchesCompiled(matchers []*labels.Matcher, cl CollectorLabels) bool` helper; keep
  `MatchesPipeline`'s existing `(Pipeline, CollectorLabels) (bool, error)` signature as a thin wrapper
  calling `CompileMatchers` once, for callers that don't pre-compile).
  *Verify:* `go test ./internal/merge/...` — new table test asserting identical error text/wrapping to
  today's inline loop (`rpc_pipeline.go:449-452`'s error shape).
- **1.2 — Add `internal/merge.Evaluate`** (decision 1). New file `internal/merge/evaluate.go`.
  ```go
  type EvalResult struct {
      Matched bool
      Served  bool   // Matched && survives role/signal enforcement
      Reason  string // "" | "unparsable_matcher" | "role_signal_mismatch" | "zero_matchers" | "signals_undetermined"
  }
  func Evaluate(p Pipeline, cl CollectorLabels, schema *ComponentSchema) (EvalResult, error)
  ```
  Extract the shared logic from `reconcileServed`'s loop body (`rpc_reconcile.go:97-131`, specifically
  the "match, then derive signals, then enforce" sequence) and `Assemble`'s per-pipeline exclusion
  computation (`merge.go:148-...`) into this one function. Both then call it instead of reimplementing.
  *Verify:* new golden test asserting `Evaluate` agrees with `Assemble`'s existing exclusion behavior for
  every fixture in `internal/merge`'s existing test suite (parity test — this is the test that proves
  the extraction didn't change behavior).
- **1.3 — Wire `reconcileServed` to `BuildCollectorLabels` + `Evaluate`.** Replace
  `rpc_reconcile.go:90-93`'s inline `merge.CollectorLabels{Labels: {"role":…, "cluster":…}}` with the
  same org-lookup → `adminLabelsIfAllowed` → local-attrs → `BuildCollectorLabels` pattern
  `previewMatchedCollectors` already uses (`rpc_pipeline.go:1268-1288`), then call `merge.Evaluate`
  instead of the hand-inlined match+derive+enforce sequence.
  *Verify:* regression test asserting a collector matched only via an admin label or local attribute
  shows up in `reconcileServed`'s desired-state list — fails on current `main`, passes after.
- **1.4 — Fix `PreviewMatches` for git-sourced pipelines** (§0 item 2). In
  `rpc_pipeline.go:803-838`, populate `mp.RepoLinkCollectorID` from the loaded pipeline row (the field
  already exists on the DB row per `reconcileServed`'s own use of `repoLinkCollectorID(ep.RepoLinkCollectorID)`, `rpc_reconcile.go:105`).
  *Verify:* new test previewing a git-sourced pipeline asserts it returns its actual linked collector,
  not zero — fails on current `main`.
- **1.5 — RESOLVED as no-op, 2026-09-24 (decision 10 / §4 item 1 / §7 open question 2).** Investigated
  before implementing, per this document's own open question 2. Finding: **the premise doesn't match the
  code.** `loadPipeline` (used by `GetPipeline` and every other pipeline RPC) enforces only that a
  pipeline belongs to the request's org — it does **not** call `authorizeOwnership`/G11; G11 gates
  **writes** only (Create/Update/Delete/Enable/Disable), never reads. Any org-reader can already
  `GetPipeline` any pipeline in the org today, team-owned or not. Separately, **there is no team-to-
  collector ownership model at all** — `teams`/`owner_team_id` own *pipelines*; the only per-collector
  access concept, `group_assignments` (IdP group → collector), is used solely as one of several fallback
  paths to decide whether a session clears the org-reader *floor* in `authorizeOrgAccess`, never to filter
  which collectors a reader sees afterward. Every other org-wide collector listing
  (`ListCollectorsByOrg` via `previewMatchedCollectors`, `recomputeOrgCaches`, `stage3Check`, the fleet
  page) shows the whole org's fleet uniformly once that floor is cleared, by any path. Scoping
  `previewMatchedCollectors` alone would be a new, one-off restriction inconsistent with how every other
  read in the app works, built on an ownership axis (team → collector) that doesn't exist as data.
  **Decision (repo owner, 2026-09-24): confirmed intentional — org-reader is an all-or-nothing floor for
  an org's fleet visibility, not further scoped by team. No code change.** Superseded by decision 10 and
  §4 item 1's framing above, which assumed the gap existed as described.
- **1.6 — Add a repo-wide guard against recurrence.** A `repocheck` rule (alongside existing ones in
  `scripts/repocheck/`) that fails on a `merge.CollectorLabels{` struct literal appearing outside
  `internal/merge` itself — this is exactly the pattern `reconcileServed` used and the class of bug this
  phase fixes. Confirm no other caller besides the now-fixed `rpc_reconcile.go` currently matches (per
  the impact-preview plan's own §7 follow-up, `internal/cli/admin.go`'s `audit-matcher-impact` does its
  own before/after diff correctly and should not trip this rule — verify explicitly).
  *Verify:* `go test ./scripts/repocheck/...`; rule fires on a deliberately reintroduced literal in a
  throwaway test file, then passes once removed.
- **1.7 — Close out.** `make lint && make test`. `CHANGELOG.md` entry under `Fixed`: fleet reconciliation
  now correctly accounts for label/attribute-matched pipelines (previously under-counted for orgs with
  either flag on); `PreviewMatches` now returns correct results for git-sourced pipelines (previously
  always empty). Merge as its own PR before Phase 2 starts — two real, independent correctness fixes
  deserve their own changelog lines and shouldn't be buried inside a feature PR.
  *Depends on:* 1.1–1.6.

### Phase 2 — Proto contract — **ASK FIRST**
- **2.1** Present §5's combined proto sketch for sign-off — one request covering `PreviewMatches`'s
  extension, the `Exclusion` message, and the `fleet.proto` comment fix. Get the person named in 0.2 (or
  their delegate) to confirm the `EvalResult`/`Evaluate` shape from Phase 1 maps cleanly onto the
  `MatchStatus`/`LabelSource`/`Exclusion` proto fields before generating code — Phase 1 lands first
  specifically so this mapping is checked against real, running code, not a sketch.
  *Depends on:* Phase 1 merged, 0.2.
- **2.2 — Codegen.** `make generate`. *Verify:* `go build ./...`; `pnpm -C web typecheck`.
  *Depends on:* 2.1 approval.

### Phase 3 — Backend: draft preview + enable-time guardrail
- **3.1 — Rewrite `PreviewMatches` handler.** Load collector snapshot once (batched query — retire the
  `previewMatchedCollectors` per-row `GetClusterByID` N+1 at `rpc_pipeline.go:1276`); baseline = saved
  pipeline's matchers if `id` set (empty if not); draft = `req.draft.matchers` if `draft` is present,
  else baseline (decision 7); call `merge.Evaluate` (Phase 1) per collector for both baseline and draft;
  emit `MatchStatus` per the three-way diff; populate `matched_on` from which label source(s)
  contributed the deciding match; sort `NO_LONGER_MATCHED` first, cap at 200, report
  `total_collectors`/`newly_matched_count`/`no_longer_matched_count` independent of the cap (§4 item 7).
  *Verify:* Ginkgo specs — new pipeline (all `NEWLY_MATCHED`), matcher removal (`NO_LONGER_MATCHED`
  entries present), truncation at the cap with correct counts still reported, git-sourced pipeline uses
  its real linked collector (regression test for 1.4), **empty `draft.matchers` with `draft` present
  returns cleanly** (the case decision 7's message design exists to make expressible), **`draft` absent
  falls back to saved matchers**, and the team-scoping test from 1.5 re-run against the new draft path
  specifically (a new attack surface, not just the old one).
  *Depends on:* Phase 1, Phase 2.
- **3.2 — REST shim.** `internal/mgmtapi/pipelines.go`'s HTTP handler: change
  `GET /pipelines/{id}/preview-matches` to accept a request body (switch to `POST
  /pipelines/{id}/preview-matches:evaluate` or similar — a `GET` with a body is non-standard and several
  proxies strip it); update `web/src/routeCoverage.test.ts`.
  *Depends on:* 3.1.
- **3.3 — Rate limiting** (§4 item 5). Add a per-org token-bucket limiter on `PreviewMatches` (reuse
  whatever limiter primitive the codebase already has for other RPCs — grep for existing
  `golang.org/x/time/rate` or equivalent usage before adding a new dependency, since new deps are also
  ask-first per `AGENTS.md:69`).
  *Verify:* test asserting the Nth call within a window in a burst gets `ResourceExhausted`, not a
  silent full scan.
  *Depends on:* 3.1.
- **3.4 — Resource limits** (§4 item 4). Cap matcher count and per-matcher length in
  `CompileMatchers`/`validateSaveInput`'s shared validation path.
  *Verify:* test asserting a 21st matcher or a 257-byte matcher string is rejected `InvalidArgument`
  before it reaches RE2 compilation.
  *Depends on:* 1.1.
- **3.5 — `EnablePipeline` zero-matchers gate** (decision 2). In the `EnablePipeline` RPC handler, reject
  `InvalidArgument` with the shared message from decision 8 (`web/src/matcherMessages.ts`'s Go-side
  equivalent constant, or a single source-of-truth string both sides import — decide the sharing
  mechanism here, e.g. a small `internal/merge/messages.go` the frontend's generated types don't
  reach, so keep the *wording* identical by convention/test, not by a shared build artifact) when
  `source ∈ {ui, wizard, visual}` and matchers are empty. `source == git` unaffected (matches by
  collector, decision unchanged from redesign plan §4.1).
  *Verify:* table test — reject empty-matcher enable for `ui`/`wizard`/`visual`, accept for `git`;
  `RestoreRevision` of a zero-matcher revision still succeeds (regression test proving decision 2 doesn't
  reintroduce the bug decision 2 itself was written to avoid).
  *Depends on:* 1.1.
- **3.6 (optional, separable) — Dry-run tie-in.** Run the existing stage-3 merge dry-run only against the
  `NEWLY_MATCHED` delta from 3.1; attach `bool has_conflict` on `MatchedCollector` (batch into the same
  Phase 2 proto approval — do not open a second ask-first round for this alone). Treat as trimmable scope
  if timeline is tight; 3.1 is the core value.
  *Depends on:* 3.1.

### Phase 4 — Frontend: shared editor + live preview
- **4.1 — Extract `<MatcherEditor>`.** From `Toolbar.tsx`'s implementation (chips, add-row, remove,
  validation, empty-state, non-empty Save gate); render it from `PipelineEditorPage.tsx` too. **Preserve
  PR #12's `<datalist>` autocomplete mechanism inside the extraction** — do not drop it and leave a
  no-autocomplete window before task 4.3 lands the structured combobox (this was flagged as an ordering
  hazard by the source redesign plan's own §11 item 1; resolved here by making autocomplete-preservation
  part of 4.1's acceptance criteria instead of a separate later task).
  *Verify:* existing `web/tests/fullstack/matcher-edit.spec.ts` and `collector-labels.spec.ts` re-run
  against both `Toolbar.tsx` and `PipelineEditorPage.tsx`, not just the Visual Builder.
- **4.2 — `useMatchPreview` hook.** `web/src/hooks/useMatchPreview.ts`: debounced (~300ms),
  `AbortController` cancels superseded calls, sends `draft: { matchers }` per decision 7 (always present
  once the author has touched the matcher list, even if currently empty — that's exactly the case the
  message design exists to distinguish from "haven't loaded a draft yet").
  *Verify:* vitest — rapid calls collapse to one network call; an empty matcher list still sends
  `draft: {matchers: []}`, not an omitted field.
  *Depends on:* Phase 2 (regenerated TS types), 4.1.
- **4.3 — Structured key/operator/value input + combobox.** Replace the free-text matcher box with key /
  operator (`=`,`!=`,`=~`,`!~`) / value controls, serialized to the Alertmanager-style string
  server-side validation expects. Combobox sources keys from the extended `ListAttributes` (admin-label
  keys unioned in, gated on `allow_label_matching` per the redesign plan's original decision 5, carried
  forward unchanged) — a strict superset of PR #12's `<datalist>`, replacing it now that it exists.
  *Verify:* frontend unit test generating every operator × representative value, asserting the serialized
  output always passes `isValidMatcher` (now defense-in-depth, not primary validation).
  *Depends on:* 4.1, extended `ListAttributes` (Phase 2/5 — admin-key union).
- **4.4 — Wire preview panel into the editor.** Total count; "+N will start receiving this" (green,
  collapsible), "−N will stop receiving this" (red, **expanded by default** — the direction that causes
  incidents must be visually prominent), "N unchanged" (collapsed). Zero-matched-collectors state uses
  the shared string from decision 8, always visible, non-blocking. Rows link to collector detail. Show
  `matched_on` (built-in / admin label / local attribute) per row per §4 item 3's defensive-visibility
  rationale.
  *Verify:* component test with mocked hook data covering empty/all-groups/truncated states, and a test
  asserting the zero-matched-collectors message text matches the constant from decision 8 exactly (not
  independently authored inline copy).
  *Depends on:* 4.2, 4.3.
- **4.5 — Diff-before-save confirm dialog** (redesign plan's C2, decision 5's second bullet). Before
  committing a matcher change to an already-enabled pipeline, call `merge.DiffMatches` (or the small
  `DiffMatchesForPipeline` helper decided in Phase 2) with old vs. new matcher sets; show "this moves the
  pipeline off N collectors, onto M" in a confirm dialog before the write goes through. Recompute the
  diff server-side at save time (do not trust a diff the client computed and cached across the dialog's
  open time — a fleet change between preview and confirm should be reflected).
  *Verify:* fullstack test — edit a matcher on an enabled pipeline, dialog shows correct gained/lost
  counts, confirm proceeds, cancel leaves the pipeline unchanged.
  *Depends on:* 1.2 (`Evaluate`)/`DiffMatches`, 4.4.
- **4.6 — Wizard parity (Workstream G).** Swap `WizardRunnerPage.tsx`'s read-only
  `isWizardAddedMatcher`-badged chip list for `<MatcherEditor>` pre-filled with derived matchers,
  editable, live-previewed via 4.2.
  *Verify:* existing wizard fullstack tests re-run against the new editable component.
  *Depends on:* 4.1, 4.2.
- **4.7 — Git-sourced pipeline panel (Workstream D).** Replace the vestigial disabled-matcher-chips
  display with a read-only "Targeted via Git" panel (linked collector + repo-link name); add a route
  param to `/git` (`?repoLinkId=` or `/git/:repoLinkId` — check existing deep-linkable pages in
  `web/src/routes/router.tsx` for precedent before picking arbitrarily) that `GitPage.tsx` reads to
  scroll-to/highlight the relevant repo link.
  *Verify:* frontend test with a `source: 'git'` fixture pipeline asserting the panel renders and its
  link's route param matches the fixture's `repo_link_id`.
  *Depends on:* none beyond Phase 1 (can land early/in parallel once Phase 0's hold clears — lowest-risk
  starting point per both source plans' sequencing).
- **4.8 — Test suite + SPA rebuild.** `pnpm -C web test`; `make test-ui` (mocked Playwright): add a
  matcher → panel updates; remove a matching matcher → collector shows red; confirm dialog appears for an
  enabled pipeline's matcher edit. Run `make web-ci` before opening the PR (`pnpm lint` alone skips
  typecheck/tests/build). Rebuild embedded SPA: `./scripts/build-web.sh`, stage `internal/spa/dist/`
  (`check-dist-consistency` guard requires this — server embeds via `go:embed`).

### Phase 5 — Exclusions surfaced to the author (Workstream E)
- **5.1 — Add the zero-matchers exclusion reason to `Assemble`.** `internal/merge/merge.go`'s
  `Assemble` already records `Exclusion{PipelineName, Reason}` for unparsable matchers and role/signal
  mismatches; a zero-matcher pipeline produces no `Exclusion` at all today. Add one, using
  `Evaluate`'s (Phase 1) `Reason: "zero_matchers"` output as the source of truth so this doesn't drift
  from Phase 1's evaluator a second time.
  *Verify:* new unit test asserting `AssembleResult.Exclusions` contains a zero-matchers entry for a
  fixture pipeline that previously produced none.
- **5.2 — Confirm no existing `Exclusions` consumer assumes only two reason categories.** `stage3Check`
  and `recomputeOrgCaches` both already log `Exclusions` today (per the redesign plan's §8 item 3) —
  check for any code keyed on reason-string prefixes before adding the third category; fix or confirm
  clean.
- **5.3 — Return exclusions through `GetPipeline`** (proto field from Phase 2). Pipeline detail page
  shows "excluded from collector X: [reason]" using decision 8's shared string module for the
  zero-matchers case specifically, so its wording matches the preview panel and the enable-time error
  exactly.
  *Verify:* frontend test asserting all three reason strings render distinctly and the zero-matchers one
  is character-identical to the `EnablePipeline` rejection message and the preview panel's warning.
  *Depends on:* 5.1, Phase 2, Phase 4 (shared string module).

### Phase 6 — Docs and release hygiene
- **6.1** Update `docs/spec.md` §6.1 (merge engine / matchers section) to describe the `Evaluate`
  function as the canonical decision point and document the `EnablePipeline` zero-matchers gate.
- **6.2** Edit `scripts/docs-content/` (source for `matchers.html`); `make docs`; `make check-docs-drift`.
  Correct any prose that still describes `Collector.labels` as non-matching, matching the proto comment
  fix from Phase 2.
- **6.3** `CHANGELOG.md` — use the existing Shipped / RPC only / Built-not-wired taxonomy per phase as it
  lands; the reconciliation and git-preview fixes (Phase 1) already got their own entry in 1.7.
- **6.4** Final gate before each PR: `make lint && make test && go test ./scripts/repocheck/ && make
  web-ci && make test-ui`. Conventional-commit subject per phase, e.g.
  `fix(reconcile): route reconcileServed through BuildCollectorLabels` (Phase 1),
  `feat(pipelines): draft matcher preview with three-way diff` (Phase 3–4).

---

## 7. Open questions (carried and reconciled from both source plans)

1. **How many currently-enabled pipelines have zero matchers today?** (Phase 0, task 0.3.) Answers
   whether decision 4's grandfathering needs a one-time owner notification alongside it.
2. **RESOLVED 2026-09-24 — see Phase 1 task 1.5.** `previewMatchedCollectors`'s org-wide (not team-scoped)
   visibility is confirmed intentional, not a bug: no team-to-collector ownership model exists to scope
   by, reads are never team-gated anywhere in the codebase (only writes, via G11), and every other
   org-wide collector listing shows the whole fleet once the org-reader floor is cleared by any path. No
   code change.
3. **`LABEL-MATCHING-PLAN.md`, cited by nine commit messages and `docs/spec.md` §6.1 as the authoritative
   design doc behind the already-shipped admin-label/local-attribute matching work, could not be found**
   anywhere reachable from this session (exhaustive `git log --all -S`, every branch/tag, working tree,
   this Downloads location). Decisions 2–4 of this document partly extend reasoning inferred from commit
   messages and `docs/spec.md` rather than that document's own stated reasoning. Recommend this block
   Phase 3 (not just be "worth confirming") until whoever wrote it, or has access to it, confirms no
   contradiction — upgraded from the redesign plan's softer framing per its own §11 item 2 self-review.
4. **The REST route rename in Phase 3 task 3.2** (`GET` → `POST … :evaluate`) is a breaking change for
   any external caller of `GET /pipelines/{id}/preview-matches` that isn't in this codebase (e.g., a
   customer script hitting the documented REST API directly, per `docs/spec.md:638`). Confirm whether
   REST API backward compatibility is a concern for this endpoint before shipping 3.2, or whether keeping
   the old `GET` route as a deprecated alias (id-only, no draft support) alongside the new `POST` route
   is warranted.
5. **Workstream D1's route param shape** (`?repoLinkId=` vs. `/git/:repoLinkId`) has no existing
   precedent in this app per the redesign plan's own finding — pick one by checking other deep-linkable
   pages' conventions at implementation time (Phase 4 task 4.7), not here.

---

## 8. Test matrix (consolidated)

| Case | Layer | Phase |
|---|---|---|
| `reconcileServed` includes label/attribute-matched pipelines | `internal/mgmtapi` | 1 |
| `PreviewMatches` returns real results for a git-sourced pipeline | `internal/mgmtapi` | 1 |
| `previewMatchedCollectors` never leaks a Team-B collector to a Team-A reader | `internal/mgmtapi` | 1, 3 |
| `merge.Evaluate` parity with `Assemble`'s existing exclusion behavior | `internal/merge` | 1 |
| `CompileMatchers` error text matches today's inline loop | `internal/merge` | 1 |
| `repocheck` catches a reintroduced `merge.CollectorLabels{...}` literal outside `internal/merge` | `scripts/repocheck` | 1 |
| Draft preview: new pipeline all `NEWLY_MATCHED`; matcher removal produces `NO_LONGER_MATCHED`; truncation at 200 with correct independent counts; `draft` absent vs. present-empty distinguished | `internal/mgmtapi` | 3 |
| Rate limiter rejects a burst past the per-org threshold | `internal/mgmtapi` | 3 |
| Matcher count/length caps reject before RE2 compilation | `internal/merge`/`internal/mgmtapi` | 3 |
| `EnablePipeline` rejects empty matchers for `ui`/`wizard`/`visual`, accepts `git`; `RestoreRevision` of a zero-matcher revision still succeeds | `internal/mgmtapi` | 3 |
| `<MatcherEditor>` behaves identically in `Toolbar.tsx` and `PipelineEditorPage.tsx`, autocomplete preserved through extraction | `web/tests/fullstack` | 4 |
| Structured key/operator/value input can never serialize an invalid matcher string | `web` unit | 4 |
| Preview panel zero-matched-collectors text is byte-identical to `EnablePipeline`'s rejection text and the exclusion panel's text | `web` component | 4, 5 |
| Diff-before-save dialog shows correct gained/lost counts, recomputed server-side at save | `web/tests/fullstack` | 4 |
| `AssembleResult.Exclusions` contains a distinct zero-matchers entry | `internal/merge` | 5 |
| No existing `Exclusions` consumer breaks on a third reason category | `internal/mgmtapi` | 5 |
| Git pipeline detail shows "Targeted via Git" panel, deep-links correctly | `web` fullstack | 4 |
| Full regression: existing Visual Builder + admin-label/local-attr matching behavior unchanged | `web/tests/fullstack` | all |

---

## 9. Sequencing summary

```
Phase 0 (gates)
   │
Phase 1 (engine fix + dedup)  ──── ships as its own PR/release
   │
Phase 2 (proto, ASK FIRST)
   │
   ├── Phase 3 (backend: draft preview, rate limit, enable gate)
   │        │
   │        └── Phase 4 (frontend: MatcherEditor, preview panel, diff dialog, wizard, git panel)
   │                 │
   └──────────────── Phase 5 (exclusions surfaced) ── depends on Phase 1's Evaluate + Phase 4's shared strings
                          │
                     Phase 6 (docs/changelog, ongoing per-phase)
```
Phase 4 task 4.7 (git-sourced panel) has no dependency beyond Phase 1 and may start in parallel with
Phase 3 if capacity allows — it touches no file Phase 3 touches.

---

## 10. Definition of done

- [ ] `internal/merge.Evaluate` is the single call site `reconcileServed`, `previewMatchedCollectors`,
      the new `PreviewMatches` handler, and `Assemble`'s exclusion computation all use.
- [ ] `reconcileServed` and `PreviewMatches`(git) bugs both have a regression test that fails on
      pre-Phase-1 `main` and passes after.
- [ ] `repocheck` guards against a `merge.CollectorLabels{}` literal recurring outside `internal/merge`.
- [ ] Draft matcher preview works for an unsaved pipeline, distinguishes "no draft sent" from "empty
      draft sent," is rate-limited, is team-scoped, and is capped with independently-reported counts.
- [ ] `EnablePipeline` rejects empty matchers for `ui`/`wizard`/`visual`; `git` and `RestoreRevision` are
      unaffected; existing enabled zero-matcher rows are untouched.
- [ ] `<MatcherEditor>` is the single implementation used by the raw editor, Visual Builder, and wizard,
      with autocomplete preserved through the extraction.
- [ ] Zero-matched-collectors messaging is byte-identical across the preview panel, the `EnablePipeline`
      error, and the exclusion panel (one shared string module).
- [ ] Diff-before-save reuses `merge.DiffMatches`/`DiffMatchesForPipeline`, recomputed server-side at
      save time.
- [ ] Every exclusion reason (unparsable matcher, role/signal mismatch, zero-matchers) reaches the
      pipeline's own page.
- [ ] Git-sourced pipelines show a dedicated read-only panel and deep-link to their repo link.
- [ ] `fleet.proto`'s `Collector.labels` comment corrected.
- [ ] §7's five open questions each have an explicit answer on record.
- [ ] Nothing has been merged to `main` without the sign-off named in Phase 0 task 0.2.

---

## 11. Independent review pass — issues found in this document (2026-09-24)

1. **Decision 5's `DiffMatchesForPipeline` helper is sketched, not designed.** Phase 2 task 2.1 defers
   its exact shape to "decide in Phase 2 review." That's acceptable for a proto-adjacent internal Go
   helper (no ask-first needed for it specifically), but whoever picks it up should not let it slip
   past Phase 2 without a concrete signature — Phase 4 task 4.5 depends on it existing.
2. **The rate limiter task (3.3) says "reuse whatever primitive already exists" without having confirmed
   one does.** If no per-org rate limiter exists anywhere in the codebase today, this becomes a new
   pattern, not just a new call site, and deserves a slightly larger review than a single task line
   implies. Worth a 15-minute grep pass at the start of Phase 3 before committing to the task as scoped.
3. **This document does not resolve open question 4 (REST backward compatibility)** — it flags it and
   defers. That's intentional (it's a product/API-contract call, not an architecture call this document
   should make unilaterally), but it means Phase 3 task 3.2 cannot fully close until someone answers it.
   Flag explicitly to whoever picks up Phase 3.
4. **RESOLVED 2026-09-24 — this "gap" was never real.** This item originally worried that Phase 1 task
   1.5's team-scoping fix should ship faster than the rest of this plan, being a pre-existing
   information-disclosure issue independent of this feature. Task 1.5's investigation found there is no
   team-to-collector ownership model in the codebase for such scoping to apply to, and org-wide fleet
   visibility for any org-reader is the codebase's consistent, confirmed-intentional design everywhere
   else. Nothing to pull out or fast-track.
