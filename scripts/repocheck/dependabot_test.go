// Specs over .github/dependabot.yml. Every spec here was written red first
// against the tree it guards; the spec comment records what the tree looked
// like when it failed.
package repocheck_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// dependabotConfig is the subset of .github/dependabot.yml these specs read.
type dependabotConfig struct {
	Version int                `yaml:"version"`
	Updates []dependabotUpdate `yaml:"updates"`
}

type dependabotUpdate struct {
	Ecosystem string                   `yaml:"package-ecosystem"`
	Directory string                   `yaml:"directory"`
	Schedule  dependabotSchedule       `yaml:"schedule"`
	Groups    map[string]dependabotGrp `yaml:"groups"`
}

type dependabotSchedule struct {
	Interval string `yaml:"interval"`
}

type dependabotGrp struct {
	UpdateTypes []string `yaml:"update-types"`
}

// Red run, 2026-09-10: .github/dependabot.yml does not exist -- the repo has
// no automated dependency-update coverage for gomod, npm, github-actions or
// docker, so every one of them can drift silently between manual bumps.
var _ = Describe(".github/dependabot.yml", func() {
	It("declares weekly grouped updates for every ecosystem the repo ships", func() {
		var cfg dependabotConfig
		loadYAML(".github/dependabot.yml", &cfg)

		Expect(cfg.Version).To(Equal(2))

		type key struct{ eco, dir string }
		want := []key{
			{"gomod", "/"},
			{"npm", "/web"},
			{"github-actions", "/"},
			{"docker", "/deploy"},
		}

		got := map[key]dependabotUpdate{}
		for _, u := range cfg.Updates {
			got[key{u.Ecosystem, u.Directory}] = u
		}

		for _, k := range want {
			u, ok := got[k]
			Expect(ok).To(BeTrue(), "missing updates entry for ecosystem %q directory %q", k.eco, k.dir)
			Expect(u.Schedule.Interval).To(Equal("weekly"), "%s %s must schedule weekly", k.eco, k.dir)
			Expect(u.Groups).NotTo(BeEmpty(), "%s %s must group its updates", k.eco, k.dir)

			var sawMinor, sawPatch bool
			for _, g := range u.Groups {
				for _, t := range g.UpdateTypes {
					if t == "minor" {
						sawMinor = true
					}
					if t == "patch" {
						sawPatch = true
					}
				}
			}
			Expect(sawMinor).To(BeTrue(), "%s %s must group minor updates", k.eco, k.dir)
			Expect(sawPatch).To(BeTrue(), "%s %s must group patch updates", k.eco, k.dir)
		}

		Expect(got).To(HaveLen(4), "expected exactly the four ecosystems, got %d entries", len(got))
	})
})
