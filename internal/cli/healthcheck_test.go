package cli

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func healthzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

// TestRunHealthcheckCleartextUnchanged locks in that default behaviour
// (no --tls) is unaffected by TLS support existing at all.
func TestRunHealthcheckCleartextUnchanged(t *testing.T) {
	ts := httptest.NewServer(healthzHandler())
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	if err := runHealthcheck(context.Background(), healthcheckOptions{addr: addr}); err != nil {
		t.Fatalf("runHealthcheck() = %v, want nil", err)
	}
}

func TestRunHealthcheckTLS(t *testing.T) {
	ts := httptest.NewTLSServer(healthzHandler())
	defer ts.Close()
	addr := strings.TrimPrefix(ts.URL, "https://")

	t.Run("--insecure-skip-verify", func(t *testing.T) {
		err := runHealthcheck(context.Background(), healthcheckOptions{
			addr: addr, tls: true, tlsExplicit: true, insecureSkipVerify: true,
		})
		if err != nil {
			t.Fatalf("runHealthcheck() = %v, want nil", err)
		}
	})

	t.Run("without skip-verify or ca-file fails closed", func(t *testing.T) {
		err := runHealthcheck(context.Background(), healthcheckOptions{
			addr: addr, tls: true, tlsExplicit: true,
		})
		if err == nil {
			t.Fatal("runHealthcheck() = nil, want a certificate verification error")
		}
	})

	t.Run("--ca-file trusts the server's certificate", func(t *testing.T) {
		caFile := writeTempPEM(t, ts.Certificate())
		err := runHealthcheck(context.Background(), healthcheckOptions{
			addr: addr, tls: true, tlsExplicit: true, caFile: caFile,
		})
		if err != nil {
			t.Fatalf("runHealthcheck() with --ca-file = %v, want nil", err)
		}
	})
}

// TestRunHealthcheckTLSFromEnv confirms SHEPHERD_SERVER_TLS_* auto-enables
// --tls when the flag was left at its default, and that an explicit
// --tls=false still overrides the environment.
func TestRunHealthcheckTLSFromEnv(t *testing.T) {
	ts := httptest.NewTLSServer(healthzHandler())
	defer ts.Close()
	addr := strings.TrimPrefix(ts.URL, "https://")
	caFile := writeTempPEM(t, ts.Certificate())

	t.Setenv("SHEPHERD_SERVER_TLS_CERT_FILE", "/etc/shepherd/tls/tls.crt")
	t.Setenv("SHEPHERD_SERVER_TLS_KEY_FILE", "/etc/shepherd/tls/tls.key")

	t.Run("env enables TLS when --tls was not passed", func(t *testing.T) {
		err := runHealthcheck(context.Background(), healthcheckOptions{addr: addr, caFile: caFile})
		if err != nil {
			t.Fatalf("runHealthcheck() = %v, want nil (env should have enabled TLS)", err)
		}
	})

	t.Run("explicit --tls=false overrides the environment", func(t *testing.T) {
		err := runHealthcheck(context.Background(), healthcheckOptions{addr: addr, tls: false, tlsExplicit: true, caFile: caFile})
		if err == nil {
			t.Fatal("runHealthcheck() = nil, want an error (plain HTTP GET against a TLS-only listener)")
		}
	})
}

func writeTempPEM(t *testing.T, cert *x509.Certificate) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "ca-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // test cleanup
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}
