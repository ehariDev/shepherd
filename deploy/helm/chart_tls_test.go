// Task §5.6/§6: render TLS on/off, cert-manager, and the probes through the
// real `helm template` — the same "read assertions back out of actual helm
// output" discipline the rest of this package follows, not a hand-parsed
// approximation of what the templates are supposed to produce.
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

// renderTLSWith mirrors chart_observability_test.go's renderWith (own copy,
// per this package's "each file renders independently" convention) —
// every rendered object keyed by "Kind/Name".
func renderTLSWith(values string) map[string]map[string]any {
	GinkgoHelper()
	dir := GinkgoT().TempDir()
	overridePath := filepath.Join(dir, "override.yaml")
	Expect(os.WriteFile(overridePath, []byte(values), 0o600)).To(Succeed())

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
		kind, _ := doc["kind"].(string)             //nolint:errcheck // rendered by helm, shape is known
		meta, _ := doc["metadata"].(map[string]any) //nolint:errcheck // same
		name, _ := meta["name"].(string)            //nolint:errcheck // same
		if kind == "" || name == "" {
			continue
		}
		objects[kind+"/"+name] = doc
	}
	return objects
}

func containerPorts(dep map[string]any) []any {
	spec, _ := dep["spec"].(map[string]any)                //nolint:errcheck // rendered by helm, shape is known
	tmpl, _ := spec["template"].(map[string]any)           //nolint:errcheck // same
	podSpec, _ := tmpl["spec"].(map[string]any)            //nolint:errcheck // same
	containers, _ := podSpec["containers"].([]any)         //nolint:errcheck // same
	shepherdContainer, _ := containers[0].(map[string]any) //nolint:errcheck // same
	ports, _ := shepherdContainer["ports"].([]any)         //nolint:errcheck // same
	return ports
}

func shepherdContainerOf(dep map[string]any) map[string]any {
	spec, _ := dep["spec"].(map[string]any)        //nolint:errcheck // rendered by helm, shape is known
	tmpl, _ := spec["template"].(map[string]any)   //nolint:errcheck // same
	podSpec, _ := tmpl["spec"].(map[string]any)    //nolint:errcheck // same
	containers, _ := podSpec["containers"].([]any) //nolint:errcheck // same
	c, _ := containers[0].(map[string]any)         //nolint:errcheck // same
	return c
}

var _ = Describe("Helm chart: TLS off (default)", func() {
	It("keeps the deployment byte-identical to the pre-TLS shape", func() {
		objs := renderTLSWith("")
		dep := objs["Deployment/shepherd"]
		Expect(dep).NotTo(BeNil())

		ports := containerPorts(dep)
		first, _ := ports[0].(map[string]any) //nolint:errcheck // asserted below
		Expect(first["name"]).To(Equal("http"))
		Expect(first["containerPort"]).To(BeNumerically("==", 8080))

		container := shepherdContainerOf(dep)
		liveness, _ := container["livenessProbe"].(map[string]any) //nolint:errcheck // asserted below
		httpGet, _ := liveness["httpGet"].(map[string]any)         //nolint:errcheck // same
		Expect(httpGet).NotTo(HaveKey("scheme"))
		Expect(httpGet["port"]).To(Equal("http"))

		volumes, _ := container["volumeMounts"].([]any) //nolint:errcheck // asserted below
		for _, v := range volumes {
			vm, _ := v.(map[string]any) //nolint:errcheck // same
			Expect(vm["name"]).NotTo(Equal("tls"), "no TLS volume mount should exist when tls.enabled is unset")
		}
	})

	It("does not render a cert-manager Certificate", func() {
		objs := renderTLSWith("")
		Expect(objs).NotTo(HaveKey("Certificate/shepherd"))
	})
})

var _ = Describe("Helm chart: TLS on with an existing secret", func() {
	values := "tls:\n  enabled: true\n  secretName: my-tls-secret\n"

	It("switches the container/probe/service to https on tls.port", func() {
		objs := renderTLSWith(values)
		dep := objs["Deployment/shepherd"]
		Expect(dep).NotTo(BeNil())

		ports := containerPorts(dep)
		first, _ := ports[0].(map[string]any) //nolint:errcheck // asserted below
		Expect(first["name"]).To(Equal("https"))
		Expect(first["containerPort"]).To(BeNumerically("==", 8443))

		container := shepherdContainerOf(dep)
		liveness, _ := container["livenessProbe"].(map[string]any) //nolint:errcheck // asserted below
		httpGet, _ := liveness["httpGet"].(map[string]any)         //nolint:errcheck // same
		Expect(httpGet["scheme"]).To(Equal("HTTPS"))
		Expect(httpGet["port"]).To(Equal("https"))

		svc := objs["Service/shepherd"]
		spec, _ := svc["spec"].(map[string]any)    //nolint:errcheck // asserted below
		svcPorts, _ := spec["ports"].([]any)       //nolint:errcheck // same
		svcPort, _ := svcPorts[0].(map[string]any) //nolint:errcheck // same
		Expect(svcPort["name"]).To(Equal("https"))
		Expect(svcPort["targetPort"]).To(Equal("https"))
		Expect(svcPort["appProtocol"]).To(Equal("https"))
	})

	It("mounts secretName read-only at /etc/shepherd/tls", func() {
		objs := renderTLSWith(values)
		dep := objs["Deployment/shepherd"]
		spec, _ := dep["spec"].(map[string]any)      //nolint:errcheck // asserted below
		tmpl, _ := spec["template"].(map[string]any) //nolint:errcheck // same
		podSpec, _ := tmpl["spec"].(map[string]any)  //nolint:errcheck // same
		volumes, _ := podSpec["volumes"].([]any)     //nolint:errcheck // same

		var tlsVolume map[string]any
		for _, v := range volumes {
			vol, _ := v.(map[string]any) //nolint:errcheck // same
			if vol["name"] == "tls" {
				tlsVolume = vol
			}
		}
		Expect(tlsVolume).NotTo(BeNil(), "expected a \"tls\" volume")
		secret, _ := tlsVolume["secret"].(map[string]any) //nolint:errcheck // asserted below
		Expect(secret["secretName"]).To(Equal("my-tls-secret"))
	})

	It("auto-wires config.server.tls unless the operator set it explicitly", func() {
		objs := renderTLSWith(values)
		cm := objs["ConfigMap/shepherd"]
		data, _ := cm["data"].(map[string]any) //nolint:errcheck // asserted below
		var parsed map[string]any
		Expect(yaml.Unmarshal([]byte(data["shepherd.yaml"].(string)), &parsed)).To(Succeed()) //nolint:errcheck,forcetypeassert // asserted immediately after
		server, _ := parsed["server"].(map[string]any)                                        //nolint:errcheck // same
		Expect(server["listen"]).To(Equal(":8443"))
		tlsCfg, _ := server["tls"].(map[string]any) //nolint:errcheck // same
		Expect(tlsCfg["cert_file"]).To(Equal("/etc/shepherd/tls/tls.crt"))
		Expect(tlsCfg["key_file"]).To(Equal("/etc/shepherd/tls/tls.key"))
		Expect(tlsCfg["min_version"]).To(Equal("1.2"))
	})

	It("respects an explicit config.server.tls block verbatim, including a custom config.server.listen", func() {
		explicit := values + "config:\n  server:\n    listen: \":9999\"\n    tls:\n      cert_file: /custom/cert.pem\n      key_file: /custom/key.pem\n"
		objs := renderTLSWith(explicit)
		cm := objs["ConfigMap/shepherd"]
		data, _ := cm["data"].(map[string]any) //nolint:errcheck // asserted below
		var parsed map[string]any
		Expect(yaml.Unmarshal([]byte(data["shepherd.yaml"].(string)), &parsed)).To(Succeed()) //nolint:errcheck,forcetypeassert // asserted immediately after
		server, _ := parsed["server"].(map[string]any)                                        //nolint:errcheck // same
		tlsCfg, _ := server["tls"].(map[string]any)                                           //nolint:errcheck // same
		Expect(tlsCfg["cert_file"]).To(Equal("/custom/cert.pem"))
		Expect(tlsCfg).NotTo(HaveKey("min_version"), "an explicit config.server.tls block must win verbatim, with no chart-injected keys added")
	})
})

var _ = Describe("Helm chart: TLS collector CA", func() {
	It("mounts a second volume and wires collector_ca_file", func() {
		values := "tls:\n  enabled: true\n  secretName: my-tls-secret\n  collectorCA:\n    secretName: my-ca-secret\n"
		objs := renderTLSWith(values)

		dep := objs["Deployment/shepherd"]
		container := shepherdContainerOf(dep)
		mounts, _ := container["volumeMounts"].([]any) //nolint:errcheck // asserted below
		found := false
		for _, m := range mounts {
			vm, _ := m.(map[string]any) //nolint:errcheck // same
			if vm["name"] == "tls-ca" {
				found = true
				Expect(vm["mountPath"]).To(Equal("/etc/shepherd/tls-ca"))
			}
		}
		Expect(found).To(BeTrue(), "expected a tls-ca volumeMount")

		cm := objs["ConfigMap/shepherd"]
		data, _ := cm["data"].(map[string]any) //nolint:errcheck // asserted below
		var parsed map[string]any
		Expect(yaml.Unmarshal([]byte(data["shepherd.yaml"].(string)), &parsed)).To(Succeed()) //nolint:errcheck,forcetypeassert // asserted immediately after
		server, _ := parsed["server"].(map[string]any)                                        //nolint:errcheck // same
		tlsCfg, _ := server["tls"].(map[string]any)                                           //nolint:errcheck // same
		Expect(tlsCfg["collector_ca_file"]).To(Equal("/etc/shepherd/tls-ca/ca.crt"))
	})
})

var _ = Describe("Helm chart: TLS via cert-manager", func() {
	values := "tls:\n  enabled: true\n  certManager:\n    enabled: true\n    issuerRef:\n      name: letsencrypt\n    dnsNames: [\"shepherd.example.internal\"]\n"

	It("renders a Certificate requesting the chart-default secretName", func() {
		objs := renderTLSWith(values)
		cert := objs["Certificate/shepherd"]
		Expect(cert).NotTo(BeNil())
		spec, _ := cert["spec"].(map[string]any) //nolint:errcheck // asserted below
		Expect(spec["secretName"]).To(Equal("shepherd-tls"))
		issuerRef, _ := spec["issuerRef"].(map[string]any) //nolint:errcheck // same
		Expect(issuerRef["name"]).To(Equal("letsencrypt"))
		Expect(issuerRef["kind"]).To(Equal("ClusterIssuer"))
		dnsNames, _ := spec["dnsNames"].([]any) //nolint:errcheck // same
		Expect(dnsNames).To(ConsistOf("shepherd.example.internal"))
	})

	It("mounts the same secretName the Certificate requests", func() {
		objs := renderTLSWith(values)
		dep := objs["Deployment/shepherd"]
		spec, _ := dep["spec"].(map[string]any)      //nolint:errcheck // asserted below
		tmpl, _ := spec["template"].(map[string]any) //nolint:errcheck // same
		podSpec, _ := tmpl["spec"].(map[string]any)  //nolint:errcheck // same
		volumes, _ := podSpec["volumes"].([]any)     //nolint:errcheck // same
		var tlsVolume map[string]any
		for _, v := range volumes {
			vol, _ := v.(map[string]any) //nolint:errcheck // same
			if vol["name"] == "tls" {
				tlsVolume = vol
			}
		}
		secret, _ := tlsVolume["secret"].(map[string]any) //nolint:errcheck // asserted below
		Expect(secret["secretName"]).To(Equal("shepherd-tls"))
	})
})

var _ = Describe("Helm chart: TLS fail-fast validation", func() {
	It("refuses tls.enabled with neither secretName nor certManager.enabled", func() {
		out := renderFailure("tls:\n  enabled: true\n")
		Expect(out).To(ContainSubstring("neither tls.secretName nor tls.certManager.enabled"))
	})

	It("refuses certManager.enabled with no issuerRef.name", func() {
		out := renderFailure("tls:\n  enabled: true\n  certManager:\n    enabled: true\n    dnsNames: [\"shepherd.example.internal\"]\n")
		Expect(out).To(ContainSubstring("issuerRef.name is empty"))
	})

	It("refuses certManager.enabled with no dnsNames", func() {
		out := renderFailure("tls:\n  enabled: true\n  certManager:\n    enabled: true\n    issuerRef:\n      name: letsencrypt\n")
		Expect(out).To(ContainSubstring("dnsNames is empty"))
	})
})
