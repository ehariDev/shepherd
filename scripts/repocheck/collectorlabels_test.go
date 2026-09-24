package repocheck_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// collectorLabelsLiteralRe matches a hand-rolled merge.CollectorLabels{...}
// struct literal. internal/merge.BuildCollectorLabels is the only correct,
// gated way to build one (it merges admin labels and local attributes and
// enforces their precedence against the built-in cluster/role labels) — see
// docs/plans/2026-09-24-matcher-targeting-unified-plan.md §0 item 4 and
// Phase 1 task 1.3. rpc_reconcile.go's reconcileServed hand-inlined this
// literal with only {role, cluster}, silently omitting admin labels and
// local attributes for any org with either matching flag on. This guard
// exists so that bug's exact shape cannot recur unnoticed anywhere else.
var collectorLabelsLiteralRe = regexp.MustCompile(`merge\.CollectorLabels\{`)

// excludedCollectorLabelsDirs are directories a repo-wide Go source walk
// skips entirely: not Go source (node_modules, web), version control
// metadata (.git), or generated code no one hand-edits (gen — see AGENTS.md
// "Never do: edit generated code").
var excludedCollectorLabelsDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"web":          true,
	"gen":          true,
}

// findCollectorLabelsLiterals walks root for .go files (skipping directories
// in skipDirs and, when skipTests is true, any _test.go file) and returns the
// repo-relative paths of every one whose content matches collectorLabelsLiteralRe.
func findCollectorLabelsLiterals(root string, skipDirs map[string]bool, skipTests bool) []string {
	GinkgoHelper()
	var hits []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if skipTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		Expect(relErr).NotTo(HaveOccurred())
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/merge/") {
			// The one package allowed to construct this literal directly —
			// it's what BuildCollectorLabels itself, and the merge package's
			// own tests calling the public API with test-controlled labels,
			// legitimately do.
			return nil
		}
		b, readErr := os.ReadFile(path)
		Expect(readErr).NotTo(HaveOccurred(), path)
		if collectorLabelsLiteralRe.Match(b) {
			hits = append(hits, rel)
		}
		return nil
	})
	Expect(err).NotTo(HaveOccurred())
	return hits
}

var _ = Describe("no merge.CollectorLabels{} literal outside internal/merge", func() {
	// Regression guard for Phase 1 task 1.3's fix: reconcileServed used to be
	// the one caller that hand-built a CollectorLabels{role, cluster} literal
	// instead of calling merge.BuildCollectorLabels, silently dropping admin
	// labels and local attributes. Also excludes _test.go files repo-wide:
	// internal/cli/dev_test.go legitimately builds a raw CollectorLabels
	// fixture to call MatchesPipeline/Assemble directly with test-controlled
	// labels, which is not the bug this guard targets (a production code path
	// computing what labels a REAL collector has).
	It("finds no hand-rolled literal in production code", func() {
		hits := findCollectorLabelsLiterals(repoRoot(), excludedCollectorLabelsDirs, true)
		Expect(hits).To(BeEmpty(),
			"use merge.BuildCollectorLabels instead of a raw CollectorLabels{} literal outside internal/merge: %v", hits)
	})

	// Explicit confirmation per task 1.6: internal/cli/admin.go's
	// audit-matcher-impact command already does its own before/after diff
	// correctly, via two BuildCollectorLabels calls, and must not trip this
	// guard.
	It("does not flag internal/cli/admin.go, which already uses BuildCollectorLabels correctly", func() {
		hits := findCollectorLabelsLiterals(repoRoot(), excludedCollectorLabelsDirs, true)
		Expect(hits).NotTo(ContainElement("internal/cli/admin.go"))
	})

	// Self-test: proves the scanner actually detects the pattern (red) and
	// stops once removed (green), using a throwaway file in a scratch temp
	// directory rather than a permanent repo file — this is the guard's own
	// proof it isn't vacuously passing because it never matches anything.
	It("fires on a deliberately introduced literal outside internal/merge, and clears once removed", func() {
		scratch := GinkgoT().TempDir()
		violating := filepath.Join(scratch, "fake_reconcile.go")
		clean := filepath.Join(scratch, "fake_reconcile_test.go") // excluded by name, must never count

		write := func(path, content string) {
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
		}

		write(violating, "package fake\n\nfunc bad() {\n\t_ = merge.CollectorLabels{}\n}\n")
		write(clean, "package fake\n\nfunc alsoBad() {\n\t_ = merge.CollectorLabels{}\n}\n")

		hits := findCollectorLabelsLiterals(scratch, map[string]bool{}, true)
		Expect(hits).To(ConsistOf("fake_reconcile.go"), "must catch the non-test file and skip the _test.go one")

		Expect(os.Remove(violating)).To(Succeed())
		Expect(findCollectorLabelsLiterals(scratch, map[string]bool{}, true)).To(BeEmpty(), "must clear once the literal is removed")
	})
})
