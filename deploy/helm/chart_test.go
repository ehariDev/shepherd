package helm_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// renderSimulatorEnabled runs the real `helm template` binary — not a
// hand-parsed approximation of it — over the chart with the simulator turned
// on, and returns every rendered object keyed by "Kind/Name". A control this
// suite does not read back out of ACTUAL helm output could pass while the
// template that produces it is broken; that is finding H4's exact mistake
// (a containment claim no test exercised), applied to the chart.
func renderSimulatorEnabled() map[string]map[string]any {
	dir := GinkgoT().TempDir()
	overridePath := filepath.Join(dir, "simulator-enabled.yaml")
	Expect(os.WriteFile(overridePath, []byte("simulator:\n  enabled: true\n"), 0o600)).To(Succeed())

	cmd := exec.Command("helm", "template", "shepherd", "shepherd",
		"-f", "shepherd/ci/default-values.yaml",
		"-f", overridePath,
	)
	out, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "helm template failed:\n%s", out)

	objects := map[string]map[string]any{}
	dec := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var doc map[string]any
		if decErr := dec.Decode(&doc); decErr != nil {
			break
		}
		if doc == nil {
			continue
		}
		kind, _ := doc["kind"].(string) //nolint:errcheck // a document missing "kind" is not a k8s object worth indexing
		meta, _ := doc["metadata"].(map[string]any)
		name, _ := meta["name"].(string) //nolint:errcheck // same
		if kind == "" || name == "" {
			continue
		}
		objects[kind+"/"+name] = doc
	}
	return objects
}

var _ = Describe("Helm chart: S3 sandbox simulator containment (finding H5)", func() {
	// Every assertion here corresponds to one bullet in values.yaml's
	// simulator block comment. A bullet with no assertion below is a claim,
	// not a control.

	It("is present in the default render WITH its NetworkPolicy — enabled by default since v0.0.1", func() {
		// Both containment gates are closed (project-status.md F5); shipping
		// the simulator by default is only acceptable as long as the render
		// that ships it also ships the default-deny NetworkPolicy — assert
		// them together so they cannot drift apart.
		out, err := exec.Command("helm", "template", "shepherd", "shepherd",
			"-f", "shepherd/ci/default-values.yaml").CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "helm template failed:\n%s", out)
		Expect(string(out)).To(ContainSubstring("Deployment"), "sanity")
		Expect(string(out)).To(ContainSubstring("shepherd-simulator"),
			"the simulator must render by default since v0.0.1")
		Expect(string(out)).To(ContainSubstring("kind: NetworkPolicy"),
			"a default render that deploys the simulator without its default-deny NetworkPolicy ships an unconfined sandbox")
	})

	It("renders nothing simulator-related when simulator.enabled is false — the documented off-switch", func() {
		out, err := exec.Command("helm", "template", "shepherd", "shepherd",
			"-f", "shepherd/ci/default-values.yaml", "--set", "simulator.enabled=false").CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "helm template failed:\n%s", out)
		Expect(string(out)).NotTo(ContainSubstring("shepherd-simulator"),
			"simulator.enabled=false must render nothing simulator-related, including the config wiring")
	})

	Describe("with simulator.enabled: true", func() {
		var objects map[string]map[string]any

		BeforeEach(func() { objects = renderSimulatorEnabled() })

		// Red run, 2026-09-11 (kind gate on the merged remediation branch): the
		// pre-install migration Job inherited SHEPHERD_SIMULATOR_TOKEN from the
		// shared shepherd.podEnv helper, but the token Secret is an ordinary
		// chart resource that does not exist yet while pre-install hooks run,
		// so every install died in CreateContainerConfigError ("secret
		// shepherd-def-simulator-token not found") until the 5-minute wait
		// expired. The migration needs the database, never the simulator.
		It("keeps the simulator token off the pre-install migration Job", func() {
			job, ok := objects["Job/shepherd-migrate"]
			Expect(ok).To(BeTrue(), "no Job/shepherd-migrate rendered")
			env := envOf(containerOf(job, "migrate"))
			Expect(env).NotTo(HaveKey("SHEPHERD_SIMULATOR_TOKEN"))
			volumes, _ := podSpecOf(job)["volumes"].([]any) //nolint:errcheck // absent volumes is a pass
			for _, raw := range volumes {
				v, _ := raw.(map[string]any) //nolint:errcheck // shape asserted by the key lookup
				if sec, ok := v["secret"].(map[string]any); ok {
					Expect(sec["secretName"]).NotTo(Equal("shepherd-simulator-token"))
				}
			}
		})

		It("renders a Deployment and a Service", func() {
			Expect(objects).To(HaveKey("Deployment/shepherd-simulator"))
			Expect(objects).To(HaveKey("Service/shepherd-simulator"))
		})

		It("does not automount a service-account token", func() {
			sa, ok := objects["ServiceAccount/shepherd-simulator"]
			Expect(ok).To(BeTrue(), "no ServiceAccount rendered")
			Expect(sa["automountServiceAccountToken"]).To(Equal(false))

			podSpec := podSpecOf(objects["Deployment/shepherd-simulator"])
			Expect(podSpec["automountServiceAccountToken"]).To(Equal(false),
				"the Pod spec must also refuse the mount — an operator who does not deploy this ServiceAccount must not fall back to the namespace default")
			Expect(podSpec["serviceAccountName"]).To(Equal("shepherd-simulator"))
		})

		It("runs as non-root", func() {
			podSpec := podSpecOf(objects["Deployment/shepherd-simulator"])
			sc, ok := podSpec["securityContext"].(map[string]any)
			Expect(ok).To(BeTrue(), "Pod spec has no securityContext")
			Expect(sc["runAsNonRoot"]).To(Equal(true))
			Expect(sc["runAsUser"]).NotTo(BeEquivalentTo(0))
		})

		It("drops every capability, forbids privilege escalation, and mounts a read-only root filesystem", func() {
			c := containerOf(objects["Deployment/shepherd-simulator"], "shepherd-simulator")
			sc, ok := c["securityContext"].(map[string]any)
			Expect(ok).To(BeTrue(), "container has no securityContext")
			Expect(sc["readOnlyRootFilesystem"]).To(Equal(true))
			Expect(sc["allowPrivilegeEscalation"]).To(Equal(false))
			caps, ok := sc["capabilities"].(map[string]any)
			Expect(ok).To(BeTrue(), "container securityContext has no capabilities block")
			Expect(caps["drop"]).To(ConsistOf("ALL"))
		})

		It("sets CPU and memory requests and limits", func() {
			c := containerOf(objects["Deployment/shepherd-simulator"], "shepherd-simulator")
			res, ok := c["resources"].(map[string]any)
			Expect(ok).To(BeTrue(), "container has no resources block")
			limits, ok := res["limits"].(map[string]any)
			Expect(ok).To(BeTrue(), "no resource limits set")
			Expect(limits).To(HaveKey("cpu"))
			Expect(limits).To(HaveKey("memory"))
			requests, ok := res["requests"].(map[string]any)
			Expect(ok).To(BeTrue(), "no resource requests set")
			Expect(requests).To(HaveKey("cpu"))
			Expect(requests).To(HaveKey("memory"))
		})

		It("default-denies egress except to its own harness ports — no DNS (D10)", func() {
			np, ok := objects["NetworkPolicy/shepherd-simulator"]
			Expect(ok).To(BeTrue(), "no NetworkPolicy rendered")

			spec, ok := np["spec"].(map[string]any)
			Expect(ok).To(BeTrue())
			policyTypes, ok := spec["policyTypes"].([]any)
			Expect(ok).To(BeTrue())
			Expect(policyTypes).To(ContainElement("Egress"))

			egress, ok := spec["egress"].([]any)
			Expect(ok).To(BeTrue(), "no egress rules at all — that is deny-ALL, not deny-by-default with a harness exception")
			Expect(egress).NotTo(BeEmpty())

			for _, raw := range egress {
				rule, ok := raw.(map[string]any)
				Expect(ok).To(BeTrue())
				// A rule with no "to" is Kubernetes' spelling of "every
				// destination" — the exact catch-all the main chart's own
				// NetworkPolicy uses (deploy/helm/shepherd/templates/networkpolicy.yaml)
				// and the one shape that would silently turn this into
				// allow-all-egress.
				Expect(rule).To(HaveKey("to"), "egress rule with no \"to\" allows every destination")
				to, ok := rule["to"].([]any)
				Expect(ok).To(BeTrue())
				Expect(to).NotTo(BeEmpty())

				for _, peerRaw := range to {
					peer, ok := peerRaw.(map[string]any)
					Expect(ok).To(BeTrue())
					// D10: every harness endpoint the sandboxed Alloy child
					// process needs is on THIS SAME POD (127.0.0.1) — nothing
					// it talks to lives in another namespace, so a
					// namespaceSelector peer here can only be the old
					// kube-system:53 DNS hole this decision removed.
					Expect(peer).NotTo(HaveKey("namespaceSelector"),
						"a namespaceSelector peer reaches outside this Pod's own namespace — D10 dropped "+
							"cluster DNS egress entirely, so no egress rule should still need one")
				}

				if ports, hasPorts := rule["ports"].([]any); hasPorts {
					for _, portRaw := range ports {
						port, ok := portRaw.(map[string]any)
						Expect(ok).To(BeTrue())
						Expect(port["port"]).NotTo(BeEquivalentTo(53),
							"port 53 (DNS) is still open — D10 dropped the sandbox's DNS egress entirely")
					}
				}
			}
		})

		It("points the sandboxed Alloy's harness endpoints at loopback, not the simulator Service's DNS name (D10)", func() {
			cfg := shepherdConfig(objects)
			sim, ok := cfg["simulator"].(map[string]any)
			Expect(ok).To(BeTrue(), "shepherd.yaml has no auto-wired simulator block")

			// control_url is dialled by SHEPHERD'S OWN Pod (a different
			// network namespace), which still needs the Service's DNS name —
			// only the sandboxed Alloy child process, which shares the
			// simulator Pod's own netns, can use loopback.
			Expect(sim["control_url"]).To(ContainSubstring("shepherd-simulator"),
				"control_url is dialled by Shepherd's own Pod and still needs the simulator Service's name")

			for key, want := range map[string]string{
				"capture_base_url":  "http://127.0.0.1:9110",
				"otlp_grpc_address": "127.0.0.1:4317",
				"syslog_host":       "127.0.0.1",
				"target_address":    "127.0.0.1:9111",
			} {
				Expect(sim[key]).To(Equal(want),
					"%s is read by the sandboxed Alloy child process INSIDE the simulator Pod's own "+
						"network namespace — it never needs to resolve the Service's DNS name, and D10 "+
						"removed the NetworkPolicy's only reason to keep cluster DNS open", key)
			}
		})

		It("restricts ingress to the main shepherd deployment on the control port", func() {
			np, ok := objects["NetworkPolicy/shepherd-simulator"]
			Expect(ok).To(BeTrue())
			spec, ok := np["spec"].(map[string]any)
			Expect(ok).To(BeTrue())
			ingress, ok := spec["ingress"].([]any)
			Expect(ok).To(BeTrue())
			Expect(ingress).To(HaveLen(1), "nothing but the control plane submits runs")
		})
	})
})

// tokenRefs reads back the Secret+key BOTH Deployments source the simulator's
// bearer token from: the simulator's own SIM_TOKEN and Shepherd's
// SHEPHERD_SIMULATOR_TOKEN. Whichever of D9's three sources supplied it, a
// mismatch here means the control API silently rejects every run the moment
// the two env vars point at different Secrets or keys.
func tokenRefs(objects map[string]map[string]any) (simSecret, simKey, appSecret, appKey string) {
	GinkgoHelper()
	simEnv := envOf(containerOf(objects["Deployment/shepherd-simulator"], "shepherd-simulator"))
	simEntry, ok := simEnv["SIM_TOKEN"].(map[string]any)
	Expect(ok).To(BeTrue(), "shepherd-simulator container has no SIM_TOKEN env var")
	simRef, ok := asMap(simEntry["valueFrom"])["secretKeyRef"].(map[string]any)
	Expect(ok).To(BeTrue(), "SIM_TOKEN is not sourced from a Secret")
	simSecret, _ = simRef["name"].(string) //nolint:errcheck // asserted via require below
	simKey, _ = simRef["key"].(string)     //nolint:errcheck // same
	Expect(simSecret).NotTo(BeEmpty())
	Expect(simKey).NotTo(BeEmpty())

	appEnv := envOf(containerOf(objects["Deployment/shepherd"], "shepherd"))
	appEntry, ok := appEnv["SHEPHERD_SIMULATOR_TOKEN"].(map[string]any)
	Expect(ok).To(BeTrue(), "shepherd container has no SHEPHERD_SIMULATOR_TOKEN env var")
	appRef, ok := asMap(appEntry["valueFrom"])["secretKeyRef"].(map[string]any)
	Expect(ok).To(BeTrue(), "SHEPHERD_SIMULATOR_TOKEN is not sourced from a Secret")
	appSecret, _ = appRef["name"].(string) //nolint:errcheck // asserted via require below
	appKey, _ = appRef["key"].(string)     //nolint:errcheck // same
	Expect(appSecret).NotTo(BeEmpty())
	Expect(appKey).NotTo(BeEmpty())
	return simSecret, simKey, appSecret, appKey
}

var _ = Describe("Helm chart: simulator bearer token (W4-S3, D9)", func() {
	It("source: simulator.token.existingSecret — both Deployments read it, nothing else renders", func() {
		objects := renderWith("simulator:\n  token:\n    existingSecret: my-sim-token\n    key: bearer\n")

		simSecret, simKey, appSecret, appKey := tokenRefs(objects)
		Expect(simSecret).To(Equal("my-sim-token"))
		Expect(simKey).To(Equal("bearer"))
		Expect(appSecret).To(Equal("my-sim-token"))
		Expect(appKey).To(Equal("bearer"))

		Expect(objects).NotTo(HaveKey("Secret/shepherd-simulator-token"),
			"existingSecret set: the chart must not ALSO generate its own token Secret")
		Expect(objects).NotTo(HaveKey("ExternalSecret/shepherd-simulator-token"),
			"existingSecret set: the chart must not ALSO render an ExternalSecret for the token")
	})

	It("source: External Secrets — both Deployments read the same Secret an ExternalSecret targets", func() {
		objects := renderWith("externalSecrets:\n  enabled: true\ncnpg:\n  enabled: true\n")

		simSecret, simKey, appSecret, appKey := tokenRefs(objects)
		Expect(simSecret).To(Equal(appSecret), "the two Deployments must reference the same Secret")
		Expect(simKey).To(Equal(appKey), "the two Deployments must reference the same key")

		es, ok := objects["ExternalSecret/"+simSecret]
		Expect(ok).To(BeTrue(), "externalSecrets.enabled must render an ExternalSecret for the simulator token")
		target, ok := asMap(asMap(es["spec"])["target"])["name"].(string)
		Expect(ok).To(BeTrue())
		Expect(target).To(Equal(simSecret), "the ExternalSecret must target the same Secret name both Deployments read")

		Expect(objects).NotTo(HaveKey("Secret/"+simSecret),
			"externalSecrets.enabled: the chart must not ALSO generate its own token Secret")
	})

	It("source: chart-generated (neither existingSecret nor External Secrets) — lookup-reuse Secret both Deployments read", func() {
		objects := renderWith("")

		simSecret, simKey, appSecret, appKey := tokenRefs(objects)
		Expect(simSecret).To(Equal(appSecret))
		Expect(simKey).To(Equal(appKey))

		secret, ok := objects["Secret/"+simSecret]
		Expect(ok).To(BeTrue(), "neither existingSecret nor externalSecrets.enabled: the chart must generate its own token Secret")
		value, ok := asMap(secret["stringData"])[simKey].(string)
		Expect(ok).To(BeTrue(), "generated Secret has no %q key", simKey)
		Expect(value).NotTo(BeEmpty(), "the generated token must not be empty")

		Expect(objects).NotTo(HaveKey("ExternalSecret/"+simSecret),
			"neither existingSecret nor externalSecrets.enabled: no ExternalSecret should render for the token")
	})

	It("fails the render when an explicit config.simulator block carries no token", func() {
		out := renderFailure("config:\n  simulator:\n    enabled: true\n    control_url: \"http://external-sim:8099\"\n")
		Expect(out).To(ContainSubstring("token"),
			"the failure must name the missing token, not fail for an unrelated reason:\n%s", out)
	})
})

func podSpecOf(deployment map[string]any) map[string]any {
	spec, _ := deployment["spec"].(map[string]any)   //nolint:errcheck // caller asserts presence via the returned value
	template, _ := spec["template"].(map[string]any) //nolint:errcheck // same
	podSpec, _ := template["spec"].(map[string]any)  //nolint:errcheck // same
	ExpectWithOffset(1, podSpec).NotTo(BeNil(), "Deployment has no pod spec: %v", deployment)
	return podSpec
}

func containerOf(deployment map[string]any, name string) map[string]any {
	podSpec := podSpecOf(deployment)
	containers, _ := podSpec["containers"].([]any) //nolint:errcheck // asserted below
	for _, raw := range containers {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if c["name"] == name {
			return c
		}
	}
	ExpectWithOffset(1, false).To(BeTrue(), "no container named %q in pod spec %v", name, podSpec)
	return nil
}
