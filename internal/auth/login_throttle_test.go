package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"shepherd/internal/auth"
	"shepherd/internal/config"
	"shepherd/internal/store"
	"shepherd/internal/store/sqlc"
)

// postLocalLogin drives LocalLoginHandler the way the real route does: a
// JSON body decoded by the handler itself, not a query string.
func postLocalLogin(tb testing.TB, h *auth.Handler, username, password string) *http.Response {
	tb.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	Expect(err).NotTo(HaveOccurred())
	req := httptest.NewRequest(http.MethodPost, "/auth/local/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.LocalLoginHandler(rr, req)
	return rr.Result()
}

var _ = Describe("Local sign-in throttle", Label("integration"), func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		st     *store.Store
		h      *auth.Handler
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		dbURL := sharedPG.IsolatedDB(ctx, GinkgoTB())
		var err error
		st, err = store.New(ctx, &config.DatabaseConfig{URL: dbURL, MaxConns: 5})
		Expect(err).NotTo(HaveOccurred())
		cfg := &config.Config{Auth: config.AuthConfig{InsecureCookies: true}}
		h = auth.NewLocalAdmin(cfg, st, slog.Default())

		hash, err := auth.HashPassword("correct-horse-battery-staple")
		Expect(err).NotTo(HaveOccurred())
		_, err = st.Queries.CreateUser(ctx, sqlc.CreateUserParams{
			Login: "throttled", Email: "throttled@example.com", DisplayName: "Throttled",
			PasswordHash: hash, IsAppAdmin: false, MustChangePassword: false,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		st.Close()
		cancel()
	})

	// httptest.NewRequest defaults RemoteAddr to the same value for every
	// request in this spec, so the per-login bucket (burst 10) is the one
	// that trips well before the per-IP bucket (burst 60) could.
	It("locks a login out after ten failed attempts within a minute, even on the eleventh correct password", func() {
		for i := 0; i < 10; i++ {
			resp := postLocalLogin(GinkgoTB(), h, "throttled", "wrong-password")
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"attempt %d should be a plain credential rejection, not yet throttled", i+1)
			resp.Body.Close() //nolint:errcheck // test cleanup
		}

		resp := postLocalLogin(GinkgoTB(), h, "throttled", "correct-horse-battery-staple")
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		Expect(resp.StatusCode).To(Equal(http.StatusTooManyRequests),
			"the 11th attempt within the window must be throttled even with the right password")
		Expect(resp.Header.Get("Retry-After")).NotTo(BeEmpty())
	})

	It("does not throttle a different login sharing the same source IP", func() {
		hash, err := auth.HashPassword("another-password")
		Expect(err).NotTo(HaveOccurred())
		_, err = st.Queries.CreateUser(ctx, sqlc.CreateUserParams{
			Login: "unthrottled", Email: "unthrottled@example.com", DisplayName: "Unthrottled",
			PasswordHash: hash, IsAppAdmin: false, MustChangePassword: false,
		})
		Expect(err).NotTo(HaveOccurred())

		for i := 0; i < 10; i++ {
			resp := postLocalLogin(GinkgoTB(), h, "throttled", "wrong-password")
			resp.Body.Close() //nolint:errcheck // test cleanup
		}

		resp := postLocalLogin(GinkgoTB(), h, "unthrottled", "another-password")
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"a different login's own bucket must not be exhausted by another login's failures")
	})
})
