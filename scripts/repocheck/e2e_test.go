// Specs over .github/workflows/e2e.yml. Every spec here was written red
// first against the tree it guards; the spec comment records what the tree
// looked like when it failed.
package repocheck_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// e2ePushTrigger is the subset of e2e.yml's on.push these specs read.
type e2ePushTrigger struct {
	Branches []string `yaml:"branches"`
	Paths    []string `yaml:"paths"`
}

// Red run, 2026-09-10: e2e.yml's `on:` had no `push:` key at all --
// workflow_dispatch, merge_group and a paths-scoped pull_request (for the
// egress job only). Per D11 and docs/spec.md's own "CI ordering" note that
// e2e lives outside every push, the agent-protocol suite (registration,
// pipelines, GitOps, RBAC, agent protocol) had never once run against main
// post-merge -- gh api confirms workflow run 32844383559's `e2e` job was
// "skipped" on a PR, and there is no merge queue configured on this repo
// (branch protection has no required_status_checks/rulesets), so merge_group
// never fires in practice either. The suite was effectively dead code.
var _ = Describe("e2e.yml", func() {
	It("runs the agent-protocol suite on pushes to main, path-filtered", func() {
		e2e := loadWorkflow("e2e.yml")

		pushRaw, ok := e2e.On["push"].(map[string]any)
		Expect(ok).To(BeTrue(), "e2e.yml has no on.push")

		branchesRaw, ok := pushRaw["branches"].([]any)
		Expect(ok).To(BeTrue(), "on.push.branches is not a list")
		var branches []string
		for _, b := range branchesRaw {
			branches = append(branches, b.(string))
		}
		Expect(branches).To(ContainElement("main"))

		pathsRaw, ok := pushRaw["paths"].([]any)
		Expect(ok).To(BeTrue(), "on.push.paths is not a list")
		var paths []string
		for _, p := range pathsRaw {
			paths = append(paths, p.(string))
		}
		// e2e/** minus e2e/k8s (the k8s suite has its own weekly/paths
		// workflow and cannot affect this compose-based suite), plus every
		// package the agent-protocol suite actually exercises.
		Expect(paths).To(ContainElement("e2e/**"))
		Expect(paths).To(ContainElement("!e2e/k8s/**"))
		Expect(paths).To(ContainElement("internal/agentapi/**"))
		Expect(paths).To(ContainElement("internal/gitsync/**"))
		Expect(paths).To(ContainElement("internal/auth/**"))
		Expect(paths).To(ContainElement("deploy/Dockerfile*"))
		Expect(paths).To(ContainElement("deploy/versions.env"))

		// The e2e job's own `if` must not have grown a push exclusion --
		// it already reads `!= 'pull_request'`, which permits push, but a
		// careless S5 implementation could easily add one back.
		e2eJob, ok := e2e.Jobs["e2e"]
		Expect(ok).To(BeTrue(), "e2e.yml has no e2e job")
		Expect(e2eJob.If).NotTo(ContainSubstring("'push'"),
			"the e2e job's if must not exclude push events")
		Expect(e2eJob.If).To(ContainSubstring("pull_request"),
			"the e2e job's if should still exclude pull_request (merge_group/dispatch/push only)")

		// e2e-egress must NOT gain the push trigger: it is a separate,
		// equally expensive job (its own runner, its own image builds) whose
		// budget (docs/spec.md's per-run cost accounting) assumed only the
		// `e2e` job runs on a qualifying push. Without an explicit exclusion
		// here, a workflow-level push trigger fires every job with no `if`,
		// silently doubling the billed cost of every push to main.
		egressJob, ok := e2e.Jobs["e2e-egress"]
		Expect(ok).To(BeTrue(), "e2e.yml has no e2e-egress job")
		Expect(egressJob.If).To(ContainSubstring("push"),
			"e2e-egress must gain an explicit if excluding the push trigger")
	})
})
