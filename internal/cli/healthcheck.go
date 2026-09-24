package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

// healthcheckOptions is runHealthcheck's input, read from cobra flags (and,
// for tlsExplicit, whether --tls was actually passed) by the command's RunE.
// Split out so runHealthcheck can be exercised directly against an
// httptest.Server without going through cobra flag parsing.
type healthcheckOptions struct {
	addr               string
	tls                bool
	tlsExplicit        bool // true when --tls/--tls=false was passed, not defaulted
	caFile             string
	insecureSkipVerify bool
}

// runHealthcheck does the actual GET /healthz and status check. Default
// behaviour (http://addr, http.DefaultClient) is byte-for-byte what it was
// before TLS support existed — nothing TLS-related runs unless opts.tls ends
// up true.
func runHealthcheck(ctx context.Context, opts healthcheckOptions) error {
	useTLS := opts.tls
	// Honour SHEPHERD_SERVER_TLS_* so a systemd ExecStartPost or container
	// healthcheck that already runs with the server's own environment
	// doesn't have to repeat --tls: the same "cert_file and key_file both
	// set" rule config.TLSConfig.Enabled uses decides it here too. An
	// explicit --tls / --tls=false always wins over the environment.
	if !opts.tlsExplicit &&
		os.Getenv("SHEPHERD_SERVER_TLS_CERT_FILE") != "" &&
		os.Getenv("SHEPHERD_SERVER_TLS_KEY_FILE") != "" {
		useTLS = true
	}

	scheme := "http"
	client := http.DefaultClient
	if useTLS {
		scheme = "https"
		tlsConfig := &tls.Config{InsecureSkipVerify: opts.insecureSkipVerify} //nolint:gosec // explicit operator opt-in via --insecure-skip-verify, for a localhost check against a certificate whose SAN is the public hostname
		if opts.caFile != "" {
			pemBytes, readErr := os.ReadFile(opts.caFile)
			if readErr != nil {
				return fmt.Errorf("reading ca-file: %w", readErr)
			}
			pool := x509.NewCertPool()
			if ok := pool.AppendCertsFromPEM(pemBytes); !ok {
				return fmt.Errorf("ca-file %s: no certificates found", opts.caFile)
			}
			tlsConfig.RootCAs = pool
		}
		client = &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scheme+"://"+opts.addr+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // process exits immediately after; close error not actionable
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: status %d", resp.StatusCode)
	}
	return nil
}

func init() {
	cmd := &cobra.Command{
		Use:          "healthcheck",
		Short:        "Check /healthz endpoint (exits 0 if healthy, 1 otherwise)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			addr, err := cmd.Flags().GetString("addr")
			if err != nil {
				return fmt.Errorf("reading addr flag: %w", err)
			}
			useTLS, err := cmd.Flags().GetBool("tls")
			if err != nil {
				return fmt.Errorf("reading tls flag: %w", err)
			}
			caFile, err := cmd.Flags().GetString("ca-file")
			if err != nil {
				return fmt.Errorf("reading ca-file flag: %w", err)
			}
			insecure, err := cmd.Flags().GetBool("insecure-skip-verify")
			if err != nil {
				return fmt.Errorf("reading insecure-skip-verify flag: %w", err)
			}
			return runHealthcheck(cmd.Context(), healthcheckOptions{
				addr:               addr,
				tls:                useTLS,
				tlsExplicit:        cmd.Flags().Changed("tls"),
				caFile:             caFile,
				insecureSkipVerify: insecure,
			})
		},
	}
	cmd.Flags().String("addr", "localhost:8080", "host:port of the shepherd server")
	cmd.Flags().Bool("tls", false, "use https:// (auto-enabled when SHEPHERD_SERVER_TLS_CERT_FILE and _KEY_FILE are both set in the environment)")
	cmd.Flags().String("ca-file", "", "PEM CA bundle to verify the server's certificate against (defaults to the system trust store)")
	cmd.Flags().Bool("insecure-skip-verify", false, "skip TLS certificate verification (for a localhost check against a certificate whose SAN is the public hostname)")
	rootCmd.AddCommand(cmd)
}
