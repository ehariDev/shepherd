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

// Red run, 2026-09-10: `shepherd hash-password` is not a registered CLI
// subcommand (internal/cli/*.go registers serve, migrate, validate,
// healthcheck, version, dev, token only) and SHEPHERD_AUTH_LOCAL_ADMIN_ENABLED
// / _PASSWORD_HASH are read nowhere in the binary. The real bootstrap path
// (internal/auth/localusers.go BootstrapAdmin, called from server.go at
// startup) reads SHEPHERD_BOOTSTRAP_ADMIN_LOGIN / SHEPHERD_BOOTSTRAP_ADMIN_PASSWORD
// instead, so `make smoke` was invoking a CLI subcommand and env vars that no
// longer exist and its local-admin-login step could never have passed.
var _ = Describe("the smoke target", func() {
	It("uses the bootstrap admin env vars, not the removed hash-password CLI", func() {
		recipe := makeRecipe("smoke")
		Expect(recipe).NotTo(ContainSubstring("hash-password"))
		Expect(recipe).NotTo(ContainSubstring("SHEPHERD_AUTH_LOCAL_ADMIN"))
		Expect(recipe).To(ContainSubstring("SHEPHERD_BOOTSTRAP_ADMIN_PASSWORD"))
		Expect(recipe).To(ContainSubstring("SHEPHERD_BOOTSTRAP_ADMIN_LOGIN"))
	})

	It("reuses the local and init images instead of building shepherd:smoke tags", func() {
		recipe := makeRecipe("smoke")
		Expect(recipe).NotTo(ContainSubstring("shepherd:smoke"))
		Expect(recipe).To(ContainSubstring("shepherd:local"))
		Expect(recipe).To(ContainSubstring("shepherd:local-init"))
		Expect(mkTargetLine("smoke")).To(SatisfyAll(
			ContainSubstring("docker-build-local"),
			ContainSubstring("docker-build-init"),
		), "smoke should depend on the docker-build-local/docker-build-init targets to build its images")
	})
})

// Red run, 2026-09-10: `make tools` installs the standalone gofumpt binary,
// but `make fmt` runs `golangci-lint fmt ./...` (its gofumpt formatter,
// module-aware) and the comment at Makefile:540-541 explains that the
// standalone binary mis-groups the dot-less `shepherd` module path and must
// NOT be used — so `make tools` installs a CLI that reformats the repo into
// a state `make lint` then refuses.
var _ = Describe("the tools target", func() {
	It("does not install the standalone gofumpt make fmt must not use", func() {
		Expect(makeRecipe("tools")).NotTo(ContainSubstring("gofumpt"))
	})
})
