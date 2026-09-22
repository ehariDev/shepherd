---
name: shepherd-pipeline-author
description: Author, validate, and ship Shepherd pipelines (Alloy config fragments + matchers) against a real Shepherd deployment, for Linux, Windows, Kubernetes, and OpenShift targets carrying metrics, logs, and/or traces. Use whenever asked to create/edit a Shepherd pipeline, add or change a matcher, add/inspect a collector "admin label", debug why a pipeline isn't reaching a collector, or pick which Alloy component to use for a given host/platform/signal. Bakes in Shepherd's label/matcher validation rules and a live-verified component reference so mistakes are caught before they hit the API.
---

# Authoring Shepherd pipelines

A **pipeline** is an Alloy config fragment plus a list of **matchers** that decide
which collectors receive it. Shepherd merges every enabled pipeline whose matchers
select a given collector into the one config that collector is served, on its next
`remotecfg` poll. This skill exists to catch the mistakes that otherwise only surface
after a live poll cycle: bad matcher syntax, label keys that look valid but aren't
matcher-safe, reserved keys, and org flags that silently make a matcher a no-op.

Everything below was verified against a real deployment (ehariDev/shepherd, PR #11 +
#12 merged locally) — not just read from source. Where line numbers are given they're
a starting point for re-checking against whatever checkout you're pointed at, not a
promise the code hasn't moved.

## 0. Before writing anything: know your label sources

A collector is matched against a synthesized label set with **three possible
sources**, merged with this precedence (low → high):

```
local_attributes  <  admin labels ("Manage labels")  <  {cluster, role}
```

| Source | Set by | Always active? | Reserved-key enforcement |
|---|---|---|---|
| `cluster`, `role` | Shepherd (built-in, from the collector's registration) | Always | N/A — these *are* the reserved keys |
| Admin labels (`collectors.labels`) | A human, via `SetCollectorLabel`/UI "Manage labels" | Only if org's `allow_label_matching = true` | Enforced at write time (rejected immediately) |
| `local_attributes` | The Alloy agent itself, via its own `remotecfg { attributes = {...} }` block | Only if org's `allow_local_attribute_matching = true` | Reserved keys are still stored (so `collector.version` etc. show in the UI) but silently dropped when building the *matched-against* label set |

**Check both org flags before debugging "my matcher doesn't match anything."** A
matcher on a non-`cluster`/`role` key is silently inert — not an error — if the
relevant flag is off. `GET /api/admin/orgs` returns `allow_label_matching` /
`allow_local_attribute_matching` per org.

`local_attributes` is agent-reported and reachable by anyone holding that
collector's agent token — that's *why* it's below admin labels in precedence and
gated by a separate flag. Never assume a `local_attributes`-sourced value is
trustworthy the way an admin label is.

**Known simplification, not a bug:** for a collector with more than one live
instance (an HA pair, or a rolling upgrade briefly running two), `local_attributes`
matching uses the single most-recently-seen instance's whole attribute blob, not a
true per-key union. Divergent instances can make served config flap by poll timing.
Don't design a matcher that depends on per-key union semantics across instances.

## 1. Validate label keys/values BEFORE calling the API

Run `scripts/validate_label.py <key> <value>` (no network needed — pure rule
check, mirrors `internal/mgmtapi/rpc_fleet.go`'s `validCollectorLabelKey`/
`validCollectorLabelValue` and `internal/merge/reserved.go`'s `IsReserved`
byte-for-byte):

- **Key:** 1–128 bytes, lowercase only: `a-z`, `0-9`, `.`, `_`, `-`, `/`. Uppercase is
  rejected outright (not silently lowercased) when set through the admin-label API —
  only agent-reported `local_attributes` get auto-lowercased.
- **Value:** 1–512 bytes, no control characters, no Unicode "format" characters
  (category `Cf` — e.g. zero-width joiners). Empty value is rejected.
- **Reserved keys — cannot be admin labels or effective matcher sources, full stop:**
  - Exact: `cluster`, `role`, `id`, `os`, `alloy_version`
  - Prefix: `collector.*`, `shepherd.*`
  - These are reserved because they're either existing built-in match labels
    (`cluster`/`role`), mirror a real `collector_instances` column that's a likely
    future built-in (`os`, `alloy_version`), mirror Grafana Cloud Fleet Management's
    own reservation (`id`), or namespace future Shepherd-native derived concepts
    (`collector.*`/`shepherd.*`).
- **Cap:** at most 64 labels per collector (both API-enforced and DB-enforced).

## 2. The matcher gotcha: matcher syntax is STRICTER than label-key syntax

**This is the mistake this skill exists to catch.** Pipeline matchers use
Prometheus Alertmanager matcher syntax (`github.com/prometheus/alertmanager/pkg/labels.ParseMatcher`),
e.g. `cluster="prod-eu-1"`, `role!="logs"`, `team=~"payments|billing"`.

The Alertmanager grammar's label-name token only allows `[a-zA-Z_][a-zA-Z0-9_]*` —
**no dots, hyphens, or slashes**. But an admin label KEY is allowed to contain
`.`, `-`, and `/`. That means:

```
demo.arch-review/dir-a  ← valid admin label key, SetCollectorLabel accepts it
demo.arch-review/dir-a="x"  ← INVALID matcher — ParseMatcher rejects it
```

Verified live: setting this label succeeds; using it in a pipeline's `matchers`
array fails at pipeline save with `"matcher ... is not valid: bad matcher format"`.

**Rule of thumb:** if you intend a label to ever be used as a matcher, restrict its
key to `[a-z_][a-z0-9_]*` from the start — i.e. don't use `.`, `-`, `/` even though
the label API alone would accept them. Run `scripts/validate_label.py --matcher-safe
<key>` to check this before setting the label.

## 3. Matcher semantics to get right

- **All matchers on a pipeline are ANDed.** There is no OR across matcher list
  entries — use `=~` with a regex alternation (`role=~"metrics|logs"`) instead of
  two separate matchers if you want OR.
- **Zero matchers = matches nothing.** This is Shepherd's deliberate safety default,
  not a bug — a freshly created pipeline with no matchers set is inert, not
  "matches everyone."
- **A missing key reads as empty string**, not "absent." A matcher like `env!="dev"`
  MATCHES a collector that has no `env` label/attribute at all (empty != "dev" is
  true). If you want "only collectors that explicitly declare env", you cannot
  express that with `!=` alone — this is a real footgun for negative matchers
  combined with an attribute that isn't universally reported.
- **`git`-sourced pipelines ignore matchers entirely** — they're matched by
  collector ID (repo link), not labels. Don't add matchers to a git pipeline
  expecting them to do anything.

## 4. The three-stage validation gate — use it, don't bypass it

Every pipeline, regardless of which editor produced it (raw/UI, wizard, visual
builder), goes through the same three stages before it can reach a collector:

1. **Syntax** — the Alloy parser (errors map to line/column).
2. **Semantics** — the real `alloy validate` binary against the pinned Alloy version
   (catches e.g. a `prometheus.scrape` missing a required argument).
3. **Merge dry-run** — the whole merged config for every collector the pipeline
   would affect, run only at **create/update of an already-enabled pipeline** and at
   **enable time**. This is the stage that catches a pipeline that's individually
   valid but collides (declare-name collision, etc.) with one already running.

**Always call `POST /api/orgs/{org}/pipelines/validate` with your draft `contents`
before creating the pipeline.** It runs stages 1+2 for free, with no side effects,
and returns `{"valid": bool, "diagnostics": [...], "signals": [...],
"signals_proven": bool, "unknown_components": [...]}`. Fix every diagnostic before
proceeding — stage 3 (merge dry-run) still runs at create/enable and WILL reject a
pipeline that passes validate but collides with another enabled pipeline's
sanitized block name (`pipe_<sanitized-name>` — see naming below).

## 5. Naming and content rules

- Pipeline name → Alloy block name via `SanitizeName`: lowercased, every char
  outside `[a-z0-9_]` becomes `_`, and a leading digit gets a `p` prefix. Two
  pipeline names that sanitize to the same block name in the same org is a hard
  create/enable-time error (`declare-name collision`) — don't reuse near-identical
  names differing only in punctuation.
- `source` is one of `ui`, `wizard`, `git` — omit it to default to `ui`.
- Pipeline `matchers` is a JSON array of strings, not a single string (the DB
  column is literally named `matchers`, plural — a `check-state.sh`-style helper
  script that queries a column named `matcher` is querying a column that doesn't
  exist).

## 6. End-to-end workflow (what this skill actually walks you through)

1. **Draft** the Alloy `contents`. **See §7 for which component to reach for** by
   target (Linux/Windows/Kubernetes/OpenShift) and signal (metrics/logs/traces) —
   don't rediscover this by grepping the schema artifact yourself. If it needs to
   attach a custom label to
   telemetry data itself (as opposed to just deciding routing), that's a normal
   Alloy relabel/static-label rule in the content — e.g.
   `prometheus.remote_write { external_labels = { team = "payments" } }` or
   `loki.process { stage.static_labels { values = { team = "payments" } } }`.
   Shepherd's admin labels/local_attributes are a **routing** mechanism (which
   pipeline reaches which collector); they are never auto-injected onto the
   telemetry stream itself — if you want a label on the actual metrics/logs, the
   pipeline content has to add it explicitly.
2. **Validate**: `POST /api/orgs/{org}/pipelines/validate {"name","contents"}`.
   Fix diagnostics; repeat until `"valid": true`.
3. **Check matcher safety** for any non-cluster/role key referenced in your
   intended matchers: run `scripts/validate_label.py --matcher-safe <key>`.
4. **Create**: `POST /api/orgs/{org}/pipelines {"name","contents","matchers":[...],"source"}`.
   Starts `enabled: false`.
5. **Preview**: `GET /api/orgs/{org}/pipelines/{id}/preview-matches` — confirms
   which collectors would receive it, *before* it goes live. Empty result when you
   expected matches usually means either a typo'd matcher value, or the relevant
   org flag (`allow_label_matching`/`allow_local_attribute_matching`) is off.
6. **Enable**: `POST /api/orgs/{org}/pipelines/{id}/enable`. This is where stage 3
   (merge dry-run) actually runs for a brand-new pipeline.
7. **Verify on a real collector**: `GET /api/orgs/{org}/collectors/{id}/served-config`
   — this is a pure cache read, NOT a recompute. A label/attribute change marks the
   cache dirty but the served content only updates on that collector's next
   `remotecfg` poll (whatever `poll_frequency` its Alloy config uses — commonly
   10-60s in these lab configs). Don't conclude "it didn't work" from a served-config
   read taken before one full poll interval has passed.
8. **Confirm data actually arrived, not just config delivery** — see §7a. This is a
   separate step from #7: a pipeline can be served correctly and still produce zero
   data (wrong scrape target, auth failure to the backend, SCC/RBAC denial on
   OpenShift, etc.).

## 7. Component reference by target/signal (Linux, Windows, Kubernetes, OpenShift)

**Tested live 2026-09-22**: created a real `prometheus.exporter.unix` pipeline through this
exact workflow, end to end — validate → create → enable → preview-matches → poll → confirmed
4,502 samples actually landed in Mimir via the agent's own
`prometheus_remote_storage_samples_total{component_path="/remotecfg/pipe_<name>.default"}`
counter (see §7a — this is the technique to use, not just a served-config check). The gap this
section fixes: the workflow above tells you *how* to ship a pipeline, but not *which component*
to reach for. Before this section existed, the honest answer was "go grep
`internal/schema/artifacts/alloy-v*.json` yourself" — do that if a component below turns out
stale, but you shouldn't have to for the common cases.

| Target | Metrics | Logs | Notes |
|---|---|---|---|
| **Linux host** | `prometheus.exporter.unix` (arg: `enable_collectors = [...]`, e.g. `cpu`, `meminfo`, `diskstats`, `netdev`, `filesystem`) | `loki.source.journal` (systemd) or `local.file_match` + `loki.source.file` (arbitrary log files) | Confirmed live. |
| **Windows host** | `prometheus.exporter.windows` (arg: **`enabled_collectors`** — note the `d`, different from Unix's `enable_collectors`; this is a real copy-paste trap between the two) | `loki.source.windowsevent` | Both confirmed present in the pinned schema (`alloy-v1.18.1.json`, `alloy-v1.19.2.json`); not live-tested this session (no Windows host in this lab) — validate before trusting blindly. |
| **Kubernetes / OpenShift pods** | `discovery.kubernetes` (`role = "pod"` \| `"service"` \| `"endpoints"` \| `"node"`) → `discovery.relabel` → `prometheus.scrape` | `discovery.kubernetes` (`role="pod"`) → `loki.source.kubernetes` (takes `targets`/`forward_to` directly) | Same components for OpenShift — it's a standard Kubernetes API underneath. Leave `api_server`/`bearer_token_file`/`kubeconfig_file` **unset** to use the pod's own in-cluster ServiceAccount token/CA automatically, on either distro. |
| **Kubernetes / OpenShift node-level (kubelet/cAdvisor)** | `discovery.kubelet` (default `url = "https://localhost:10250"`, per-node via DaemonSet) | — | Same on OpenShift, but see the OpenShift caveat below. |
| **Kubernetes / OpenShift events** | — | `loki.source.kubernetes_events` | Cluster-scoped, not per-pod. |
| **Traces (any platform)** | `otelcol.receiver.otlp` → `otelcol.processor.batch` → `otelcol.exporter.otlp`/`otelcol.exporter.otlphttp` | (same components carry logs too — OTLP is multi-signal) | Confirmed live 2026-09-21: pushed a real OTLP span through a Shepherd-managed pipeline, retrieved it back from Tempo by trace ID. **`otelcol.receiver.otlp` derives as `signals: [metrics, logs, traces]` regardless of which output you wire** — see the role-enforcement gotcha below. |
| **Kubernetes/OpenShift trace enrichment** | `otelcol.processor.k8sattributes` (adds pod/namespace/node metadata to spans via the k8s API) | — | Slot it between the OTLP receiver and exporter; same in-cluster-auth story as `discovery.kubernetes`. |

**The OpenShift-specific thing that isn't a pipeline problem:** `discovery.kubelet` and any
DaemonSet-based node exporter typically need `hostNetwork`/`hostPath`/privileged-ish access.
Vanilla Kubernetes RBAC often allows this by default; OpenShift's SecurityContextConstraints
(SCC) do not, and refusing it is a **cluster-admin action on the OpenShift side**, not a Shepherd
or Alloy config problem — no pipeline content or matcher fixes a missing SCC grant. If a
`discovery.kubelet`-based pipeline validates clean, enables, previews matching the right
collectors, but the collector's `remote_config_status` still shows no data reaching Mimir, an
SCC/RBAC denial on the OpenShift side is the first thing to rule out, before re-debugging the
pipeline itself.

**The role-enforcement trap that bites traces (and any OTLP pipeline) specifically:**
`otelcol.receiver.otlp` is derived as carrying all three of `{metrics, logs, traces}` — Shepherd
has no way to know you only wired the `traces` output — and role enforcement
(`internal/signals/policy.go`) only allows that full set on `role="receiver"` or unrestricted
`role="singleton"`. A collector deployed as `role="metrics"` or `role="logs"` (the natural-looking
choice for a fleet rollout) will have any OTLP-receiver pipeline **silently excluded** — visible
only in the served config's `// Excluded (N) - signal/role mismatch` header, not a hard error
anywhere else. **If OTLP ingestion (traces or otherwise) is in scope for a fleet, that fleet's
`role` attribute — set once, at agent-deploy time, in `remotecfg.attributes` — must be
`receiver` or `singleton` from day one.** This is usually not fixable after the fact without
redeploying the agent config.

## 7a. Definitive verification: did the DATA actually arrive, not just the config

§6 step 7 (`served-config`) only proves the collector *received* the pipeline — not that it's
successfully producing/shipping data. Two pipelines from different collectors can produce
identically-named metrics (e.g. every `prometheus.exporter.unix` pipeline emits
`node_uname_info`), so querying Mimir/Loki for the metric name alone doesn't prove *your*
pipeline is the source — confirmed this ambiguity live when three near-identical
`node_uname_info` series showed up from different pipelines with no obvious way to tell them
apart by label alone.

**The unambiguous check:** every Alloy agent exposes its own self-metrics (commonly on the
agent's local HTTP port — `12346`/`18888`/etc. depending on the lab's `--server.http.listen-addr`,
check the collector's own config). Query it directly for a component-path-scoped counter that
bakes in your pipeline's sanitized name:

```
# Metrics pipelines:
curl http://<agent-local-addr>/metrics | grep 'prometheus_remote_storage_samples_total.*pipe_<your_pipeline_name>'

# Any component, generically:
curl http://<agent-local-addr>/metrics | grep 'component_path="/remotecfg/pipe_<your_pipeline_name>.default"'
```

A nonzero, climbing sample/line count here is definitive — it's scoped to your exact pipeline's
component graph, not a shared metric/log-stream name that other pipelines might also produce.
Only fall back to querying Mimir/Loki/Tempo directly (as in §6) when you need to confirm the
*receiving* backend also accepted it (e.g. after a `partialSuccess` OTLP response, or to check
for backend-side rejections that never surface in the agent's own send-success counters).

## 8. Auth note for testing against a dev/lab instance

Connect-RPC endpoints (e.g. `SetCollectorLabel`, which has **no REST shim** — it's
Connect-only) need `Content-Type: application/json`, `Connect-Protocol-Version: 1`,
and, like every mutating call, `X-Requested-With: XMLHttpRequest` (CSRF check) plus
a valid session cookie. Path shape: `POST /shepherd.mgmt.v1.<Service>/<Method>`
with the request message as a JSON body. For a lab/dev box (never production), the
binary's own `shepherd dev create-session --persona appadmin` mints a session
without touching the real bootstrap-admin credential.

## 9. Reference: reserved keys (copy-paste safe)

```
cluster, role, id, os, alloy_version        # exact match, case-sensitive-lowercase
collector.*, shepherd.*                      # prefix match
```
