package cli

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"shepherd/internal/merge"
)

// LABEL-MATCHING-PLAN.md §8's operator table: a "=" or "=~" matcher only
// ever WIDENS once admin labels are wired in (it matches nothing today,
// since an unwired key reads as ""), while a "!=" or "!~" matcher already
// matches everyone today and can only ever SHRINK. auditMatcherImpact must
// report both directions correctly for all four operators -- a tool that
// only checked "matched count went up" would miss every shrink case, which
// is the more dangerous one (a regression on something already live).
var (
	flagsOff  = matchFlags{}
	flagsBoth = matchFlags{Labels: true, LocalAttrs: true}
)

var _ = Describe("auditMatcherImpact", func() {
	collector := func(id, cluster, role string, labels map[string]string) auditCollector {
		return auditCollector{ID: id, Cluster: cluster, Role: role, AdminLabels: labels}
	}

	It("reports a widen (added) for an '=' matcher once the admin label is set", func() {
		pipelines := []merge.Pipeline{
			{ID: "p1", Name: "equals-widen", Matchers: []string{`team="platform"`}, Source: "ui"},
		}
		collectors := []auditCollector{
			collector("c1", "prod", "metrics", map[string]string{"team": "platform"}),
			collector("c2", "prod", "metrics", map[string]string{"team": "payments"}),
		}
		impacts := auditMatcherImpact(pipelines, collectors, flagsOff, flagsBoth)
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].PipelineID).To(Equal("p1"))
		Expect(impacts[0].Added).To(ConsistOf(collectorRef{ID: "c1", Cluster: "prod", Role: "metrics"}))
		Expect(impacts[0].Removed).To(BeEmpty())
	})

	It("reports a widen (added) for an '=~' matcher once the admin label is set", func() {
		pipelines := []merge.Pipeline{
			{ID: "p1", Name: "regex-widen", Matchers: []string{`team=~"platform.*"`}, Source: "ui"},
		}
		collectors := []auditCollector{
			collector("c1", "prod", "metrics", map[string]string{"team": "platform-eu"}),
			collector("c2", "prod", "metrics", map[string]string{"team": "payments"}),
		}
		impacts := auditMatcherImpact(pipelines, collectors, flagsOff, flagsBoth)
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].Added).To(ConsistOf(collectorRef{ID: "c1", Cluster: "prod", Role: "metrics"}))
		Expect(impacts[0].Removed).To(BeEmpty())
	})

	It("reports a shrink (removed) for a '!=' matcher when the admin label equals the excluded value", func() {
		pipelines := []merge.Pipeline{
			{ID: "p1", Name: "not-equals-shrink", Matchers: []string{`team!="platform"`}, Source: "ui"},
		}
		collectors := []auditCollector{
			// Today: unwired "team" reads as "", "" != "platform" is true, so
			// both collectors already match every pipeline using this matcher.
			collector("c1", "prod", "metrics", map[string]string{"team": "platform"}), // becomes excluded
			collector("c2", "prod", "metrics", map[string]string{"team": "payments"}), // stays matched
		}
		impacts := auditMatcherImpact(pipelines, collectors, flagsOff, flagsBoth)
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].Removed).To(ConsistOf(collectorRef{ID: "c1", Cluster: "prod", Role: "metrics"}))
		Expect(impacts[0].Added).To(BeEmpty())
	})

	It("reports a shrink (removed) for a '!~' matcher when the admin label matches the excluded regex", func() {
		pipelines := []merge.Pipeline{
			{ID: "p1", Name: "not-regex-shrink", Matchers: []string{`team!~"platform.*"`}, Source: "ui"},
		}
		collectors := []auditCollector{
			collector("c1", "prod", "metrics", map[string]string{"team": "platform-eu"}), // becomes excluded
			collector("c2", "prod", "metrics", map[string]string{"team": "payments"}),    // stays matched
		}
		impacts := auditMatcherImpact(pipelines, collectors, flagsOff, flagsBoth)
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].Removed).To(ConsistOf(collectorRef{ID: "c1", Cluster: "prod", Role: "metrics"}))
		Expect(impacts[0].Added).To(BeEmpty())
	})

	It("omits a pipeline with zero impact (e.g. a cluster/role-only matcher, unaffected by admin labels)", func() {
		pipelines := []merge.Pipeline{
			{ID: "p1", Name: "cluster-only", Matchers: []string{`cluster="prod"`}, Source: "ui"},
		}
		collectors := []auditCollector{
			collector("c1", "prod", "metrics", map[string]string{"team": "platform"}),
		}
		Expect(auditMatcherImpact(pipelines, collectors, flagsOff, flagsBoth)).To(BeEmpty())
	})

	It("never reports a git pipeline: it matches by collector ID, which admin labels can't change", func() {
		pipelines := []merge.Pipeline{
			{ID: "p1", Name: "git-pipe", Source: "git", RepoLinkCollectorID: "c1"},
		}
		collectors := []auditCollector{
			collector("c1", "prod", "metrics", map[string]string{"anything": "goes"}),
		}
		Expect(auditMatcherImpact(pipelines, collectors, flagsOff, flagsBoth)).To(BeEmpty())
	})

	It("aggregates multiple flipped collectors onto the same pipeline's impact entry", func() {
		pipelines := []merge.Pipeline{
			{ID: "p1", Name: "multi", Matchers: []string{`team="platform"`}, Source: "ui"},
		}
		collectors := []auditCollector{
			collector("c1", "prod", "metrics", map[string]string{"team": "platform"}),
			collector("c2", "prod", "logs", map[string]string{"team": "platform"}),
			collector("c3", "prod", "metrics", map[string]string{"team": "payments"}),
		}
		impacts := auditMatcherImpact(pipelines, collectors, flagsOff, flagsBoth)
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].Added).To(ConsistOf(
			collectorRef{ID: "c1", Cluster: "prod", Role: "metrics"},
			collectorRef{ID: "c2", Cluster: "prod", Role: "logs"},
		))
	})

	It("reports a widen (added) for an '=' matcher once the local attribute is reported", func() {
		pipelines := []merge.Pipeline{
			{ID: "p1", Name: "equals-widen-attrs", Matchers: []string{`team="platform"`}, Source: "ui"},
		}
		collectors := []auditCollector{
			{ID: "c1", Cluster: "prod", Role: "metrics", LocalAttrs: map[string]string{"team": "platform"}},
			{ID: "c2", Cluster: "prod", Role: "metrics", LocalAttrs: map[string]string{"team": "payments"}},
		}
		impacts := auditMatcherImpact(pipelines, collectors, flagsOff, flagsBoth)
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].Added).To(ConsistOf(collectorRef{ID: "c1", Cluster: "prod", Role: "metrics"}))
		Expect(impacts[0].Removed).To(BeEmpty())
	})

	It("reports a shrink (removed) for a '!=' matcher when the local attribute equals the excluded value", func() {
		pipelines := []merge.Pipeline{
			{ID: "p1", Name: "not-equals-shrink-attrs", Matchers: []string{`env!="dev"`}, Source: "ui"},
		}
		collectors := []auditCollector{
			// Today: unwired "env" reads as "", "" != "dev" is true, so both
			// collectors already match every pipeline using this matcher --
			// the exact self-report-env=dev-to-evade-env!=dev exploit shape
			// for agent-reported attributes.
			{ID: "c1", Cluster: "prod", Role: "metrics", LocalAttrs: map[string]string{"env": "dev"}},  // becomes excluded
			{ID: "c2", Cluster: "prod", Role: "metrics", LocalAttrs: map[string]string{"env": "prod"}}, // stays matched
		}
		impacts := auditMatcherImpact(pipelines, collectors, flagsOff, flagsBoth)
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].Removed).To(ConsistOf(collectorRef{ID: "c1", Cluster: "prod", Role: "metrics"}))
		Expect(impacts[0].Added).To(BeEmpty())
	})

	It("combines admin labels and local attributes in the same after-state", func() {
		// A matcher needing a key only local attrs ever supply must still
		// show up as an added collector once local_attrs are wired in
		// alongside an unrelated admin label on the same collector.
		pipelines := []merge.Pipeline{
			{ID: "p1", Name: "combined", Matchers: []string{`team="platform"`, `host="web-1"`}, Source: "ui"},
		}
		collectors := []auditCollector{
			{
				ID: "c1", Cluster: "prod", Role: "metrics",
				AdminLabels: map[string]string{"team": "platform"},
				LocalAttrs:  map[string]string{"host": "web-1"},
			},
		}
		impacts := auditMatcherImpact(pipelines, collectors, flagsOff, flagsBoth)
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].Added).To(ConsistOf(collectorRef{ID: "c1", Cluster: "prod", Role: "metrics"}))
	})

	It("returns no impacts across an empty pipeline or collector set", func() {
		Expect(auditMatcherImpact(nil, []auditCollector{collector("c1", "prod", "metrics", nil)}, flagsOff, flagsBoth)).To(BeEmpty())
		Expect(auditMatcherImpact([]merge.Pipeline{{ID: "p1", Name: "x", Matchers: []string{`team="platform"`}, Source: "ui"}}, nil, flagsOff, flagsBoth)).To(BeEmpty())
	})
})

var _ = Describe("formatCollectorRefs", func() {
	It("renders '(none)' for an empty slice", func() {
		Expect(formatCollectorRefs(nil)).To(Equal("(none)"))
	})

	It("renders comma-separated cluster/role (id) entries", func() {
		got := formatCollectorRefs([]collectorRef{
			{ID: "c1", Cluster: "prod", Role: "metrics"},
			{ID: "c2", Cluster: "prod", Role: "logs"},
		})
		Expect(got).To(Equal("prod/metrics (c1), prod/logs (c2)"))
	})

})

var _ = Describe("auditMatcherImpact flag states", func() {
	// The baseline is the org's current flags, not "everything off": a flag
	// already on is part of what is served today, so its effects are not new.
	It("does not report admin-label effects as new when allow_label_matching is already on", func() {
		pipelines := []merge.Pipeline{
			{ID: "p-label", Name: "by-label", Matchers: []string{`team="platform"`}, Source: "ui"},
			{ID: "p-attr", Name: "by-attr", Matchers: []string{`host="web-1"`}, Source: "ui"},
		}
		collectors := []auditCollector{{
			ID: "c1", Cluster: "prod", Role: "metrics",
			AdminLabels: map[string]string{"team": "platform"},
			LocalAttrs:  map[string]string{"host": "web-1"},
		}}
		impacts := auditMatcherImpact(pipelines, collectors, matchFlags{Labels: true}, flagsBoth)
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].PipelineID).To(Equal("p-attr"), "only the local attribute is new; the admin label already matches today")
		Expect(impacts[0].Added).To(ConsistOf(collectorRef{ID: "c1", Cluster: "prod", Role: "metrics"}))
	})

	It("audits local attributes on their own, ignoring admin labels that stay off", func() {
		pipelines := []merge.Pipeline{
			{ID: "p-label", Name: "by-label", Matchers: []string{`team="platform"`}, Source: "ui"},
			{ID: "p-attr", Name: "by-attr", Matchers: []string{`host="web-1"`}, Source: "ui"},
		}
		collectors := []auditCollector{{
			ID: "c1", Cluster: "prod", Role: "metrics",
			AdminLabels: map[string]string{"team": "platform"},
			LocalAttrs:  map[string]string{"host": "web-1"},
		}}
		impacts := auditMatcherImpact(pipelines, collectors, flagsOff, matchFlags{LocalAttrs: true})
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].PipelineID).To(Equal("p-attr"))
	})

	It("audits admin labels on their own, ignoring local attributes that stay off", func() {
		pipelines := []merge.Pipeline{
			{ID: "p-label", Name: "by-label", Matchers: []string{`team="platform"`}, Source: "ui"},
			{ID: "p-attr", Name: "by-attr", Matchers: []string{`host="web-1"`}, Source: "ui"},
		}
		collectors := []auditCollector{{
			ID: "c1", Cluster: "prod", Role: "metrics",
			AdminLabels: map[string]string{"team": "platform"},
			LocalAttrs:  map[string]string{"host": "web-1"},
		}}
		impacts := auditMatcherImpact(pipelines, collectors, flagsOff, matchFlags{Labels: true})
		Expect(impacts).To(HaveLen(1))
		Expect(impacts[0].PipelineID).To(Equal("p-label"))
	})
})

var _ = Describe("matchFlags.withEnabled", func() {
	It("turns on the named flag and keeps flags that are already on", func() {
		got, err := matchFlags{Labels: true}.withEnabled(enableLocalAttrs)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(flagsBoth))

		got, err = flagsOff.withEnabled(enableLabels)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(matchFlags{Labels: true}))

		got, err = flagsOff.withEnabled(enableBoth)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(flagsBoth))
	})

	It("rejects an unknown value", func() {
		_, err := flagsOff.withEnabled("everything")
		Expect(err).To(MatchError(ContainSubstring("invalid --enable")))
	})
})

// An unreadable stored blob must be reported by the caller, never read as
// "no attributes": that collector might match differently than the audit says.
var _ = Describe("decodeAuditMap", func() {
	It("reads an object, and treats an empty or null blob as empty", func() {
		m, ok := decodeAuditMap([]byte(`{"team":"x"}`))
		Expect(ok).To(BeTrue())
		Expect(m).To(Equal(map[string]string{"team": "x"}))

		m, ok = decodeAuditMap(nil)
		Expect(ok).To(BeTrue())
		Expect(m).To(BeEmpty())

		m, ok = decodeAuditMap([]byte(`null`))
		Expect(ok).To(BeTrue())
		Expect(m).To(BeEmpty())
	})

	It("reports unreadable data instead of returning an empty map silently", func() {
		for _, raw := range []string{`not json`, `["a","b"]`, `{"team":5}`} {
			_, ok := decodeAuditMap([]byte(raw))
			Expect(ok).To(BeFalse(), raw)
		}
	})
})
