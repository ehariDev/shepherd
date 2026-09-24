package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"shepherd/internal/metrics"
)

// fileStamp is a cheap, allocation-free proxy for "has this file changed":
// mtime+size, checked before ever calling tls.LoadX509KeyPair. Kubernetes
// secret volumes swap a symlink target on rotation (mtime changes, size
// usually too), so this is the reason decision 2 in the task picked polling
// over fsnotify in the first place — a symlink swap is exactly what naive
// inotify-on-the-target watching misses.
type fileStamp struct {
	mtime time.Time
	size  int64
}

func statStamp(path string) (fileStamp, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return fileStamp{}, err
	}
	return fileStamp{mtime: fi.ModTime(), size: fi.Size()}, nil
}

// certReloader serves a TLS certificate — and, when a client CA bundle is
// configured, a client-cert verification pool — that rotates without a
// process restart. Two triggers call Reload: internal/cli/serve.go's SIGHUP
// handler, and Run's polling ticker. Both read through the same atomic
// pointers a concurrent TLS handshake uses via Get, so a handshake never
// observes a half-updated certificate.
//
// A failed Reload keeps serving whatever loaded last (task decision 4): the
// atomic pointers are only ever swapped on success, and a failure never
// returns an error that would bring the listener down — Run and the SIGHUP
// handler both log it and carry on.
type certReloader struct {
	certFile, keyFile string
	// caFile is server.tls.client_ca_file. Empty means no client-CA pool is
	// maintained; ClientCAs then always returns nil, and it is
	// internal/server's job (not this type's) to only set a ClientAuth mode
	// that requires verification when a CA file is actually configured —
	// that rule is enforced at config-validation time, not here.
	caFile string

	cert   atomic.Pointer[tls.Certificate]
	caPool atomic.Pointer[x509.CertPool]

	// mu serializes Reload (so two triggers firing at once — e.g. SIGHUP
	// landing mid-poll — do one load, not two racing ones) and guards the
	// last-seen stamps statChanged compares against.
	mu                        sync.Mutex
	lastCert, lastKey, lastCA fileStamp
}

// newCertReloader constructs a reloader and performs its first, mandatory
// load. Unlike every later Reload, this one's failure is fatal — there is no
// "last good certificate" yet to fall back to, so internal/server.New
// surfaces it as a startup error exactly like any other misconfiguration.
func newCertReloader(certFile, keyFile, caFile string) (*certReloader, error) {
	r := &certReloader{certFile: certFile, keyFile: keyFile, caFile: caFile}
	if err := r.Reload(); err != nil {
		return nil, fmt.Errorf("loading initial TLS certificate: %w", err)
	}
	return r, nil
}

// Get implements tls.Config.GetCertificate.
func (r *certReloader) Get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c := r.cert.Load()
	if c == nil {
		return nil, fmt.Errorf("no TLS certificate loaded")
	}
	return c, nil
}

// ClientCAs returns the currently loaded client-CA pool, or nil when
// server.tls.client_ca_file is unset. internal/server reads this from inside
// a tls.Config.GetConfigForClient closure (not a static ClientCAs
// assignment) so a rotated CA bundle takes effect on the next handshake.
func (r *certReloader) ClientCAs() *x509.CertPool {
	return r.caPool.Load()
}

// Reload loads the certificate (and, if configured, the client CA bundle)
// unconditionally — it does not check whether the files changed; statChanged
// is what gates the polling path in Run. Called directly by
// newCertReloader (initial load) and by internal/cli/serve.go's SIGHUP
// handler (forced reload on operator request); Run calls it only after
// statChanged reports a difference.
//
// The certificate and the client-CA bundle are reloaded as two independent
// steps, each keeping its own last-good value on failure — not one
// all-or-nothing unit. They used to be coupled: a transient or corrupt
// client_ca_file failure aborted the whole Reload before the certificate was
// ever stored, so a perfectly good certificate rotation was silently
// discarded right along with the CA bundle it had nothing to do with. Now a
// CA-bundle failure is reported (see the returned error) but never blocks an
// otherwise-valid certificate from taking effect.
func (r *certReloader) Reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		metrics.TLSCertReloadsTotal.WithLabelValues("failure").Inc()
		return fmt.Errorf("loading key pair: %w", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		metrics.TLSCertReloadsTotal.WithLabelValues("failure").Inc()
		return fmt.Errorf("parsing leaf certificate: %w", err)
	}
	if time.Now().After(leaf.NotAfter) {
		metrics.TLSCertReloadsTotal.WithLabelValues("failure").Inc()
		return fmt.Errorf("certificate expired at %s", leaf.NotAfter)
	}
	cert.Leaf = leaf

	// The certificate is known-good: store and stamp it now, independent of
	// whatever happens with the client-CA bundle below.
	r.cert.Store(&cert)
	if s, err := statStamp(r.certFile); err == nil {
		r.lastCert = s
	}
	if s, err := statStamp(r.keyFile); err == nil {
		r.lastKey = s
	}
	metrics.TLSCertNotAfter.Set(float64(leaf.NotAfter.Unix()))

	var caErr error
	if r.caFile != "" {
		if pemBytes, err := os.ReadFile(r.caFile); err != nil {
			caErr = fmt.Errorf("reading client CA file: %w", err)
		} else {
			pool := x509.NewCertPool()
			if ok := pool.AppendCertsFromPEM(pemBytes); !ok {
				caErr = fmt.Errorf("no certificates found in %s", r.caFile)
			} else {
				r.caPool.Store(pool)
				if s, err := statStamp(r.caFile); err == nil {
					r.lastCA = s
				}
			}
		}
	}
	if caErr != nil {
		metrics.TLSCertReloadsTotal.WithLabelValues("failure").Inc()
		return fmt.Errorf("certificate reloaded, but the client CA bundle was not (kept the last good one): %w", caErr)
	}
	metrics.TLSCertReloadsTotal.WithLabelValues("success").Inc()
	return nil
}

// statChanged reports whether cert/key/CA mtime or size differ from the last
// successful Reload, without opening or parsing anything — the cheap check
// Run's ticker does every interval so an unchanged deployment costs three
// stat(2) calls, not three LoadX509KeyPair/PEM-parse calls.
func (r *certReloader) statChanged() (bool, error) {
	certStamp, err := statStamp(r.certFile)
	if err != nil {
		return false, fmt.Errorf("stat cert_file: %w", err)
	}
	keyStamp, err := statStamp(r.keyFile)
	if err != nil {
		return false, fmt.Errorf("stat key_file: %w", err)
	}
	var caStamp fileStamp
	if r.caFile != "" {
		caStamp, err = statStamp(r.caFile)
		if err != nil {
			return false, fmt.Errorf("stat client_ca_file: %w", err)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return certStamp != r.lastCert || keyStamp != r.lastKey || caStamp != r.lastCA, nil
}

// Run polls for certificate/CA changes every interval until ctx is done. An
// interval of 0 (server.tls.reload_interval: 0) disables polling entirely —
// SIGHUP-triggered reload still works, since that calls Reload directly, not
// through Run.
func (r *certReloader) Run(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.pollOnce(logger)
		}
	}
}

func (r *certReloader) pollOnce(logger *slog.Logger) {
	changed, err := r.statChanged()
	if err != nil {
		logger.Warn("checking TLS certificate files for changes", "err", err)
		return
	}
	if !changed {
		return
	}
	if err := r.Reload(); err != nil {
		// err may report only a client-CA bundle problem — the certificate
		// itself can have rotated fine in the same call (see Reload's
		// comment), so this is not always "nothing changed."
		logger.Warn("TLS reload had a problem; each of the certificate and client-CA bundle keeps whichever it last loaded successfully", "err", err)
		return
	}
	logger.Info("TLS certificate reloaded")
}
