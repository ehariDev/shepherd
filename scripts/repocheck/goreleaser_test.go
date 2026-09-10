// Specs over .goreleaser.yaml. Every spec here was written red first against
// the tree it guards; the spec comment records what the tree looked like
// when it failed.
package repocheck_test

import (
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// goreleaserConfig is the subset of .goreleaser.yaml these specs read.
type goreleaserConfig struct {
	Changelog struct {
		Filters struct {
			Exclude []string `yaml:"exclude"`
		} `yaml:"filters"`
	} `yaml:"changelog"`
}

// Red run, 2026-09-10: the exclude list was ['^docs:', '^chore:', '^test:',
// '^ci:'] -- unscoped, so it missed every scoped conventional-commit subject
// this repo actually uses: `git log` shows four "chore(release): prepare
// vX" commits (the release-prep commit itself), plus docs(...), test(chart):
// and ci(...) subjects, none of which matched the plain prefix.
var _ = Describe(".goreleaser.yaml", func() {
	It("keeps release-prep and scoped chore/docs/test/ci commits out of the changelog", func() {
		var cfg goreleaserConfig
		loadYAML(".goreleaser.yaml", &cfg)

		var combined []*regexp.Regexp
		for _, pat := range cfg.Changelog.Filters.Exclude {
			combined = append(combined, regexp.MustCompile(pat))
		}
		matches := func(subject string) bool {
			for _, re := range combined {
				if re.MatchString(subject) {
					return true
				}
			}
			return false
		}

		for _, subject := range []string{
			"chore(release): prepare v0.3.5",
			"test(chart): render NOTES offline, and satisfy errcheck",
			"docs(chart): correct the pages",
			"ci(deps): bump an action",
			// Unscoped forms must still match too.
			"chore: tidy",
			"docs: rewrite the README",
			"test: add a spec",
			"ci: fix a workflow",
		} {
			Expect(matches(subject)).To(BeTrue(), subject)
		}

		// feat/fix stay in the changelog regardless of scope.
		for _, subject := range []string{
			"feat(auth): add local users",
			"fix: gitsync name collisions",
		} {
			Expect(matches(subject)).To(BeFalse(), subject)
		}
	})
})
