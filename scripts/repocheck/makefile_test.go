package repocheck_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Red run, 2026-09-10: `docs` was missing from .PHONY and a docs/ directory
// exists at the repo root, so `make -n docs` printed "make: 'docs' is up to
// date." and never invoked scripts/build-docs.py. check-docs-drift's error
// message told people to run a target that did nothing.
var _ = Describe("the Makefile", func() {
	It("declares the docs targets phony so a docs/ directory cannot satisfy them", func() {
		mk := readRepoFile("Makefile")
		for _, t := range []string{"docs", "check-docs-drift", "check-docs-version"} {
			Expect(mk).To(MatchRegexp(`(?m)^\.PHONY:.*\b`+t+`\b`), t)
		}
	})

	It("invokes the site generator from make docs even though docs/ exists", func() {
		out, err := runMake("-n", "docs")
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("scripts/build-docs.py"))
		Expect(out).NotTo(ContainSubstring("is up to date"))
		Expect(makeRecipe("docs")).To(ContainSubstring("build-docs.py"))
	})
})
