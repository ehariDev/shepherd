package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/spf13/cobra"

	"shepherd/internal/config"
	"shepherd/internal/merge"
	"shepherd/internal/store"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Administrative maintenance commands (requires DB access)",
}

var (
	auditMatcherImpactOrgID  string
	auditMatcherImpactEnable string
)

var adminAuditMatcherImpactCmd = &cobra.Command{
	Use:   "audit-matcher-impact",
	Short: "Show which pipelines would gain/lose collectors if admin labels/local_attributes were wired into matching",
	Long: `audit-matcher-impact is the rollout-gate precondition for an org's
allow_label_matching and allow_local_attribute_matching flags
(LABEL-MATCHING-PLAN.md §8): for every enabled pipeline in
the org, it diffs which collectors match under the org's current flag values
against which would match once the flags chosen with --enable are also on
(default: both). Flags already on are not reported as new effects. A pipeline
using a negative matcher (!=, !~) can only ever LOSE collectors from this
wiring, never gain any -- treat a removed collector as higher severity than
an added one, and get sign-off from that pipeline's author before flipping
either flag.`,
	RunE: runAdminAuditMatcherImpact,
}

func init() {
	adminAuditMatcherImpactCmd.Flags().StringVar(&auditMatcherImpactOrgID, "org", "", "org ID (required)")
	adminAuditMatcherImpactCmd.Flags().StringVar(&auditMatcherImpactEnable, "enable", enableBoth,
		"which flag(s) to audit turning on: labels (allow_label_matching), local-attributes (allow_local_attribute_matching) or both")
	if err := adminAuditMatcherImpactCmd.MarkFlagRequired("org"); err != nil {
		panic(err)
	} // programming error if missing
	adminCmd.AddCommand(adminAuditMatcherImpactCmd)
	rootCmd.AddCommand(adminCmd)
}

// auditCollector is one collector's identity and real admin labels/local
// attributes, as auditMatcherImpact needs them -- independent of how the
// caller sourced them (a live DB query in production, a literal slice in
// tests). LocalAttrs mirrors AdminLabels: populated from real stored data
// unconditionally, regardless of the org's current allow_label_matching /
// allow_local_attribute_matching flag values, since this tool's whole
// purpose is previewing the effect of turning a flag on before it's on.
type auditCollector struct {
	ID, Cluster, Role string
	AdminLabels       map[string]string
	LocalAttrs        map[string]string
}

// matchFlags is which of an org's two matching flags are on in one state of
// an audit: admin labels (allow_label_matching) and agent-reported local
// attributes (allow_local_attribute_matching).
type matchFlags struct {
	Labels, LocalAttrs bool
}

// Values of the --enable flag.
const (
	enableLabels     = "labels"
	enableLocalAttrs = "local-attributes"
	enableBoth       = "both"
)

// withEnabled returns f with the flag(s) named by enable turned on. Flags
// already on stay on: the audit reports only what turning more on changes.
func (f matchFlags) withEnabled(enable string) (matchFlags, error) {
	switch enable {
	case enableLabels:
		f.Labels = true
	case enableLocalAttrs:
		f.LocalAttrs = true
	case enableBoth:
		f.Labels, f.LocalAttrs = true, true
	default:
		return f, fmt.Errorf("invalid --enable %q: want %s, %s or %s", enable, enableLabels, enableLocalAttrs, enableBoth)
	}
	return f, nil
}

// labelsFor builds the collector's matchable labels as the merge engine sees
// them with the given flags on: admin labels and local attributes enter
// matching only for an org that has opted into each.
func (c auditCollector) labelsFor(f matchFlags) merge.CollectorLabels {
	var admin, local map[string]string
	if f.Labels {
		admin = c.AdminLabels
	}
	if f.LocalAttrs {
		local = c.LocalAttrs
	}
	return merge.BuildCollectorLabels(c.ID, c.Cluster, c.Role, admin, local)
}

// decodeAuditMap decodes a stored JSON object of string pairs. An empty blob
// is an empty map; ok is false for data that is present but unreadable, which
// the caller must report rather than treat as "no attributes".
func decodeAuditMap(raw []byte) (m map[string]string, ok bool) {
	if len(raw) == 0 {
		return nil, true
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	return m, true
}

// collectorRef names one collector in a pipelineImpact's Added/Removed list.
type collectorRef struct {
	ID, Cluster, Role string
}

// pipelineImpact is one enabled pipeline's full added/removed collector diff
// between the org's current matching flags and the flags being audited. A pipeline with neither is omitted by
// auditMatcherImpact -- it has zero impact and isn't worth a report line.
type pipelineImpact struct {
	PipelineID   string
	PipelineName string
	Added        []collectorRef
	Removed      []collectorRef
}

// auditMatcherImpact is §8's rollout-gate diff, reusable per-org: for every
// pipeline, it compares merge.MatchesPipeline against the collector's labels
// with the org's current flags on (current) versus with the audited flags on
// (target), and reports every collector whose match status would flip either
// direction. Admin labels and local attributes enter a state only when its
// flag is on, as in serving, so a flag already on is not reported as new and
// either flag can be audited on its own.
//
// This deliberately reuses BuildCollectorLabels/MatchesPipeline rather than
// re-implementing matcher semantics: §8's operator table shows a
// negative matcher (!=, !~) can only ever LOSE collectors when a label starts
// to exist, which is easy to get backwards by hand. Running the real matching
// function against both label sets, instead of hand-coding which operators
// can widen versus shrink, means there is no second code path to keep in sync
// with merge.MatchesPipeline as matcher support evolves.
func auditMatcherImpact(pipelines []merge.Pipeline, collectors []auditCollector, current, target matchFlags) []pipelineImpact {
	var out []pipelineImpact
	for _, p := range pipelines {
		var added, removed []collectorRef
		for _, c := range collectors {
			before := c.labelsFor(current)
			after := c.labelsFor(target)
			wasMatched, err := merge.MatchesPipeline(p, before)
			if err != nil {
				wasMatched = false
			}
			isMatched, err := merge.MatchesPipeline(p, after)
			if err != nil {
				isMatched = false
			}
			if wasMatched == isMatched {
				continue
			}
			ref := collectorRef{ID: c.ID, Cluster: c.Cluster, Role: c.Role}
			if isMatched {
				added = append(added, ref)
			} else {
				removed = append(removed, ref)
			}
		}
		if len(added) == 0 && len(removed) == 0 {
			continue
		}
		out = append(out, pipelineImpact{
			PipelineID: p.ID, PipelineName: p.Name, Added: added, Removed: removed,
		})
	}
	return out
}

func runAdminAuditMatcherImpact(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}

	var orgID pgtype.UUID
	if err := orgID.Scan(auditMatcherImpactOrgID); err != nil {
		return fmt.Errorf("invalid --org UUID: %w", err)
	}

	st, err := store.New(cmd.Context(), &cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer st.Close()

	org, err := st.Queries.GetOrgByID(cmd.Context(), orgID)
	if err != nil {
		return fmt.Errorf("loading org: %w", err)
	}
	current := matchFlags{Labels: org.AllowLabelMatching, LocalAttrs: org.AllowLocalAttributeMatching}
	target, err := current.withEnabled(auditMatcherImpactEnable)
	if err != nil {
		return err
	}
	if target == current {
		return fmt.Errorf("org %s already has %s on: nothing to audit", auditMatcherImpactOrgID, auditMatcherImpactEnable)
	}

	// Rows whose stored JSON cannot be read are counted, never silently
	// treated as empty: a collector whose attributes are unreadable might
	// match differently than this audit computes.
	var skipped []string

	pipelineRows, err := st.Queries.ListEnabledPipelinesForMerge(cmd.Context(), orgID)
	if err != nil {
		return fmt.Errorf("listing enabled pipelines: %w", err)
	}
	pipelines := make([]merge.Pipeline, 0, len(pipelineRows))
	for i := range pipelineRows {
		r := pipelineRows[i]
		var matchers []string
		if jsonErr := json.Unmarshal(r.Matchers, &matchers); jsonErr != nil {
			matchers = nil
			skipped = append(skipped, fmt.Sprintf("matchers of pipeline %q", r.Name))
		}
		repoLinkCollectorID := ""
		if r.RepoLinkCollectorID.Valid {
			repoLinkCollectorID = r.RepoLinkCollectorID.String()
		}
		pipelines = append(pipelines, merge.Pipeline{
			ID: r.ID.String(), Name: r.Name, Matchers: matchers, Source: r.Source,
			RepoLinkCollectorID: repoLinkCollectorID,
		})
	}

	collectorRows, err := st.Queries.ListCollectorsWithClusterByOrg(cmd.Context(), orgID)
	if err != nil {
		return fmt.Errorf("listing collectors: %w", err)
	}

	// Real local_attributes per collector, same "one bulk query for the
	// whole org" shape mgmtapi's localAttrsByOrg uses. Read whatever the
	// org's flag says: whether they enter matching is decided per state by
	// matchFlags, since this tool previews turning a flag on.
	localAttrRows, err := st.Queries.ListLatestLocalAttributesByOrg(cmd.Context(), orgID)
	if err != nil {
		return fmt.Errorf("listing local attributes: %w", err)
	}
	localAttrsByCollector := make(map[string]map[string]string, len(localAttrRows))
	for _, row := range localAttrRows {
		attrs, ok := decodeAuditMap(row.LocalAttributes)
		if !ok {
			skipped = append(skipped, fmt.Sprintf("local_attributes of collector %s", row.CollectorID.String()))
			continue
		}
		localAttrsByCollector[row.CollectorID.String()] = attrs
	}

	collectors := make([]auditCollector, 0, len(collectorRows))
	for i := range collectorRows {
		c := collectorRows[i]
		labels, ok := decodeAuditMap(c.Labels)
		if !ok {
			skipped = append(skipped, fmt.Sprintf("admin labels of collector %s", c.ID.String()))
		}
		collectors = append(collectors, auditCollector{
			ID: c.ID.String(), Cluster: c.ClusterName, Role: c.Role,
			AdminLabels: labels, LocalAttrs: localAttrsByCollector[c.ID.String()],
		})
	}

	impacts := auditMatcherImpact(pipelines, collectors, current, target)
	fmt.Printf("org %s: auditing %s -> %s\n", auditMatcherImpactOrgID, describeFlags(current), describeFlags(target))
	if len(skipped) > 0 {
		fmt.Fprintf(os.Stderr, "WARNING: %d stored value(s) could not be read and were left out of this audit; the result may be incomplete:\n", len(skipped))
		for _, what := range skipped {
			fmt.Fprintf(os.Stderr, "  - %s\n", what)
		}
	}
	if len(impacts) == 0 {
		if len(skipped) > 0 {
			fmt.Printf("org %s: no matcher impact among the readable data (see the warning above)\n", auditMatcherImpactOrgID)
		} else {
			fmt.Printf("org %s: no matcher impact -- every enabled pipeline's matched-collector set is unchanged\n", auditMatcherImpactOrgID)
		}
		return nil
	}

	sort.Slice(impacts, func(i, j int) bool { return impacts[i].PipelineName < impacts[j].PipelineName })
	fmt.Printf("org %s: %d pipeline(s) with matcher impact\n", auditMatcherImpactOrgID, len(impacts))
	for _, im := range impacts {
		fmt.Printf("\npipeline %q (%s)\n", im.PipelineName, im.PipelineID)
		fmt.Printf("  added   (%d): %s\n", len(im.Added), formatCollectorRefs(im.Added))
		fmt.Printf("  removed (%d): %s\n", len(im.Removed), formatCollectorRefs(im.Removed))
		if len(im.Removed) > 0 {
			fmt.Println("  ^ removed collectors are a coverage LOSS -- get sign-off from this pipeline's author before flipping allow_label_matching / allow_local_attribute_matching")
		}
	}
	return nil
}

// describeFlags renders a flag state for the audit header.
func describeFlags(f matchFlags) string {
	var on []string
	if f.Labels {
		on = append(on, "allow_label_matching")
	}
	if f.LocalAttrs {
		on = append(on, "allow_local_attribute_matching")
	}
	if len(on) == 0 {
		return "no matching flags on"
	}
	return strings.Join(on, " + ")
}

func formatCollectorRefs(refs []collectorRef) string {
	if len(refs) == 0 {
		return "(none)"
	}
	out := ""
	for i, r := range refs {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%s/%s (%s)", r.Cluster, r.Role, r.ID)
	}
	return out
}
