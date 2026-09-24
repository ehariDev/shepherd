package merge_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"shepherd/internal/merge"
)

var _ = Describe("Evaluate", func() {
	reg := testRegistry()

	It("requires a non-nil schema registry", func() {
		_, err := merge.Evaluate(merge.Pipeline{Name: "x", Source: "ui"}, merge.CollectorLabels{}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("non-nil schema registry"))
	})

	It("matches and serves a git pipeline whose RepoLinkCollectorID equals the collector", func() {
		cl := merge.CollectorLabels{CollectorID: "coll-1", Labels: map[string]string{"role": "metrics"}}
		p := merge.Pipeline{Name: "git-pipe", Source: "git", RepoLinkCollectorID: "coll-1", Contents: metricsOnlyPipeline}
		result, err := merge.Evaluate(p, cl, reg)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(merge.EvalResult{Matched: true, Served: true}))
	})

	It("does not match a git pipeline linked to a different collector", func() {
		cl := merge.CollectorLabels{CollectorID: "coll-1", Labels: map[string]string{"role": "metrics"}}
		p := merge.Pipeline{Name: "git-pipe", Source: "git", RepoLinkCollectorID: "coll-2", Contents: metricsOnlyPipeline}
		result, err := merge.Evaluate(p, cl, reg)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(merge.EvalResult{}))
	})

	It("reports zero_matchers for a non-git pipeline with no matchers, without an error", func() {
		cl := merge.CollectorLabels{CollectorID: "coll-1", Labels: map[string]string{"role": "metrics", "cluster": "test"}}
		p := merge.Pipeline{Name: "empty", Source: "ui", Matchers: []string{}, Contents: metricsOnlyPipeline}
		result, err := merge.Evaluate(p, cl, reg)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(merge.EvalResult{Reason: merge.ReasonZeroMatchers}))
	})

	It("reports an ordinary non-match with no reason", func() {
		cl := merge.CollectorLabels{CollectorID: "coll-1", Labels: map[string]string{"role": "metrics", "cluster": "test"}}
		p := merge.Pipeline{Name: "no-match", Source: "ui", Matchers: []string{`cluster="other"`}, Contents: metricsOnlyPipeline}
		result, err := merge.Evaluate(p, cl, reg)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(merge.EvalResult{}))
	})

	It("reports unparsable_matcher with a detailed error, matched false", func() {
		cl := merge.CollectorLabels{CollectorID: "coll-1", Labels: map[string]string{"role": "metrics", "cluster": "test"}}
		p := merge.Pipeline{Name: "broken", Source: "ui", Matchers: []string{`cluster=~"["`}, Contents: metricsOnlyPipeline}
		result, err := merge.Evaluate(p, cl, reg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unparsable matcher"))
		Expect(result).To(Equal(merge.EvalResult{Reason: merge.ReasonUnparsableMatcher}))
	})

	It("reports role_signal_mismatch for a matched pipeline whose signals the role disallows", func() {
		cl := merge.CollectorLabels{CollectorID: "coll-1", Labels: map[string]string{"role": "logs", "cluster": "test"}}
		p := merge.Pipeline{Name: "metrics-pipe", Source: "ui", Matchers: []string{`cluster="test"`}, Contents: metricsOnlyPipeline}
		result, err := merge.Evaluate(p, cl, reg)
		Expect(err).To(HaveOccurred())
		Expect(result).To(Equal(merge.EvalResult{Matched: true, Reason: merge.ReasonRoleSignalMismatch}))
	})

	It("matches and serves a pipeline whose signals the role allows", func() {
		cl := merge.CollectorLabels{CollectorID: "coll-1", Labels: map[string]string{"role": "metrics", "cluster": "test"}}
		p := merge.Pipeline{Name: "metrics-pipe", Source: "ui", Matchers: []string{`cluster="test"`}, Contents: metricsOnlyPipeline}
		result, err := merge.Evaluate(p, cl, reg)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(merge.EvalResult{Matched: true, Served: true}))
	})

	// Parity: Evaluate's per-pipeline verdict must agree with what Assemble
	// (which now calls Evaluate internally) actually does with the same
	// fixture, for every case exercised below. This is the test that proves
	// the extraction into a standalone function didn't change behavior — see
	// docs/plans/2026-09-24-matcher-targeting-unified-plan.md Phase 1 task 1.2.
	DescribeTable("agrees with Assemble's selection and exclusion outcome",
		func(cl merge.CollectorLabels, pipelines []merge.Pipeline) {
			assembled, err := merge.Assemble("coll-1", "test", cl, pipelines, "dev", "2024-01-01T00:00:00Z", merge.WithRoleEnforcement(reg))
			Expect(err).NotTo(HaveOccurred())

			for _, p := range pipelines {
				result, evalErr := merge.Evaluate(p, cl, reg)
				servedInAssemble := strings.Contains(assembled.Content, "pipe_"+merge.SanitizeName(p.Name))
				Expect(result.Served).To(Equal(servedInAssemble), "pipeline %q Served mismatch", p.Name)

				excluded := exclusionFor(assembled.Exclusions, p.Name)
				switch {
				case result.Served:
					Expect(excluded).To(BeNil(), "pipeline %q served but also excluded", p.Name)
				case evalErr != nil:
					Expect(excluded).NotTo(BeNil(), "pipeline %q should have an exclusion", p.Name)
					Expect(excluded.Reason).To(Equal(evalErr.Error()))
				default:
					Expect(excluded).To(BeNil(), "pipeline %q has no eval error but was excluded", p.Name)
				}
			}
		},
		Entry("mixed match/role-mismatch/unparsable set", merge.CollectorLabels{
			CollectorID: "coll-1", Labels: map[string]string{"cluster": "test", "role": "logs"},
		}, []merge.Pipeline{
			{Name: "broken-matcher", Contents: logsOnlyPipeline, Matchers: []string{`cluster=~"["`}, Source: "ui"},
			{Name: "metrics-pipe", Contents: metricsOnlyPipeline, Matchers: []string{`cluster="test"`}, Source: "ui"},
			{Name: "logs-pipe", Contents: logsOnlyPipeline, Matchers: []string{`cluster="test"`}, Source: "ui"},
			{Name: "no-match-pipe", Contents: logsOnlyPipeline, Matchers: []string{`cluster="other"`}, Source: "ui"},
			{Name: "empty-matchers-pipe", Contents: logsOnlyPipeline, Matchers: []string{}, Source: "ui"},
		}),
		Entry("singleton role is unrestricted", merge.CollectorLabels{
			CollectorID: "coll-1", Labels: map[string]string{"cluster": "test", "role": "singleton"},
		}, []merge.Pipeline{
			{Name: "mixed-pipe", Contents: metricsOnlyPipeline + "\n" + logsOnlyPipeline, Matchers: []string{`cluster="test"`}, Source: "ui"},
		}),
	)
})

func exclusionFor(exclusions []merge.Exclusion, pipelineName string) *merge.Exclusion {
	for i := range exclusions {
		if exclusions[i].PipelineName == pipelineName {
			return &exclusions[i]
		}
	}
	return nil
}
