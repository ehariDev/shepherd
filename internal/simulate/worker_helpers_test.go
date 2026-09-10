package simulate_test

import (
	"strings"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"shepherd/internal/simulate"
)

// SanitizeStderrTail is what stands between the sandbox Alloy's raw stderr
// bytes (bufio.Scanner.Text() on grafana/alloy's own log lines — not
// guaranteed valid UTF-8, and free to contain control characters) and
// simulate_runs.stderr_tail, a Postgres TEXT column. A byte-index truncation
// that lands mid-rune produces invalid UTF-8, which Postgres refuses to
// store (SQLSTATE 22021) — CompleteSimulateRun then fails and the run is
// stuck until the janitor reaps it (internal/simulate/worker.go:363-368).
//
// maxTailBytes below must match worker_helpers.go's unexported
// maxStderrTailBytes (8 * 1024); it is duplicated here because this spec
// lives in the external simulate_test package.
const maxTailBytes = 8 * 1024

var _ = Describe("SanitizeStderrTail", func() {
	It("never splits a multi-byte UTF-8 rune when truncating to the tail cap", func() {
		// The euro sign is 3 bytes (E2 82 AC). maxTailBytes (8192) is not a
		// multiple of 3, so a naive s[len(s)-maxTailBytes:] byte cut through
		// a run of nothing but euro signs is guaranteed to start mid-rune.
		s := strings.Repeat("a", 1000) + strings.Repeat("€", 5000)
		Expect(len(s)).To(BeNumerically(">", maxTailBytes))

		out := simulate.SanitizeStderrTail(s)

		Expect(utf8.ValidString(out)).To(BeTrue(),
			"a byte-index cut through multi-byte runes must not produce invalid UTF-8")
		Expect(len(out)).To(BeNumerically("<=", maxTailBytes))
		// The cut must still land ON a rune boundary that was actually
		// present in the input — not just happen to decode as *something*
		// valid (which strings.ToValidUTF8 alone could do by replacing the
		// partial rune's bytes) — so the tail is a genuine suffix of s.
		Expect(strings.HasSuffix(s, out)).To(BeTrue())
	})

	It("strips control characters (including invalid UTF-8 bytes) but keeps newlines and tabs", func() {
		s := "component started\x00\x1b[31m error\x01: dial tcp\ttimed out\x7f\nnext line\xff\xfe"
		out := simulate.SanitizeStderrTail(s)

		Expect(utf8.ValidString(out)).To(BeTrue())
		Expect(out).To(ContainSubstring("component started"))
		Expect(out).To(ContainSubstring("\ttimed out"))
		Expect(out).To(ContainSubstring("\nnext line"))
		Expect(out).NotTo(ContainSubstring("\x00"))
		Expect(out).NotTo(ContainSubstring("\x1b"))
		Expect(out).NotTo(ContainSubstring("\x01"))
		Expect(out).NotTo(ContainSubstring("\x7f"))
	})

	It("returns short, already-clean input unchanged", func() {
		s := "alloy: config loaded\ncomponent prometheus.scrape.app started"
		Expect(simulate.SanitizeStderrTail(s)).To(Equal(s))
	})
})
