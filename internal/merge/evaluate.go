package merge

import (
	"errors"
	"fmt"

	"shepherd/internal/schema"
	"shepherd/internal/signals"
)

// Reason codes for EvalResult.Reason — see its doc comment.
const (
	ReasonUnparsableMatcher   = "unparsable_matcher"
	ReasonRoleSignalMismatch  = "role_signal_mismatch"
	ReasonZeroMatchers        = "zero_matchers"
	ReasonSignalsUndetermined = "signals_undetermined"
)

// EvalResult is the outcome of evaluating one pipeline against one
// collector's label set. It is the single answer every consumer that asks
// "does this pipeline reach this collector" — Assemble's exclusion
// accounting, reconcileServed's desired-state view, the matcher preview
// handler, and the wizard preview — computes from, so the answer cannot
// drift between them the way it did before (see
// docs/plans/2026-09-24-matcher-targeting-unified-plan.md §0 item 4).
type EvalResult struct {
	// Matched reports whether the pipeline's matchers (or, for a git
	// pipeline, its RepoLinkCollectorID) select this collector, before
	// role/signal enforcement.
	Matched bool
	// Served reports whether the pipeline is actually included in the
	// collector's assembled config: Matched && it survives enforcement.
	Served bool
	// Reason explains why Served is false, for the cases worth surfacing to
	// an author or an operator rather than treating as an ordinary,
	// unremarkable non-match: "unparsable_matcher", "role_signal_mismatch",
	// "zero_matchers", or "signals_undetermined". Empty when Served is true
	// or when the pipeline simply didn't match anything unremarkable (e.g.
	// its matchers are valid but the collector's labels don't satisfy them).
	Reason string
}

// Evaluate is the single decision point wrapping MatchesPipeline and the
// role/signal enforcement logic enforce.go's enforceRoles and
// rpc_reconcile.go's reconcileServed each used to reimplement separately.
// reg must be non-nil: Evaluate always enforces (gate G6) — there is no
// unenforced mode here the way Assemble's zero-option behavior allows,
// because every caller of Evaluate wants the answer that will actually be
// served, not a matcher-only superset.
//
// The returned error, when non-nil, carries the same human-readable detail
// Assemble's header comment has always shown for an exclusion (the
// underlying parse or enforcement error); EvalResult.Reason is the same
// information as a stable, machine-comparable code. A caller that only
// needs Served can ignore the error.
func Evaluate(p Pipeline, cl CollectorLabels, reg *schema.Registry) (EvalResult, error) {
	if reg == nil {
		return EvalResult{}, errors.New("merge: Evaluate requires a non-nil schema registry")
	}

	var matched bool
	switch {
	case p.Source == "git":
		matched = p.RepoLinkCollectorID == cl.CollectorID
	case len(p.Matchers) == 0:
		return EvalResult{Reason: ReasonZeroMatchers}, nil
	default:
		compiled, cerr := CompileMatchers(p.Matchers)
		if cerr != nil {
			return EvalResult{Reason: ReasonUnparsableMatcher},
				fmt.Errorf("unparsable matcher, excluded: %w", cerr)
		}
		matched = matchesCompiled(compiled, cl)
	}
	if !matched {
		return EvalResult{}, nil
	}

	sig, derr := signals.Derive(p.Contents, reg)
	if derr != nil {
		return EvalResult{Matched: true, Reason: ReasonSignalsUndetermined},
			fmt.Errorf("signal derivation failed, excluded fail-safe: %w", derr)
	}

	checkSet := sig.Combined
	var unprovenNote string
	if !sig.Proven() {
		checkSet = signals.NewSet(signals.All...)
		unprovenNote = fmt.Sprintf(" (signal set not provable: unknown components %v, unclassified wire types %v — assumed worst-case)",
			unknownComponentNames(sig.Unknown), sig.Unclassified)
	}

	if enforceErr := signals.Enforce(cl.Labels["role"], checkSet); enforceErr != nil {
		return EvalResult{Matched: true, Reason: ReasonRoleSignalMismatch},
			errors.New(commentSafe(enforceErr.Error() + unprovenNote))
	}

	return EvalResult{Matched: true, Served: true}, nil
}

// unknownComponentNames extracts just the component names from a
// []signals.UnknownComponent, for compact display in an exclusion reason.
func unknownComponentNames(u []signals.UnknownComponent) []string {
	names := make([]string, len(u))
	for i, c := range u {
		names[i] = c.Component
	}
	return names
}
