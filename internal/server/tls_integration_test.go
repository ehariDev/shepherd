package server

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"shepherd/internal/config"
	"shepherd/internal/spa"
	"shepherd/internal/store"
	"shepherd/internal/validate"
)

// TestTLSListenerHTTP1AndHTTP2 is the test matrix's "TLS: HTTP/1.1 and HTTP/2
// (ALPN)" row: a real tls.Listener serving the production router (not a
// recorder), with two clients — one restricted to http/1.1 in its ALPN
// offer, one requesting HTTP/2 — confirming the server negotiates each
// correctly against the same listener. newRouter needs no database (see
// server_test.go's productionRouter), so this stays a fast unit-style test.
func TestTLSListenerHTTP1AndHTTP2(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertFiles(t, dir, 1, time.Now().Add(time.Hour))

	reloader, err := newCertReloader(certFile, keyFile, "")
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}
	tlsCfg := buildTLSConfig(config.TLSConfig{MinVersion: "1.2", ClientAuth: "none"}, reloader)

	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := newRouter(cfg, &store.Store{}, nil, nil, validate.New(&cfg.Validate), spa.BuildInfo{}, logger)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	httpSrv := &http.Server{Handler: router, TLSConfig: tlsCfg}
	go httpSrv.Serve(ln)  //nolint:errcheck // errors surface as failed client requests below
	defer httpSrv.Close() //nolint:errcheck // test cleanup

	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(certPEM); !ok {
		t.Fatal("could not parse the test certificate as a trust root")
	}

	addr := ln.Addr().String()

	tests := []struct {
		name      string
		transport *http.Transport
		wantProto string
	}{
		{
			name:      "HTTP/1.1",
			transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, NextProtos: []string{"http/1.1"}}},
			wantProto: "HTTP/1.1",
		},
		{
			name:      "HTTP/2 via ALPN",
			transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}, ForceAttemptHTTP2: true},
			wantProto: "HTTP/2.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: tt.transport}
			resp, err := client.Get("https://" + addr + "/healthz")
			if err != nil {
				t.Fatalf("GET /healthz: %v", err)
			}
			defer resp.Body.Close() //nolint:errcheck // test cleanup
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if resp.Proto != tt.wantProto {
				t.Fatalf("negotiated proto = %s, want %s", resp.Proto, tt.wantProto)
			}
		})
	}
}
