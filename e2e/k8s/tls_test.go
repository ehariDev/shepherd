//go:build e2ek8s

package k8s_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/pkg/utils"
)

// tlsIssuerName is a cluster-scoped, self-signed ClusterIssuer created once
// alongside the cert-manager operator. Self-signed rather than an ACME
// account: this suite proves Shepherd's OWN TLS plumbing (Certificate ->
// Secret -> volume mount -> HTTPS listener), not cert-manager's ability to
// talk to a real CA, which is cert-manager's own test suite's job.
const tlsIssuerName = "shepherd-e2e-selfsigned"

// installCertManagerOperator installs cert-manager, pinned from
// deploy/versions.env the same way CNPG/ESO are (chart_deps_test.go), plus
// the shared ClusterIssuer every TLS feature in this file uses.
func installCertManagerOperator(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	version, err := readVersionsEnvValue("CERT_MANAGER_CHART_VERSION")
	if err != nil {
		return ctx, err
	}
	log.Printf("installing cert-manager %s (TLS operator under test)", version)
	cmd := fmt.Sprintf(
		"helm install cert-manager oci://quay.io/jetstack/charts/cert-manager --version %s --kubeconfig %s -n cert-manager --create-namespace --set crds.enabled=true --wait --timeout %s",
		version, cfg.KubeconfigFile(), operatorInstallTimeout,
	)
	if p := utils.RunCommand(cmd); p.Err() != nil {
		return ctx, fmt.Errorf("installing cert-manager %s: %w: %s", version, p.Err(), p.Result())
	}

	issuerYAML := fmt.Sprintf(`apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: %s
spec:
  selfSigned: {}
`, tlsIssuerName)
	dir, err := os.MkdirTemp("", "shepherd-e2e-tls-*")
	if err != nil {
		return ctx, fmt.Errorf("creating temp dir for ClusterIssuer manifest: %w", err)
	}
	defer os.RemoveAll(dir) //nolint:errcheck // best-effort temp dir cleanup
	issuerPath := filepath.Join(dir, "issuer.yaml")
	if err := os.WriteFile(issuerPath, []byte(issuerYAML), 0o600); err != nil {
		return ctx, fmt.Errorf("writing ClusterIssuer manifest: %w", err)
	}
	if p := utils.RunCommand(fmt.Sprintf("kubectl --kubeconfig %s apply -f %s", cfg.KubeconfigFile(), issuerPath)); p.Err() != nil {
		return ctx, fmt.Errorf("applying ClusterIssuer: %w: %s", p.Err(), p.Result())
	}
	return ctx, nil
}

// TestChartTLSWithCertManager is what `helm template` cannot prove: that the
// chart's tls.certManager path actually issues a certificate the migration
// Job and the Deployment can both mount, that the Deployment reaches Ready
// through its HTTPS probes (ALPN-negotiated HTTP/2, not h2c), and that a
// real client gets a real HTTPS response through the Service -- not just the
// Pod directly, the same "through the Service, the selector is what charts
// get wrong" discipline TestHelmChartInstalls already follows.
//
// This exact chain caught a real bug during development, not in this
// suite (agents do not run make e2e-k8s -- AGENTS.md) but by hand against a
// kind cluster: the migration Job shares its ConfigMap body with the runtime
// Deployment by design, so once tls.enabled it also named
// /etc/shepherd/tls/tls.crt -- but had no TLS volume mounted, so it failed
// before ever reaching the database. Fixed in migrate-job.yaml and
// certificate.yaml (hook ordering ahead of the migrate Job's hook-weight).
// This spec is what should have caught it first.
func TestChartTLSWithCertManager(t *testing.T) {
	var f *fixture
	const release = "shepherd"
	const tlsPort = 8443

	feat := features.New("chart TLS via cert-manager").
		WithLabel("suite", "tls").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			f = newFixture(ctx, t, cfg, "tls")
			return ctx
		}).
		Assess("helm install succeeds with tls.enabled via cert-manager",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				dnsName := fmt.Sprintf("%s.%s.svc.cluster.local", release, f.ns)
				helmRun(t, cfg, f, "install", release,
					"--set simulator.enabled=false",
					"--set replicas=1",
					"--set service.port="+fmt.Sprint(tlsPort),
					"--set tls.enabled=true",
					"--set tls.port="+fmt.Sprint(tlsPort),
					"--set tls.certManager.enabled=true",
					"--set tls.certManager.issuerRef.name="+tlsIssuerName,
					"--set tls.certManager.issuerRef.kind=ClusterIssuer",
					"--set tls.certManager.dnsNames[0]="+dnsName,
				)
				return ctx
			}).
		Assess("migrations were actually applied to the database",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				// Same reasoning as TestHelmChartInstalls: this is the assertion
				// that caught the migrate-job.yaml bug above. "helm install
				// succeeded" only means the hook exited 0.
				n, raw := f.migrationCount(t, cfg)
				if n < 1 {
					t.Fatalf("expected at least one applied migration, psql said %q\n%s",
						raw, describeNS(cfg, f.ns))
				}
				return ctx
			}).
		Assess("cert-manager issued a Ready Certificate",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				p := utils.RunCommand(fmt.Sprintf(
					"kubectl --kubeconfig %s -n %s get certificate %s -o jsonpath={.status.conditions[?(@.type==\"Ready\")].status}",
					cfg.KubeconfigFile(), f.ns, release,
				))
				if strings.TrimSpace(p.Result()) != "True" {
					t.Fatalf("Certificate/%s never went Ready: %q\n%s", release, p.Result(), describeNS(cfg, f.ns))
				}
				return ctx
			}).
		Assess("the shepherd Deployment becomes Available (its liveness/readiness probes are HTTPS)",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				waitDeploymentAvailable(t, cfg, f.ns, release)
				return ctx
			}).
		Assess("shepherd healthcheck --tls succeeds against the running pod",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				// Execs the real binary inside the real pod -- no shell needed
				// (distroless has none), just the one static binary the image
				// already runs.
				p := utils.RunCommand(fmt.Sprintf(
					"kubectl --kubeconfig %s -n %s exec deploy/%s -- /usr/local/bin/shepherd healthcheck --tls --ca-file /etc/shepherd/tls/tls.crt --addr localhost:%d",
					cfg.KubeconfigFile(), f.ns, release, tlsPort,
				))
				if p.Err() != nil {
					t.Fatalf("shepherd healthcheck --tls failed inside the pod: %v: %s\n%s",
						p.Err(), p.Result(), describeNS(cfg, f.ns))
				}
				return ctx
			}).
		Assess("shepherd answers HTTPS with HTTP/2 THROUGH the Service, trusting the issued certificate",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				caPEM := fetchSecretKey(t, cfg, f.ns, release+"-tls", "tls.crt")
				httpCode, httpVersion := curlHTTPSIn(t, cfg, f.ns, "tls-verify",
					fmt.Sprintf("https://%s.%s.svc.cluster.local:%d/healthz", release, f.ns, tlsPort),
					caPEM)
				if httpCode != "200" {
					t.Fatalf("GET /healthz through the Service = http_code %q, want 200\n%s", httpCode, describeNS(cfg, f.ns))
				}
				if httpVersion != "2" {
					t.Fatalf("GET /healthz through the Service negotiated HTTP/%s, want HTTP/2 (ALPN) -- "+
						"the h2c-to-ALPN switch is the task's own highest-risk regression\n%s",
						httpVersion, describeNS(cfg, f.ns))
				}
				return ctx
			}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			f.cleanup(cfg)
			return ctx
		}).
		Feature()

	testenv.Test(t, feat)
}

// fetchSecretKey reads one key of a Secret's data through the typed client
// (which decodes the wire-format base64 for us) and fails the test if the
// Secret or key is missing.
func fetchSecretKey(t *testing.T, cfg *envconf.Config, ns, name, key string) []byte {
	t.Helper()
	var sec corev1.Secret
	if err := cfg.Client().Resources().Get(context.Background(), name, ns, &sec); err != nil {
		t.Fatalf("fetching secret %s/%s: %v", ns, name, err)
	}
	v, ok := sec.Data[key]
	if !ok {
		// keysOf is defined in chart_deps_test.go, same package.
		t.Fatalf("secret %s/%s has no key %q (keys present: %v)", ns, name, key, keysOf(sec.Data))
	}
	return v
}

// curlHTTPSIn runs a one-shot curl Pod (applied from a temp-file manifest --
// the same pattern installCertManagerOperator uses for the ClusterIssuer,
// chosen over `kubectl run --overrides` because quoting a JSON overrides
// blob inside a shell command string is exactly the kind of thing that
// looks fine until it silently breaks) that trusts caPEM via a mounted
// ConfigMap and requests url, returning the response's HTTP status code and
// negotiated protocol version ("1.1" or "2"). A real client hitting the
// Service, not the Deployment's own probes or an in-pod exec -- the same
// "through the Service" discipline TestHelmChartInstalls uses for
// cleartext.
func curlHTTPSIn(t *testing.T, cfg *envconf.Config, ns, name, url string, caPEM []byte) (httpCode, httpVersion string) {
	t.Helper()
	ctx := context.Background()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-ca", Namespace: ns},
		Data:       map[string]string{"ca.crt": string(caPEM)},
	}
	if err := cfg.Client().Resources().Create(ctx, cm); err != nil {
		t.Fatalf("creating CA ConfigMap: %v", err)
	}
	defer func() {
		_ = cfg.Client().Resources().Delete(context.Background(), cm) //nolint:errcheck // best-effort cleanup
	}()

	podYAML := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
spec:
  restartPolicy: Never
  volumes:
    - name: ca
      configMap:
        name: %s-ca
  containers:
    - name: curl
      image: curlimages/curl:8.11.1
      command: ["curl", "-s", "--cacert", "/ca/ca.crt", "-o", "/dev/null",
                "-w", "http_code=%%{http_code} http_version=%%{http_version}", %q]
      volumeMounts:
        - name: ca
          mountPath: /ca
`, name, ns, name, url)
	dir, err := os.MkdirTemp("", "shepherd-e2e-curl-*")
	if err != nil {
		t.Fatalf("creating temp dir for curl pod manifest: %v", err)
	}
	defer os.RemoveAll(dir) //nolint:errcheck // best-effort temp dir cleanup
	podPath := filepath.Join(dir, "pod.yaml")
	if err := os.WriteFile(podPath, []byte(podYAML), 0o600); err != nil {
		t.Fatalf("writing curl pod manifest: %v", err)
	}
	defer func() {
		_ = utils.RunCommand(fmt.Sprintf("kubectl --kubeconfig %s -n %s delete pod %s --ignore-not-found --wait=false",
			cfg.KubeconfigFile(), ns, name))
	}()

	deadline := time.Now().Add(connectDeadline)
	var lastOut string
	for time.Now().Before(deadline) {
		_ = utils.RunCommand(fmt.Sprintf("kubectl --kubeconfig %s -n %s delete pod %s --ignore-not-found --wait=true --timeout=30s",
			cfg.KubeconfigFile(), ns, name))
		if p := utils.RunCommand(fmt.Sprintf("kubectl --kubeconfig %s apply -f %s", cfg.KubeconfigFile(), podPath)); p.Err() != nil {
			lastOut = "apply failed: " + p.Result()
			time.Sleep(5 * time.Second)
			continue
		}
		// Poll phase rather than `kubectl wait --for=condition=Ready`: this Pod
		// is restartPolicy Never and expected to EXIT, so Ready never becomes
		// (and should never become) true -- Succeeded/Failed is the completion
		// signal here, the same distinction dialIn's "Job, not a long-running
		// Deployment" reasoning elsewhere in this package relies on.
		phaseDeadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(phaseDeadline) {
			phase := utils.RunCommand(fmt.Sprintf("kubectl --kubeconfig %s -n %s get pod %s -o jsonpath={.status.phase}",
				cfg.KubeconfigFile(), ns, name))
			ph := strings.TrimSpace(phase.Result())
			if ph == "Succeeded" || ph == "Failed" {
				break
			}
			time.Sleep(2 * time.Second)
		}
		logs := utils.RunCommand(fmt.Sprintf("kubectl --kubeconfig %s -n %s logs %s", cfg.KubeconfigFile(), ns, name))
		lastOut = strings.TrimSpace(logs.Result())
		if strings.Contains(lastOut, "http_code=200") {
			break
		}
		time.Sleep(5 * time.Second)
	}

	httpCode = fieldAfter(lastOut, "http_code=")
	httpVersion = fieldAfter(lastOut, "http_version=")
	return httpCode, httpVersion
}

// fieldAfter extracts the whitespace-delimited token following prefix in s,
// e.g. fieldAfter("http_code=200 http_version=2", "http_version=") == "2".
func fieldAfter(s, prefix string) string {
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}
	rest := s[i+len(prefix):]
	if sp := strings.IndexByte(rest, ' '); sp >= 0 {
		return rest[:sp]
	}
	return rest
}
