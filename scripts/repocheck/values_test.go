package repocheck_test

import (
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// emptyValuesDescriptions shells out to scripts/values_reference.py's own
// parser (rather than re-implementing YAML-comment parsing here) and returns
// every leaf key in deploy/helm/shepherd/values.yaml whose rendered
// description is blank -- the same emptiness site/docs/helm-values.html
// would ship for that row.
func emptyValuesDescriptions() []string {
	GinkgoHelper()
	script := `
import sys
sys.path.insert(0, "scripts")
import values_reference as v
keys = [k for s, r, _ in v.parse("deploy/helm/shepherd/values.yaml") for k, d, desc in r if not desc.strip()]
print("\n".join(keys))
`
	cmd := exec.Command("python3", "-B", "-c", script)
	cmd.Dir = repoRoot()
	out, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), string(out))
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// Red run, 2026-09-11: 64 leaves in deploy/helm/shepherd/values.yaml had no
// comment directly above them, so scripts/values_reference.py's parser (the
// same one that generates site/docs/helm-values.html) rendered each of them
// with an empty description cell on the docs page -- image.repository,
// service.type, every config.* leaf that is only a viper default with no
// values.yaml comment (server.listen, database.max_conns, oidc.scopes,
// auth.session_ttl, graph.base_url, agent.inactive_after/delete_after,
// validate.*, gitsync.tick, tracing.endpoint/protocol/insecure/service_name,
// log.level/format), cnpg.enabled/storage.size/owner/resources,
// externalSecrets.enabled/generatorApiVersion/bootstrapAdmin.length,
// route.*, ingress.*, metrics.enabled and every metrics.serviceMonitor.*
// leaf but `enabled` and `labels`, autoscaling.*, networkPolicy.enabled, and
// most of simulator.* (enabled, image.*, replicas, resources.requests/
// limits.cpu/memory, token.*, podAnnotations, nodeSelector, tolerations,
// affinity). A page with 64 blank description cells is not documentation.
var _ = Describe("deploy/helm/shepherd/values.yaml leaf comments", func() {
	It("gives every leaf key a non-empty description", func() {
		empty := emptyValuesDescriptions()
		Expect(empty).To(BeEmpty(),
			"%d values.yaml leaves render with an empty description in site/docs/helm-values.html "+
				"-- add a comment directly above each key in deploy/helm/shepherd/values.yaml:\n%s",
			len(empty), strings.Join(empty, "\n"))
	})
})
