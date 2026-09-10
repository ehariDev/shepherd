package repocheck_test

import (
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Red run, 2026-09-10: scripts/__pycache__/values_reference.cpython-314.pyc
// is a tracked file (git ls-files confirms exactly one) and .gitignore has no
// __pycache__/ or *.pyc line, so every `python3 scripts/build-docs.py`
// invocation (without -B) recreates untracked bytecode next to it and a
// `git add -A` would happily commit more of it.
var _ = Describe("python bytecode", func() {
	It("is untracked", func() {
		cmd := exec.Command("git", "ls-files", "--", "*.pyc", "**/__pycache__/*")
		cmd.Dir = repoRoot()
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
		Expect(strings.TrimSpace(string(out))).To(BeEmpty(), "tracked .pyc files:\n%s", out)
	})

	It("is ignored", func() {
		gi := readRepoFile(".gitignore")
		Expect(gi).To(ContainSubstring("__pycache__/"))
		Expect(gi).To(ContainSubstring("*.pyc"))

		cmd := exec.Command("git", "check-ignore", "-q", "scripts/__pycache__/probe.pyc")
		cmd.Dir = repoRoot()
		Expect(cmd.Run()).To(Succeed(), "scripts/__pycache__/probe.pyc should be git-ignored")
	})
})
