// Package merge implements the pipeline matching and declare-wrap merge engine.
//
// Each enabled pipeline's contents are wrapped in a declare block and instantiated,
// namespacing its components to prevent collisions across pipelines.
package merge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/alertmanager/pkg/labels"
)

// Pipeline is the minimal representation of a pipeline needed by the merge engine.
type Pipeline struct {
	// ID is the pipeline UUID string.
	ID string
	// Name is the human-readable name used for block naming.
	Name string
	// Contents is the raw Alloy syntax content.
	Contents string
	// Matchers is the list of Alertmanager matcher strings (all must match — AND).
	// Empty means match nothing (safety default).
	Matchers []string
	// Source is "ui", "wizard", or "git".
	Source string
	// Revision is the current revision number.
	Revision int
	// RepoLinkCollectorID is set (non-empty) when Source == "git".
	// Git pipelines match only their target collector, not via matchers.
	RepoLinkCollectorID string
}

// CollectorLabels represents the label set used to match pipelines to a
// collector. It includes the built-in labels (cluster, role) plus whatever
// adminLabels and localAttrs BuildCollectorLabels was given — this type
// itself has no opinion on how a caller computed localAttrs (e.g. whether it
// reflects one instance or several); that's BuildCollectorLabels' caller's
// responsibility, not this merge step's.
type CollectorLabels struct {
	CollectorID string
	Labels      map[string]string
}

// BuildCollectorLabels merges the built-in cluster/role labels with a
// collector's admin-set "Manage labels" (adminLabels) and agent-reported
// local_attributes (localAttrs), for callers gated on the org's
// allow_label_matching / allow_local_attribute_matching flags respectively
// (procoduck/shepherd#139). A caller not opted into one of the two sources
// must pass nil/empty for it — this function applies no gating itself, so it
// always merges whatever it is given.
//
// Precedence, low to high: localAttrs < adminLabels < {cluster, role}.
// localAttrs is agent-reported — i.e. reachable via a compromised agent
// token — and must never be allowed to shadow an admin-set label, mirroring
// why Fleet Management makes remote-reported data lose to admin-set data.
// localAttrs keys are lowercased before merging: admin labels are already
// forced lowercase at write time (SetCollectorLabel), so an unnormalized
// agent-reported "Team" would otherwise silently fail to match a
// `team="..."` matcher written in the admin-label convention.
//
// Any localAttrs or adminLabels key IsReserved rejects is dropped here even
// though the write paths already refuse one (defense in depth against a row
// that predates a key becoming reserved — see docs' "retroactive collision"
// known issue), and cluster/role are set after both filters so they can
// never be shadowed regardless of iteration order.
func BuildCollectorLabels(collectorID, cluster, role string, adminLabels, localAttrs map[string]string) CollectorLabels {
	labels := make(map[string]string, len(adminLabels)+len(localAttrs)+2)
	for k, v := range localAttrs {
		k = strings.ToLower(k)
		if IsReserved(k) {
			continue
		}
		labels[k] = v
	}
	for k, v := range adminLabels {
		if IsReserved(k) {
			continue
		}
		labels[k] = v
	}
	labels["cluster"] = cluster
	labels["role"] = role
	return CollectorLabels{CollectorID: collectorID, Labels: labels}
}

// sanitizeRe matches characters outside [a-z0-9_].
var sanitizeRe = regexp.MustCompile(`[^a-z0-9_]`)

// SanitizeName converts a pipeline name to a valid Alloy identifier.
// Result is lowercase, replaces non-[a-z0-9_] with underscores, and
// prepends "p" if the first character would be a digit.
func SanitizeName(name string) string {
	r := sanitizeRe.ReplaceAllString(strings.ToLower(name), "_")
	if len(r) == 0 || (r[0] >= '0' && r[0] <= '9') {
		r = "p" + r
	}
	return r
}

// CompileMatchers parses each raw Alertmanager-style matcher string, returning
// the compiled matchers or the first parse error. The error is wrapped
// exactly as validateSaveInput's historical inline loop did
// (internal/mgmtapi/rpc_pipeline.go), so callers mapping this error to
// CodeInvalidArgument see unchanged text.
func CompileMatchers(raw []string) ([]*labels.Matcher, error) {
	compiled := make([]*labels.Matcher, 0, len(raw))
	for _, ms := range raw {
		m, err := labels.ParseMatcher(ms)
		if err != nil {
			return nil, fmt.Errorf("matcher %q is not valid: %w", ms, err)
		}
		compiled = append(compiled, m)
	}
	return compiled, nil
}

// matchesCompiled reports whether every pre-compiled matcher matches cl (AND).
func matchesCompiled(matchers []*labels.Matcher, cl CollectorLabels) bool {
	for _, m := range matchers {
		v := cl.Labels[m.Name]
		if !m.Matches(v) {
			return false
		}
	}
	return true
}

// MatchesPipeline reports whether the pipeline matches the collector label set.
// Git pipelines are matched by collector ID, not by label matchers.
// UI/wizard pipelines with zero matchers match nothing.
func MatchesPipeline(p Pipeline, cl CollectorLabels) (bool, error) {
	if p.Source == "git" {
		return p.RepoLinkCollectorID == cl.CollectorID, nil
	}
	if len(p.Matchers) == 0 {
		return false, nil
	}
	compiled, err := CompileMatchers(p.Matchers)
	if err != nil {
		return false, err
	}
	return matchesCompiled(compiled, cl), nil
}

// AssembleResult is the output of the merge engine for a single collector.
type AssembleResult struct {
	Content string
	Hash    string
	// Exclusions lists every pipeline that matched cl's labels but was left
	// out of Content because of role enforcement (see WithRoleEnforcement).
	// Empty when enforcement was not requested or excluded nothing.
	Exclusions []Exclusion
}

// Assemble builds the merged Alloy configuration for a collector.
//
// It selects matching pipelines (git first, then ui/wizard sorted by name),
// optionally drops any whose signals the collector's role does not allow
// (WithRoleEnforcement — gate G6), wraps each survivor in a declare block,
// and prepends a header comment naming both what was included and what was
// excluded and why. Returns (content, hash, exclusions) ready to be stored
// in serve_cache.
func Assemble(collectorID, collectorDisplayName string, cl CollectorLabels, pipelines []Pipeline, version, generatedAt string, opts ...AssembleOption) (AssembleResult, error) {
	var cfg assembleConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.enforcementRequested && cfg.registry == nil {
		return AssembleResult{}, errors.New(
			"merge: role enforcement was requested but the schema registry is nil — " +
				"refusing to serve unenforced config that would look enforced")
	}

	var selected []Pipeline
	var exclusions []Exclusion

	if cfg.registry != nil {
		// Enforced path: Evaluate is the single decision point (match, then
		// derive signals, then enforce role) that reconcileServed and the
		// matcher preview handler also call — see
		// docs/plans/2026-09-24-matcher-targeting-unified-plan.md §0 item 4 —
		// so this can no longer drift from them the way the old two-pass
		// select-then-enforceRoles split once did.
		for _, p := range pipelines {
			result, evalErr := Evaluate(p, cl, cfg.registry)
			switch {
			case result.Served:
				selected = append(selected, p)
			case evalErr != nil:
				// Matched-but-excluded (role/signal mismatch, signal
				// derivation failure) or unparsable-matcher: worth a header
				// line. An ordinary non-match or a zero-matcher pipeline
				// (evalErr == nil) needs none — same as before this change.
				exclusions = append(exclusions, Exclusion{PipelineName: p.Name, Reason: evalErr.Error()})
			}
		}
	} else {
		// Unenforced: matcher/collector-ID selection only, no signal/role
		// check (Assemble's pre-W1 behavior). Still relied on by gitsync's
		// dry-run validation, which has no schema registry to enforce with.
		for _, p := range pipelines {
			if p.Source == "git" {
				if p.RepoLinkCollectorID == cl.CollectorID {
					selected = append(selected, p)
				}
				continue
			}
			matched, err := MatchesPipeline(p, cl)
			if err != nil {
				// One unparsable matcher used to abort the whole assembly, which
				// meant a single bad pipeline froze config serving for every
				// collector in the org -- and, for a collector with no cache row
				// yet, produced an EMPTY served config that wiped what it was
				// already running. Exclude just the offender and say so in the
				// header, the same way role enforcement reports its exclusions.
				// Matchers are also parsed at save now, so reaching this means the
				// row predates that check or was written outside the API.
				exclusions = append(exclusions, Exclusion{
					PipelineName: p.Name,
					Reason:       fmt.Sprintf("unparsable matcher, excluded: %v", err),
				})
				continue
			}
			if matched {
				selected = append(selected, p)
			}
		}
	}

	// Stable sort: selection order above mixes git and ui/wizard pipelines in
	// input order — re-sort all by name for full determinism.
	slices.SortStableFunc(selected, func(a, b Pipeline) int {
		return strings.Compare(a.Name, b.Name)
	})

	if len(selected) == 0 {
		// Empty config (nothing matched, or role enforcement excluded everything that did).
		content := buildHeader(collectorID, collectorDisplayName, version, generatedAt, nil, exclusions)
		return AssembleResult{Content: content, Hash: HashContent(content), Exclusions: exclusions}, nil
	}

	var sb strings.Builder

	// Header comment.
	sb.WriteString(buildHeader(collectorID, collectorDisplayName, version, generatedAt, selected, exclusions))

	// Check for sanitized-name collisions before assembling.
	seen := make(map[string]string, len(selected)) // blockName → pipeline name
	for _, p := range selected {
		blockName := "pipe_" + SanitizeName(p.Name)
		if existing, ok := seen[blockName]; ok {
			return AssembleResult{}, fmt.Errorf(
				"declare-name collision: pipelines %q and %q both sanitize to block name %q",
				existing, p.Name, blockName,
			)
		}
		seen[blockName] = p.Name
	}

	// Declare-wrapped blocks.
	for i, p := range selected {
		if i > 0 {
			sb.WriteString("\n")
		}
		blockName := "pipe_" + SanitizeName(p.Name)
		fmt.Fprintf(&sb, "declare %q {\n", blockName)
		// Indent pipeline contents by one level.
		for _, line := range strings.Split(p.Contents, "\n") {
			if line == "" {
				sb.WriteString("\n")
			} else {
				sb.WriteString("  " + line + "\n")
			}
		}
		sb.WriteString("}\n")
		fmt.Fprintf(&sb, "%s \"default\" { }\n", blockName)
	}

	content := sb.String()
	return AssembleResult{Content: content, Hash: HashContent(content), Exclusions: exclusions}, nil
}

// HashContent computes hex(sha256(content)).
func HashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// buildHeader returns the header comment for a merged config. exclusions
// (from role enforcement — see WithRoleEnforcement) are always listed when
// present, even when pipelines is otherwise empty, so an excluded pipeline
// is never invisible just because nothing else matched.
func buildHeader(collectorID, displayName, version, generatedAt string, pipelines []Pipeline, exclusions []Exclusion) string {
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "// Generated by Shepherd %s at %s\n", version, generatedAt)
	// Every interpolated string goes through commentSafe: a line break in a
	// name would end the comment and break the assembled config (see the
	// "header comment hardening" spec).
	fmt.Fprintf(&sb, "// Collector: %s (%s)\n", commentSafe(displayName), commentSafe(collectorID))
	if len(pipelines) == 0 {
		sb.WriteString("// No pipelines matched\n")
	} else {
		fmt.Fprintf(&sb, "// Pipelines (%d):\n", len(pipelines))
		for _, p := range pipelines {
			fmt.Fprintf(&sb, "//   - %s (rev %d)\n", commentSafe(p.Name), p.Revision)
		}
	}
	if len(exclusions) > 0 {
		fmt.Fprintf(&sb, "// Excluded (%d) - signal/role mismatch (docs/gateway-tier-plan.md W1, gate G6):\n", len(exclusions))
		for _, e := range exclusions {
			fmt.Fprintf(&sb, "//   - %s: %s\n", commentSafe(e.PipelineName), commentSafe(e.Reason))
		}
	}
	sb.WriteString("\n")
	return sb.String()
}
