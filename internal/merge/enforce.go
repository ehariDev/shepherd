package merge

import (
	"strings"

	"shepherd/internal/schema"
)

// Exclusion records one pipeline that matched a collector's labels but was
// left out of the assembled config because its derived signals are not
// allowed for the collector's role. This is the visible record gate G6
// (docs/gateway-tier-plan.md §8 rule 2) requires: excluding the offending
// pipeline is correct (fail-safe), but the exclusion must never be silent.
type Exclusion struct {
	// PipelineName is the excluded pipeline's Name.
	PipelineName string
	// Reason is a human-readable explanation, safe to embed verbatim in a
	// "// " header comment line (never contains a raw newline — see
	// buildHeader).
	Reason string
}

// AssembleOption configures optional Assemble behavior. The zero value (no
// options) reproduces Assemble's pre-W1 behavior exactly: pipelines are
// selected by label/matcher only, with no signal/role check.
type AssembleOption func(*assembleConfig)

type assembleConfig struct {
	registry *schema.Registry
	// enforcementRequested records that WithRoleEnforcement was passed at all,
	// separately from whether it carried a usable registry. Without this, a
	// caller that asks for enforcement but hands over a nil registry gets
	// silence — the control disabled by the very call that requested it.
	enforcementRequested bool
}

// WithRoleEnforcement turns on signal/role enforcement (gate G6,
// docs/gateway-tier-plan.md W1): among the pipelines that already matched
// the collector's labels, one whose derived signal set (via signals.Derive
// against reg) is not allowed for the collector's role
// (CollectorLabels.Labels["role"]) is excluded from the assembled config
// rather than emitted into it.
//
// This is fail-safe, not fail-stop: a single bad pipeline is dropped, never
// the whole assembly. Every exclusion — the pipeline name and why — is
// recorded in the generated header comment and returned in
// AssembleResult.Exclusions, so callers can surface it instead of it being
// silently missing from a collector's config.
//
// Callers that do not pass this option keep the old, unenforced behavior.
//
// BOTH serving paths pass it: internal/mgmtapi's eager recompute and
// internal/agentapi.Service.recomputeServeCache, the lazy path taken when
// serve_cache is dirty. That was not true when this option was first written
// — the agent path was left unwired because the *schema.Registry had not been
// threaded through internal/agentapi.Service — and this comment used to say
// so. It was closed in the same session, and G6 is proven on the agent path
// specifically (internal/agentapi/service_test.go, "does not serve a metrics
// pipeline to a logs collector through the dirty-window path"), because
// enforcing one of two paths that produce the same served config is not
// enforcement.
//
// The stale wording survived until a review caught it, which is worth a note
// of its own: a comment that DENIES a control now wired misleads exactly as
// badly as one claiming a control that is not.
// Passing a nil registry is a wiring error, not a way to opt out: Assemble
// fails loudly rather than serving unenforced config that looks enforced. To
// genuinely opt out, do not pass the option.
func WithRoleEnforcement(reg *schema.Registry) AssembleOption {
	return func(c *assembleConfig) {
		c.registry = reg
		c.enforcementRequested = true
	}
}

// commentSafe collapses any embedded line break to a space. Everything the
// header interpolates — exclusion reasons, pipeline names, the collector
// display name — is written verbatim into a "// " comment line of the
// generated Alloy config; a raw newline would end the comment and turn the
// remainder into syntax, which Stage 1 then rejects for the whole assembled
// output. Names come from the database and the API only requires them to be
// non-empty, so the header is the last line of defence.
func commentSafe(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}
