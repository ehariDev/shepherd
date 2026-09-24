package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadLoggingConfiguration(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	tests := []struct {
		name   string
		file   string
		env    string
		level  string
		format string
	}{
		{name: "default", level: "info", format: "json"},
		{name: "config file", file: "log:\n  level: debug\n  format: text\n", level: "debug", format: "text"},
		{name: "environment", env: "debug", level: "debug", format: "json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHEPHERD_DATABASE_URL", "postgres://example")
			t.Setenv("SHEPHERD_SECURITY_ENCRYPTION_KEY", key)
			if tt.env != "" {
				t.Setenv("SHEPHERD_LOG_LEVEL", tt.env)
			} else {
				if err := os.Unsetenv("SHEPHERD_LOG_LEVEL"); err != nil {
					t.Fatal(err)
				}
			}
			file := ""
			if tt.file != "" {
				file = filepath.Join(t.TempDir(), "shepherd.yaml")
				if err := os.WriteFile(file, []byte(tt.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load(file)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Log.Level != tt.level || cfg.Log.Format != tt.format {
				t.Fatalf("logging config = (%q, %q), want (%q, %q)", cfg.Log.Level, cfg.Log.Format, tt.level, tt.format)
			}
		})
	}
}

// TestGitSyncLimitDefaults locks in the defaults from
// docs/git-provider-design.md §3.6, which internal/gitrepo.DefaultLimits
// duplicates as untyped constants (config intentionally has no dependency
// on gitrepo).
func TestGitSyncLimitDefaults(t *testing.T) {
	t.Setenv("SHEPHERD_DATABASE_URL", "postgres://example")
	t.Setenv("SHEPHERD_SECURITY_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}

	const (
		wantMaxRepoBytes = 50 * 1024 * 1024
		wantMaxFileBytes = 1 * 1024 * 1024
		wantMaxFiles     = 500
		wantFetchTimeout = 60 * time.Second
	)
	if cfg.GitSync.MaxRepoBytes != wantMaxRepoBytes {
		t.Errorf("gitsync.max_repo_bytes = %d, want %d", cfg.GitSync.MaxRepoBytes, wantMaxRepoBytes)
	}
	if cfg.GitSync.MaxFileBytes != wantMaxFileBytes {
		t.Errorf("gitsync.max_file_bytes = %d, want %d", cfg.GitSync.MaxFileBytes, wantMaxFileBytes)
	}
	if cfg.GitSync.MaxFiles != wantMaxFiles {
		t.Errorf("gitsync.max_files = %d, want %d", cfg.GitSync.MaxFiles, wantMaxFiles)
	}
	if cfg.GitSync.FetchTimeout != wantFetchTimeout {
		t.Errorf("gitsync.fetch_timeout = %s, want %s", cfg.GitSync.FetchTimeout, wantFetchTimeout)
	}
}

// TestGitSyncLimitOverrides confirms every new gitsync limit key can be
// overridden via SHEPHERD_GITSYNC_* environment variables.
func TestGitSyncLimitOverrides(t *testing.T) {
	t.Setenv("SHEPHERD_DATABASE_URL", "postgres://example")
	t.Setenv("SHEPHERD_SECURITY_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("SHEPHERD_GITSYNC_MAX_REPO_BYTES", "1000")
	t.Setenv("SHEPHERD_GITSYNC_MAX_FILE_BYTES", "2000")
	t.Setenv("SHEPHERD_GITSYNC_MAX_FILES", "7")
	t.Setenv("SHEPHERD_GITSYNC_FETCH_TIMEOUT", "5s")

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitSync.MaxRepoBytes != 1000 {
		t.Errorf("gitsync.max_repo_bytes = %d, want 1000", cfg.GitSync.MaxRepoBytes)
	}
	if cfg.GitSync.MaxFileBytes != 2000 {
		t.Errorf("gitsync.max_file_bytes = %d, want 2000", cfg.GitSync.MaxFileBytes)
	}
	if cfg.GitSync.MaxFiles != 7 {
		t.Errorf("gitsync.max_files = %d, want 7", cfg.GitSync.MaxFiles)
	}
	if cfg.GitSync.FetchTimeout != 5*time.Second {
		t.Errorf("gitsync.fetch_timeout = %s, want 5s", cfg.GitSync.FetchTimeout)
	}
}

// TestRouteApplyConfig pins the tenant-route reconciler's config: off by
// default, loadable from the env vars the chart sets, and refused when enabled
// without the namespace and receiver backend it cannot guess.
func TestRouteApplyConfig(t *testing.T) {
	base := func(t *testing.T) {
		t.Setenv("SHEPHERD_DATABASE_URL", "postgres://example")
		t.Setenv("SHEPHERD_SECURITY_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	}

	t.Run("off by default", func(t *testing.T) {
		base(t)
		cfg, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		a := cfg.Gateway.Routes.Apply
		if a.Enabled {
			t.Error("gateway.routes.apply.enabled defaults to true, want false")
		}
		if a.BackendPort != 4318 || a.Interval != 60*time.Second || a.AttachTimeout != 60*time.Second {
			t.Errorf("defaults = port %d, interval %s, attach_timeout %s; want 4318, 60s, 60s",
				a.BackendPort, a.Interval, a.AttachTimeout)
		}
	})

	t.Run("enabled from env", func(t *testing.T) {
		base(t)
		t.Setenv("SHEPHERD_GATEWAY_ROUTES_APPLY_ENABLED", "true")
		t.Setenv("SHEPHERD_GATEWAY_ROUTES_APPLY_NAMESPACE", "shepherd")
		t.Setenv("SHEPHERD_GATEWAY_ROUTES_APPLY_BACKEND_SERVICE", "shepherd-receiver")
		t.Setenv("SHEPHERD_GATEWAY_ROUTES_APPLY_BACKEND_PORT", "4319")
		cfg, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		a := cfg.Gateway.Routes.Apply
		if !a.Enabled || a.Namespace != "shepherd" || a.BackendService != "shepherd-receiver" || a.BackendPort != 4319 {
			t.Errorf("loaded %+v", a)
		}
	})

	t.Run("enabled without namespace or backend is refused", func(t *testing.T) {
		base(t)
		t.Setenv("SHEPHERD_GATEWAY_ROUTES_APPLY_ENABLED", "true")
		_, err := Load("")
		if err == nil {
			t.Fatal("Load succeeded, want a configuration error")
		}
		for _, want := range []string{"gateway.routes.apply.namespace", "gateway.routes.apply.backend_service"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not name %s", err, want)
			}
		}
	})
}

// TestGatewayPublicBaseURL: the connect-an-app default URL loads from the env
// var the chart sets, and a non-https value fails at startup rather than
// rendering snippets that point apps at plaintext.
func TestGatewayPublicBaseURL(t *testing.T) {
	for _, tc := range []struct {
		value   string
		wantErr bool
	}{
		{"", false},
		{"https://telemetry.example.com", false},
		{"http://telemetry.example.com", true},
		{"https://", true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("SHEPHERD_DATABASE_URL", "postgres://example")
			t.Setenv("SHEPHERD_SECURITY_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
			t.Setenv("SHEPHERD_GATEWAY_ROUTES_PUBLIC_BASE_URL", tc.value)
			cfg, err := Load("")
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "gateway.routes.public_base_url") {
					t.Fatalf("Load(%q) = %v, want an error naming gateway.routes.public_base_url", tc.value, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Gateway.Routes.PublicBaseURL != tc.value {
				t.Errorf("public_base_url = %q, want %q", cfg.Gateway.Routes.PublicBaseURL, tc.value)
			}
		})
	}
}

func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SHEPHERD_DATABASE_URL", "postgres://example")
	t.Setenv("SHEPHERD_SECURITY_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
}

// writeTempCert writes non-empty placeholder cert/key files and returns their
// paths. validateTLS only checks the files are openable, so their contents
// don't need to be valid PEM.
func writeTempCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	dir := t.TempDir()
	certFile = filepath.Join(dir, "tls.crt")
	keyFile = filepath.Join(dir, "tls.key")
	if err := os.WriteFile(certFile, []byte("cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

// TestTLSDisabledByDefault confirms cleartext stays the default: no TLS env
// vars set, Enabled() is false, and MinVersion/ClientAuth still take their
// defaults so a later Enabled deployment doesn't need to set them.
func TestTLSDisabledByDefault(t *testing.T) {
	setBaseEnv(t)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.TLS.Enabled() {
		t.Fatal("TLS.Enabled() = true, want false with no cert/key configured")
	}
	if cfg.Server.TLS.MinVersion != "1.2" {
		t.Errorf("server.tls.min_version = %q, want \"1.2\"", cfg.Server.TLS.MinVersion)
	}
	if cfg.Server.TLS.ClientAuth != "none" {
		t.Errorf("server.tls.client_auth = %q, want \"none\"", cfg.Server.TLS.ClientAuth)
	}
	if cfg.Server.TLS.ReloadInterval != 30*time.Second {
		t.Errorf("server.tls.reload_interval = %s, want 30s", cfg.Server.TLS.ReloadInterval)
	}
}

// TestTLSEnabledViaEnv confirms every server.tls.* key binds from its
// SHEPHERD_SERVER_TLS_* environment variable.
func TestTLSEnabledViaEnv(t *testing.T) {
	setBaseEnv(t)
	certFile, keyFile := writeTempCert(t)
	caFile, _ := writeTempCert(t) // reuse the helper as a stand-in CA bundle path
	t.Setenv("SHEPHERD_SERVER_TLS_CERT_FILE", certFile)
	t.Setenv("SHEPHERD_SERVER_TLS_KEY_FILE", keyFile)
	t.Setenv("SHEPHERD_SERVER_TLS_MIN_VERSION", "1.3")
	t.Setenv("SHEPHERD_SERVER_TLS_CLIENT_AUTH", "require_and_verify")
	t.Setenv("SHEPHERD_SERVER_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("SHEPHERD_SERVER_TLS_RELOAD_INTERVAL", "10s")
	t.Setenv("SHEPHERD_SERVER_TLS_COLLECTOR_CA_FILE", caFile)

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Server.TLS.Enabled() {
		t.Fatal("TLS.Enabled() = false, want true")
	}
	if cfg.Server.TLS.MinVersion != "1.3" {
		t.Errorf("min_version = %q, want 1.3", cfg.Server.TLS.MinVersion)
	}
	if cfg.Server.TLS.ClientAuth != "require_and_verify" {
		t.Errorf("client_auth = %q, want require_and_verify", cfg.Server.TLS.ClientAuth)
	}
	if cfg.Server.TLS.ClientCAFile != caFile {
		t.Errorf("client_ca_file = %q, want %q", cfg.Server.TLS.ClientCAFile, caFile)
	}
	if cfg.Server.TLS.ReloadInterval != 10*time.Second {
		t.Errorf("reload_interval = %s, want 10s", cfg.Server.TLS.ReloadInterval)
	}
	if cfg.Server.TLS.CollectorCAFile != caFile {
		t.Errorf("collector_ca_file = %q, want %q", cfg.Server.TLS.CollectorCAFile, caFile)
	}
}

// TestTLSValidation covers every startup-error rule from the task's design
// decision 1 and §5.1: a half-configured pair, an unknown min_version /
// client_auth, client_auth requiring a CA, and unreadable files. Table-style
// per the file's existing convention.
func TestTLSValidation(t *testing.T) {
	certFile, keyFile := writeTempCert(t)
	caFile, _ := writeTempCert(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist.pem")

	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "cert without key",
			env:     map[string]string{"SHEPHERD_SERVER_TLS_CERT_FILE": certFile},
			wantErr: "cert_file and server.tls.key_file must both be set",
		},
		{
			name:    "key without cert",
			env:     map[string]string{"SHEPHERD_SERVER_TLS_KEY_FILE": keyFile},
			wantErr: "cert_file and server.tls.key_file must both be set",
		},
		{
			name: "unknown min_version",
			env: map[string]string{
				"SHEPHERD_SERVER_TLS_CERT_FILE":   certFile,
				"SHEPHERD_SERVER_TLS_KEY_FILE":    keyFile,
				"SHEPHERD_SERVER_TLS_MIN_VERSION": "1.1",
			},
			wantErr: `min_version must be "1.2" or "1.3", got "1.1"`,
		},
		{
			name: "unknown client_auth",
			env: map[string]string{
				"SHEPHERD_SERVER_TLS_CERT_FILE":   certFile,
				"SHEPHERD_SERVER_TLS_KEY_FILE":    keyFile,
				"SHEPHERD_SERVER_TLS_CLIENT_AUTH": "sometimes",
			},
			wantErr: "client_auth must be one of none, request, require_and_verify",
		},
		{
			name: "client_auth without client_ca_file",
			env: map[string]string{
				"SHEPHERD_SERVER_TLS_CERT_FILE":   certFile,
				"SHEPHERD_SERVER_TLS_KEY_FILE":    keyFile,
				"SHEPHERD_SERVER_TLS_CLIENT_AUTH": "request",
			},
			wantErr: "client_ca_file is required when server.tls.client_auth is not",
		},
		{
			name: "unreadable cert file",
			env: map[string]string{
				"SHEPHERD_SERVER_TLS_CERT_FILE": missing,
				"SHEPHERD_SERVER_TLS_KEY_FILE":  keyFile,
			},
			wantErr: "server.tls.cert_file:",
		},
		{
			name: "unreadable client_ca_file",
			env: map[string]string{
				"SHEPHERD_SERVER_TLS_CERT_FILE":      certFile,
				"SHEPHERD_SERVER_TLS_KEY_FILE":       keyFile,
				"SHEPHERD_SERVER_TLS_CLIENT_AUTH":    "request",
				"SHEPHERD_SERVER_TLS_CLIENT_CA_FILE": missing,
			},
			wantErr: "server.tls.client_ca_file:",
		},
		{
			name: "valid, TLS off",
			env:  nil,
		},
		{
			name: "valid, TLS on with mTLS require_and_verify",
			env: map[string]string{
				"SHEPHERD_SERVER_TLS_CERT_FILE":      certFile,
				"SHEPHERD_SERVER_TLS_KEY_FILE":       keyFile,
				"SHEPHERD_SERVER_TLS_CLIENT_AUTH":    "require_and_verify",
				"SHEPHERD_SERVER_TLS_CLIENT_CA_FILE": caFile,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setBaseEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			_, err := Load("")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Load() = %v, want no error", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Load() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
