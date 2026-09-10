// Specs over .github/workflows/release.yml: the workflow that cuts a tagged
// release with goreleaser. Every spec here was written red first against the
// tree it guards; the spec comment records what the tree looked like when it
// failed.
package repocheck_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Red run, 2026-09-10: the "chart version already published" probe lived
// only inside the post-goreleaser "Publish the Helm chart to GHCR (OCI)"
// step (steps[15]), which runs after "goreleaser release" (steps[9]) has
// already pushed both images and cut the GitHub release. A duplicate chart
// version failed the workflow only once that half-published state existed,
// exactly what the comment above the appVersion check (lines 89-93) says the
// workflow tries to avoid.
var _ = Describe("release.yml", func() {
	It("refuses a duplicate chart version before anything is published", func() {
		rel := loadWorkflow("release.yml")
		release, ok := rel.Jobs["release"]
		Expect(ok).To(BeTrue(), "release.yml has no release job")
		steps := release.Steps

		idxProbe, idxGoreleaser := -1, -1
		for i, s := range steps {
			if idxProbe == -1 && strings.Contains(s.Run, "already published") {
				idxProbe = i
			}
			if idxGoreleaser == -1 && strings.HasPrefix(s.Uses, "goreleaser/goreleaser-action") {
				idxGoreleaser = i
			}
		}
		Expect(idxProbe).To(BeNumerically(">=", 0), "no step probes for an already-published chart version")
		Expect(idxGoreleaser).To(BeNumerically(">=", 0), "no goreleaser step found")
		Expect(idxProbe).To(BeNumerically("<", idxGoreleaser),
			"the already-published probe must run before goreleaser publishes images")
	})

	// Red run, 2026-09-10: release.yml had one job ("release") that checked
	// out the tagged commit and ran goreleaser directly -- no lint, no
	// `go vet`, no `go test`, no guards. A tag that failed CI on main (or
	// was never even pushed as a branch) could still cut a release.
	It("lints and tests the tagged commit before goreleaser runs", func() {
		rel := loadWorkflow("release.yml")

		verify, ok := rel.Jobs["verify"]
		Expect(ok).To(BeTrue(), "release.yml has no verify job")

		release, ok := rel.Jobs["release"]
		Expect(ok).To(BeTrue(), "release.yml has no release job")
		Expect(needsList(release.Needs)).To(ContainElement("verify"),
			"the release job must need the verify job")

		joined := joinedRuns(verify.Steps)
		Expect(joined).To(ContainSubstring("make guards"))
		Expect(joined).To(ContainSubstring("go build ./..."))
		Expect(joined).To(ContainSubstring("go vet ./..."))
		Expect(joined).To(ContainSubstring("go test ./..."))
		Expect(joined).To(ContainSubstring("make helm-lint"))
		Expect(joined).To(ContainSubstring("docker pull"))

		var usesGolangciLint bool
		for _, s := range verify.Steps {
			if strings.HasPrefix(s.Uses, "golangci/golangci-lint-action") {
				usesGolangciLint = true
			}
		}
		Expect(usesGolangciLint).To(BeTrue(), "verify job must run golangci-lint")
	})
})

// needsList normalizes a job's `needs:` field (a bare string or a list of
// strings in YAML) into a slice.
func needsList(needs any) []string {
	switch v := needs.(type) {
	case nil:
		return nil
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
