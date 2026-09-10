package simsvc

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeAlloy writes an executable shell script standing in for the real
// grafana/alloy binary and returns its path. runAlloy invokes it with
// Alloy's real flags (run <config> --storage.path=... --server.http...),
// which the script is free to ignore; body is what actually runs.
func fakeAlloy(body string) string {
	GinkgoHelper()
	dir := GinkgoT().TempDir()
	path := filepath.Join(dir, "fake-alloy")
	Expect(os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700)).To(Succeed())
	return path
}

// discardLogger is a logger runAlloy can write its debug noise to without a
// spec having to assert on it.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// healthTracker is what makes a sandbox run report "this component was broken"
// instead of "this component exited" — VB-1 §6.4's requirement that a run whose
// component went unhealthy still COMPLETES with that state in the health tab,
// rather than failing the request. The stickiness it implements is not
// observable from the run API against real Alloy today (measured: the S3
// transform stubs or removes every component that reports unhealthy at
// runtime — discovery/log sources are replaced by fixtures, local.file and
// remote.http are removed as secret sources, and the otelcol wrapper reports
// "healthy" even when its receiver fails to bind; see
// docs/proofs/sandbox-sim-e2e.md §4). These specs pin it at the level where it
// IS reachable, so the branch is not carried untested on the strength of an
// argument.
var _ = Describe("healthTracker", func() {
	comp := func(id, state, msg string) alloyComponent {
		var c alloyComponent
		c.LocalID = id
		c.Health.State = state
		c.Health.Message = msg
		return c
	}

	It("keeps an unhealthy observation when shutdown flips the component to a benign state", func() {
		t := newHealthTracker()
		t.observe([]alloyComponent{comp("prometheus.remote_write.sink", "healthy", "started component")})
		t.observe([]alloyComponent{comp("prometheus.remote_write.sink", "unhealthy", "remote write failed")})
		// Teardown: Alloy reports every component "exited" as the run ends. A
		// run that was broken for most of its life must not be reported clean
		// because of the last poll before shutdown.
		t.observe([]alloyComponent{comp("prometheus.remote_write.sink", "exited", "component shut down")})

		snap := t.snapshot(map[string]string{"prometheus.remote_write.sink": "n2"})
		Expect(snap).To(HaveLen(1))
		Expect(snap[0].NodeID).To(Equal("n2"))
		Expect(snap[0].Health).To(Equal("unhealthy"))
		Expect(snap[0].Message).To(Equal("remote write failed"))
	})

	It("reports the latest state for a component that was never unhealthy", func() {
		t := newHealthTracker()
		t.observe([]alloyComponent{comp("prometheus.scrape.app", "healthy", "started component")})
		t.observe([]alloyComponent{comp("prometheus.scrape.app", "exited", "component shut down")})

		snap := t.snapshot(map[string]string{"prometheus.scrape.app": "n1"})
		Expect(snap).To(HaveLen(1))
		// Not sticky in the other direction: only "unhealthy" is held, so a
		// healthy component still reports its real final state.
		Expect(snap[0].Health).To(Equal("exited"))
	})

	It("reports every component in first-seen order, and a component with no node mapping still appears", func() {
		t := newHealthTracker()
		t.observe([]alloyComponent{
			comp("discovery.relabel.k8s", "healthy", "started component"),
			comp("prometheus.scrape.app", "healthy", "started component"),
		})
		// A component the transform added has no authored node behind it; it
		// must still be reported rather than dropped, otherwise the health tab
		// silently omits whatever the sandbox actually ran.
		snap := t.snapshot(map[string]string{"prometheus.scrape.app": "n1"})
		Expect(snap).To(HaveLen(2))
		Expect(snap[0].LocalID).To(Equal("discovery.relabel.k8s"))
		Expect(snap[0].NodeID).To(BeEmpty())
		Expect(snap[1].LocalID).To(Equal("prometheus.scrape.app"))
		Expect(snap[1].NodeID).To(Equal("n1"))
	})
})

// These specs run the real exec.CommandContext path against a fake Alloy
// binary — a shell script — rather than mocking os/exec, because the bug
// class here (an inherited environment, a signal never sent, a directory
// never cleaned up) only exists at the level of what the OS actually does
// with the child process.
var _ = Describe("runAlloy", func() {
	var dir string

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	baseOpts := func(dir, binary string) runnerOptions {
		return runnerOptions{
			AlloyBinary: binary,
			Config:      "// no-op",
			// Long enough that a fake binary's own process-start overhead
			// (measured up to ~150ms for a freshly-written script on this
			// platform) never races the run's own duration timeout — a
			// short Duration here would cancel the child before its script
			// body ever ran, making every assertion below vacuously true.
			Duration:   800 * time.Millisecond,
			RunDir:     filepath.Join(dir, "run"),
			StorageDir: filepath.Join(dir, "storage"),
			AlloyHTTP:  "127.0.0.1:0",
		}
	}

	It("does not pass the simulator's environment to the sandboxed Alloy", func() {
		GinkgoT().Setenv("SIM_TEST_CANARY", "leak-if-inherited")
		outcome := runAlloy(context.Background(), baseOpts(dir, fakeAlloy("env >&2")), discardLogger())
		Expect(strings.Join(outcome.StderrTail, "\n")).NotTo(ContainSubstring("SIM_TEST_CANARY"),
			"the sandboxed Alloy must get an explicit, minimal environment — not the simulator's own (which after "+
				"W4-S3/S4 carries SIM_TOKEN, readable by any user config via sys.env(...))")
	})

	It("invokes Alloy with the exact run flags runnerOptions describes", func() {
		opts := baseOpts(dir, fakeAlloy(`printf '%s\n' "$@" >&2`))
		outcome := runAlloy(context.Background(), opts, discardLogger())
		Expect(outcome.StderrTail).To(Equal([]string{
			"run", filepath.Join(opts.RunDir, "config.alloy"),
			"--storage.path=" + opts.StorageDir,
			"--server.http.listen-addr=" + opts.AlloyHTTP,
			"--disable-reporting",
			"--server.http.enable-pprof=false",
			"--server.http.disable-support-bundle",
			"--stability.level=" + opts.StabilityLevel,
		}))
	})

	It("always tears down its run and storage directories, whether or not Alloy started successfully", func() {
		// One-line-revert red proof (control already holds today): commenting
		// out `defer os.RemoveAll(opts.RunDir)` at runner.go:76 leaves this
		// spec's `NotTo(BeADirectory())` failing with the directory still
		// present, restored immediately after confirming the failure.
		opts := baseOpts(dir, fakeAlloy("exit 0"))
		runAlloy(context.Background(), opts, discardLogger())
		Expect(opts.RunDir).NotTo(BeADirectory())
		Expect(opts.StorageDir).NotTo(BeADirectory())
	})

	It("reports a start failure as a completed outcome carrying errAlloyStartFailed, not a transport error", func() {
		opts := baseOpts(dir, filepath.Join(dir, "does-not-exist"))
		outcome := runAlloy(context.Background(), opts, discardLogger())
		Expect(errors.Is(outcome.Err, errAlloyStartFailed)).To(BeTrue(),
			"a binary that fails to start must surface as outcome.Err wrapping errAlloyStartFailed — §6.4 treats a "+
				"start-time failure as the run's RESULT, not an API error")
		// The scratch dirs are created before cmd.Start() and must still be
		// torn down on this early-return path.
		Expect(opts.RunDir).NotTo(BeADirectory())
		Expect(opts.StorageDir).NotTo(BeADirectory())
	})

	It("kills Alloy after KillGrace, rather than the production 15s constant, once it ignores the initial interrupt", func() {
		// Duration must comfortably outlast a freshly-exec'd shell's own
		// startup latency (measured up to ~150ms on this platform): a
		// shorter one would deliver the SIGINT before the script's `trap`
		// line ever ran, killing it on the default disposition instead of
		// exercising the grace path this spec means to prove.
		opts := baseOpts(dir, fakeAlloy(`trap '' TERM INT; sleep 5`))
		opts.Duration = 500 * time.Millisecond
		opts.KillGrace = 300 * time.Millisecond

		result := make(chan runOutcome, 1)
		go func() { result <- runAlloy(context.Background(), opts, discardLogger()) }()

		// Duration + KillGrace + slop: comfortably more than the ~800ms this
		// should take, comfortably less than the 15s+ it takes if KillGrace
		// were not actually wired to cmd.WaitDelay.
		select {
		case <-result:
		case <-time.After(3 * time.Second):
			Fail("runAlloy did not return within Duration+KillGrace — the injected KillGrace was not honored")
		}
	})
})
