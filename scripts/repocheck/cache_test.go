// Specs over the actions/cache buildx-layer-cache steps shared by ci.yml,
// e2e.yml and e2e-k8s.yml. Every spec here was written red first against the
// tree it guards; the spec comment records what the tree looked like when it
// failed.
package repocheck_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// buildxCacheSteps returns every step across every job in the given
// workflow file whose `with.path` is the shared buildx cache directory.
func buildxCacheSteps(file string) []workflowStep {
	GinkgoHelper()
	w := loadWorkflow(file)
	var out []workflowStep
	for _, j := range w.Jobs {
		for _, s := range j.Steps {
			if p, ok := s.With["path"].(string); ok && p == "/tmp/.buildx-cache" {
				out = append(out, s)
			}
		}
	}
	return out
}

// Red run, 2026-09-10: all four buildx cache steps (ci.yml's test-fullstack
// job, e2e.yml's e2e and e2e-egress jobs, e2e-k8s.yml's e2e-k8s job) used the
// identical key `buildx-${{ runner.os }}-${{ hashFiles(...) }}` with the
// identical set of hashed files. actions/cache never overwrites an existing
// key -- whichever workflow run saved that key first "wins" it permanently,
// and every other workflow (and every later run of the SAME workflow, since
// the hashed inputs rarely change) hits a plain cache read with no save, so
// its own "Rotate Docker layer cache" step's `/tmp/.buildx-cache-new` never
// exists and the rotate is a silent no-op forever.
var _ = Describe("buildx layer cache keys", func() {
	for _, file := range []string{"ci.yml", "e2e.yml", "e2e-k8s.yml"} {
		It(fmt.Sprintf("scopes each cache key in %s to its own workflow and run", file), func() {
			steps := buildxCacheSteps(file)
			Expect(steps).NotTo(BeEmpty(), "%s has no buildx cache step", file)

			for i, s := range steps {
				key, ok := s.With["key"].(string)
				Expect(ok).To(BeTrue(), "%s cache step %d: with.key is not a string", file, i)
				Expect(key).To(ContainSubstring("github.workflow"),
					"%s cache step %d: key must include github.workflow so ci/e2e/e2e-k8s don't share one slot", file, i)
				Expect(key).To(ContainSubstring("github.run_id"),
					"%s cache step %d: key must include github.run_id so actions/cache always has something new to save (it never overwrites an existing key)", file, i)

				restore, ok := s.With["restore-keys"].(string)
				Expect(ok).To(BeTrue(), "%s cache step %d: with.restore-keys is not a string", file, i)
				Expect(strings.TrimSpace(restore)).To(ContainSubstring("github.workflow"),
					"%s cache step %d: restore-keys must stay scoped per workflow so a restore can't cross-pollinate from a different workflow's cache", file, i)
				Expect(restore).NotTo(ContainSubstring("github.run_id"),
					"%s cache step %d: restore-keys must NOT include github.run_id -- a prefix match needs to hit an EARLIER run's saved cache, not require the exact current one", file, i)
			}
		})
	}
})
