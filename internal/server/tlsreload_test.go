package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// generateCert builds a self-signed EC certificate in-process — no fixtures
// checked in, per §5.2's test requirement. serial distinguishes one
// generated certificate from another so a test can confirm which one a
// reloader ended up serving.
func generateCert(t *testing.T, serial int64, notAfter time.Time) (certPEM, keyPEM []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "shepherd-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// IP/DNS SANs so tls_integration_test.go's real listener test (dialed
		// as 127.0.0.1) validates without InsecureSkipVerify. Harmless for
		// every other test here, which only exercises certReloader directly.
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func writeCertFiles(t *testing.T, dir string, serial int64, notAfter time.Time) (certFile, keyFile string) {
	t.Helper()
	certPEM, keyPEM := generateCert(t, serial, notAfter)
	certFile = filepath.Join(dir, "tls.crt")
	keyFile = filepath.Join(dir, "tls.key")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

// mustGet fetches the reloader's current certificate, failing the test on
// error rather than discarding it — Get only errors when nothing has ever
// loaded successfully, which every test here rules out before calling this.
func mustGet(t *testing.T, r *certReloader) *tls.Certificate {
	t.Helper()
	cert, err := r.Get(nil)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func leafSerial(t *testing.T, cert *tls.Certificate) int64 {
	t.Helper()
	if cert.Leaf != nil {
		return cert.Leaf.SerialNumber.Int64()
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf.SerialNumber.Int64()
}

func TestCertReloaderInitialLoad(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertFiles(t, dir, 1, time.Now().Add(time.Hour))

	r, err := newCertReloader(certFile, keyFile, "")
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}
	cert, err := r.Get(nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := leafSerial(t, cert); got != 1 {
		t.Fatalf("serial = %d, want 1", got)
	}
}

func TestCertReloaderRotationViaPolling(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertFiles(t, dir, 1, time.Now().Add(time.Hour))

	r, err := newCertReloader(certFile, keyFile, "")
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go r.Run(ctx, 20*time.Millisecond, logger)

	// Ensure the new files get a distinguishable mtime from the initial
	// write before rewriting — some filesystems have coarse mtime
	// resolution.
	time.Sleep(30 * time.Millisecond)
	writeCertFiles(t, dir, 2, time.Now().Add(time.Hour))

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cert, err := r.Get(nil)
		if err == nil && leafSerial(t, cert) == 2 {
			return // rotation observed
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("polling did not pick up the rotated certificate (serial 2) within the deadline")
}

func TestCertReloaderPollDisabledBySIGHUPStillWorks(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertFiles(t, dir, 1, time.Now().Add(time.Hour))

	r, err := newCertReloader(certFile, keyFile, "")
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx, 0, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()

	writeCertFiles(t, dir, 2, time.Now().Add(time.Hour))
	time.Sleep(100 * time.Millisecond)
	if leafSerial(t, mustGet(t, r)) != 1 {
		t.Fatal("reload_interval=0 polled anyway; it should only reload via a direct Reload() call (SIGHUP)")
	}

	// The SIGHUP path calls Reload() directly, bypassing polling entirely.
	if err := r.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if leafSerial(t, mustGet(t, r)) != 2 {
		t.Fatal("direct Reload() (the SIGHUP path) did not pick up the rewritten certificate")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancellation with polling disabled")
	}
}

func TestCertReloaderCorruptFileKeepsOldCertificate(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertFiles(t, dir, 1, time.Now().Add(time.Hour))

	r, err := newCertReloader(certFile, keyFile, "")
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}

	if err := os.WriteFile(certFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Reload(); err == nil {
		t.Fatal("Reload with a corrupt cert file returned nil error, want an error")
	}
	cert, err := r.Get(nil)
	if err != nil {
		t.Fatalf("Get after failed reload: %v", err)
	}
	if got := leafSerial(t, cert); got != 1 {
		t.Fatalf("serial after failed reload = %d, want 1 (the last good certificate)", got)
	}
}

func TestCertReloaderExpiredCertificateRejected(t *testing.T) {
	dir := t.TempDir()

	t.Run("initial load", func(t *testing.T) {
		certFile, keyFile := writeCertFiles(t, dir, 1, time.Now().Add(-time.Minute))
		if _, err := newCertReloader(certFile, keyFile, ""); err == nil {
			t.Fatal("newCertReloader with an already-expired certificate returned nil error")
		}
	})

	t.Run("reload keeps the last good certificate", func(t *testing.T) {
		freshDir := t.TempDir()
		certFile, keyFile := writeCertFiles(t, freshDir, 1, time.Now().Add(time.Hour))
		r, err := newCertReloader(certFile, keyFile, "")
		if err != nil {
			t.Fatalf("newCertReloader: %v", err)
		}
		writeCertFiles(t, freshDir, 2, time.Now().Add(-time.Minute))
		if err := r.Reload(); err == nil {
			t.Fatal("Reload with an expired certificate returned nil error")
		}
		if leafSerial(t, mustGet(t, r)) != 1 {
			t.Fatal("an expired reload replaced the last good certificate")
		}
	})
}

func TestCertReloaderKeyCertMismatchRejected(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertFiles(t, dir, 1, time.Now().Add(time.Hour))
	r, err := newCertReloader(certFile, keyFile, "")
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}

	// Swap in a key from an unrelated keypair — cert stays as-is.
	_, mismatchedKeyPEM := generateCert(t, 2, time.Now().Add(time.Hour))
	if err := os.WriteFile(keyFile, mismatchedKeyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Reload(); err == nil {
		t.Fatal("Reload with a mismatched key returned nil error")
	}
	if leafSerial(t, mustGet(t, r)) != 1 {
		t.Fatal("a mismatched-key reload replaced the last good certificate")
	}
}

func TestCertReloaderClientCARotates(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertFiles(t, dir, 1, time.Now().Add(time.Hour))
	caPEM, _ := generateCert(t, 100, time.Now().Add(time.Hour))
	caFile := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	r, err := newCertReloader(certFile, keyFile, caFile)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}
	firstPool := r.ClientCAs()
	if firstPool == nil {
		t.Fatal("ClientCAs() = nil after initial load with client_ca_file set")
	}

	rotatedCAPEM, _ := generateCert(t, 101, time.Now().Add(time.Hour))
	if err := os.WriteFile(caFile, rotatedCAPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	// Reload always builds and stores a brand new *x509.CertPool on success
	// (see certReloader.Reload) — pointer identity is what actually proves
	// the swap happened; Subjects() is deprecated and, worse, identical here
	// regardless of rotation, since every generateCert call in this file
	// reuses the same CommonName.
	secondPool := r.ClientCAs()
	if firstPool == secondPool {
		t.Fatal("ClientCAs() returned the same pool after rotating client_ca_file")
	}
}
