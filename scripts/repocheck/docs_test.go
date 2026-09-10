package repocheck_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Red run, 2026-09-10: scripts/build-docs.py hard-codes OUT = site/docs and
// has no --out flag, so it can only ever write into the committed tree —
// check-docs-drift's `python3 scripts/build-docs.py` step (Makefile:491)
// rewrites site/docs/ on every `make lint`, which is why check-docs-drift's
// own recipe comment has to explain that away instead of just diffing.
var _ = Describe("scripts/build-docs.py", func() {
	It("can build the site into a directory other than site/docs", func() {
		tmp := GinkgoT().TempDir()
		cmd := exec.Command("python3", "-B", "scripts/build-docs.py", "--out", tmp)
		cmd.Dir = repoRoot()
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
		Expect(filepath.Join(tmp, "quickstart.html")).To(BeAnExistingFile())
		Expect(filepath.Join(tmp, "index.html")).To(BeAnExistingFile())
	})

	It("--check compares against site/docs without writing to it", func() {
		target := filepath.Join(repoRoot(), "site", "docs", "index.html")
		before, err := os.Stat(target)
		Expect(err).NotTo(HaveOccurred())
		mtimeBefore := before.ModTime()

		cmd := exec.Command("python3", "-B", "scripts/build-docs.py", "--check")
		cmd.Dir = repoRoot()
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))

		after, err := os.Stat(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.ModTime()).To(Equal(mtimeBefore), "--check must not write to site/docs (mtime changed): %s", string(out))

		diffOut, dErr := runMake("check-docs-drift")
		Expect(dErr).NotTo(HaveOccurred(), diffOut)
	})
})
