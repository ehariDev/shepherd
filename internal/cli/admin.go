package cli

import (
	"encoding/json"
	"fmt"
	"sort"

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

var auditMatcherImpactOrgID string

var adminAuditMatcherImpactCmd = &cobra.Command{
	Use:   "audit-matcher-impact",
	Short: "Show which pipelines would gain/lose collectors if admin labels/local_attributes were wired into matching",
	Long: `audit-matcher-impact is the rollout-gate precondition for an org's
allow_label_matching and allow_local_attribute_matching flags
(LABEL-MATCHING-PLAN.md §8, PR-144 review §9): for every enabled pipeline in
the org, it diffs which collectors match today (neither wired, exactly
current behavior) against which would match once both are on. A pipeline
using a negative matcher (!=, !~) can only ever LOSE collectors from this
wiring, never gain any -- treat a removed collector as higher severity than
an added one, and get sign-off from that pipeline's author before flipping
either flag.`,
	RunE: runAdminAuditMatcherImpact,
}

func init() {
	adminAuditMatcherImpactCmd.Flags().StringVar(&auditMatcherImpactOrgID, "org", "", "org ID (required)")
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
// purpose is previewing the effect of turning a flag on before it's on
// (PR-144 review §9).
type auditCollector struct {
	ID, Cluster, Role string
	AdminLabels       map[string]string
	LocalAttrs        map[string]string
}

// collectorRef names one collector in a pipelineImpact's Added/Removed list.
type collectorRef struct {
	ID, Cluster, Role string
}

// pipelineImpact is one enabled pipeline's full added/removed collector diff
// between admin labels unwired (today) and wired (after the org's
// allow_label_matching flag goes on). A pipeline with neither is omitted by
// auditMatcherImpact -- it has zero impact and isn't worth a report line.
type pipelineImpact struct {
	PipelineID   string
	PipelineName string
	Added        []collectorRef
	Removed      []collectorRef
}

// auditMatcherImpact is §8's rollout-gate diff, reusable per-org: for every
// pipeline, it compares merge.MatchesPipeline against
// merge.BuildCollectorLabels(id, cluster, role, nil, nil) (today's behavior,
// neither admin labels nor local_attributes enter matching) versus
// merge.BuildCollectorLabels(id, cluster, role, c.AdminLabels, c.LocalAttrs)
// (the post-wiring behavior, both flags on) for every collector, and reports
// every collector whose match status would flip either direction.
//
// PR-144 review §9: extended to also diff local_attributes rather than
// building a second tool, reusing the same BuildCollectorLabels call with
// real vs. nil localAttrs the admin-label side already used -- "no second
// code path to maintain."
//
// This deliberately reuses BuildCollectorLabels/MatchesPipeline rather than
// re-implementing matcher semantics: §8's operator table shows a
// "matched count went from zero to nonzero" check misses every !=/!~
// pipeline (already matching everyone today, on "" != "value"), which can
// only ever SHRINK once real values are wired in -- the dangerous direction,
// since it regresses something currently working rather than activating
// something currently inert. Running the real matching function against
// both label sets, instead of hand-coding which operators can widen versus
// shrink, means there is no second code path to keep in sync with
// merge.MatchesPipeline as matcher support evolves.
func auditMatcherImpact(pipelines []merge.Pipeline, collectors []auditCollector) []pipelineImpact {
	var out []pipelineImpact
	for _, p := range pipelines {
		var added, removed []collectorRef
		for _, c := range collectors {
			before := merge.BuildCollectorLabels(c.ID, c.Cluster, c.Role, nil, nil)
			after := merge.BuildCollectorLabels(c.ID, c.Cluster, c.Role, c.AdminLabels, c.LocalAttrs)
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
	// whole org" shape mgmtapi's localAttrsByOrg uses -- not gated on
	// org.AllowLocalAttributeMatching, since (like AdminLabels above) this
	// tool's purpose is previewing the effect of turning that flag on.
	localAttrRows, err := st.Queries.ListLatestLocalAttributesByOrg(cmd.Context(), orgID)
	if err != nil {
		return fmt.Errorf("listing local attributes: %w", err)
	}
	localAttrsByCollector := make(map[string]map[string]string, len(localAttrRows))
	for _, row := range localAttrRows {
		var attrs map[string]string
		if jsonErr := json.Unmarshal(row.LocalAttributes, &attrs); jsonErr != nil {
			continue
		}
		localAttrsByCollector[row.CollectorID.String()] = attrs
	}

	collectors := make([]auditCollector, 0, len(collectorRows))
	for i := range collectorRows {
		c := collectorRows[i]
		var labels map[string]string
		if jsonErr := json.Unmarshal(c.Labels, &labels); jsonErr != nil {
			labels = nil
		}
		collectors = append(collectors, auditCollector{
			ID: c.ID.String(), Cluster: c.ClusterName, Role: c.Role,
			AdminLabels: labels, LocalAttrs: localAttrsByCollector[c.ID.String()],
		})
	}

	impacts := auditMatcherImpact(pipelines, collectors)
	if len(impacts) == 0 {
		fmt.Printf("org %s: no matcher impact -- every enabled pipeline's matched-collector set is unchanged once admin labels and local_attributes are wired in\n", auditMatcherImpactOrgID)
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
