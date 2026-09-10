// Specs over every .github/workflows/*.yml file, checking that every third-
// party Action is pinned to a full commit SHA (not a mutable major/minor tag)
// with a trailing version comment. Every spec here was written red first
// against the tree it guards; the spec comment records what the tree looked
// like when it failed.
package repocheck_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// usesLineRE matches a `uses: <ref>` line anywhere in a workflow file
// (top-level step or a job's own `uses:`, at any indentation).
var usesLineRE = regexp.MustCompile(`(?m)^\s*(?:-\s*)?uses:\s*(\S+)\s*(#.*)?$`)

// pinnedRefRE requires owner/repo(/subpath)*@<40-hex-sha>.
var pinnedRefRE = regexp.MustCompile(`^[\w.-]+/[\w.-]+(?:/[\w.-]+)*@[0-9a-f]{40}$`)

// versionCommentRE requires a trailing `# vX...` comment recording the tag
// the pinned SHA corresponds to (the whole point of pinning by SHA instead of
// a tag is losing nothing a human reads -- the comment is what a reviewer
// actually checks against `gh api .../git/ref/tags/<tag>`).
var versionCommentRE = regexp.MustCompile(`^#\s*v[0-9]`)

func workflowFiles() []string {
	GinkgoHelper()
	entries, err := os.ReadDir(filepath.Join(repoRoot(), ".github", "workflows"))
	Expect(err).NotTo(HaveOccurred())
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yml" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	Expect(files).NotTo(BeEmpty(), "found no .github/workflows/*.yml files")
	return files
}

// Red run, 2026-09-10: ci.yml, e2e.yml, e2e-k8s.yml and schema-verify.yml
// referenced 9 distinct third-party Actions (actions/checkout,
// actions/setup-go, actions/cache, actions/setup-node,
// actions/upload-artifact, pnpm/action-setup, golangci/golangci-lint-action,
// docker/setup-buildx-action, helm/kind-action) by mutable major-version tag
// (@v4, @v5, @v3, @v1, @v8...) rather than a commit SHA -- e.g. ci.yml:
// `uses: actions/checkout@v4`. A tag can be force-moved by its publisher (or,
// for a compromised account, by an attacker) to point at different, unreviewed
// code that then runs with this repo's GITHUB_TOKEN on every PR. release.yml
// and pages.yml were already fully SHA-pinned (an earlier workstream); this
// closes the same gap everywhere else and locks BOTH files in permanently so
// neither can regress back to a tag.
var _ = Describe("every workflow Action is pinned to a full commit SHA", func() {
	for _, file := range workflowFiles() {
		file := file
		It(fmt.Sprintf("%s: every `uses:` is <owner>/<repo>@<40-hex-sha> # <version>", file), func() {
			content := readRepoFile(filepath.Join(".github", "workflows", file))
			matches := usesLineRE.FindAllStringSubmatch(content, -1)
			Expect(matches).NotTo(BeEmpty(), "%s has no `uses:` steps to check", file)

			for _, m := range matches {
				ref, comment := m[1], m[2]
				Expect(pinnedRefRE.MatchString(ref)).To(BeTrue(),
					"%s: %q is not pinned to a full commit SHA (owner/repo@<40-hex>)", file, ref)
				Expect(versionCommentRE.MatchString(comment)).To(BeTrue(),
					"%s: %q has no trailing `# vX...` version comment (got %q)", file, ref, comment)
			}
		})
	}
})
