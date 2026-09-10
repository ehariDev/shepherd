package repocheck_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Red run, 2026-09-10: ci.yml's guards job ran six make guards and helm lint
// and never executed this package, so nothing here could gate a PR.
var _ = Describe("the repocheck harness", func() {
	It("runs in CI's guards job", func() {
		ci := loadWorkflow("ci.yml")
		guards, ok := ci.Jobs["guards"]
		Expect(ok).To(BeTrue(), "ci.yml has no guards job")
		Expect(joinedRuns(guards.Steps)).To(ContainSubstring("go test ./scripts/repocheck"))
	})
})
