package mgmtapi_test

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mgmtv1 "shepherd/gen/shepherd/mgmt/v1"
	"shepherd/internal/auth"
	"shepherd/internal/config"
	"shepherd/internal/mgmtapi"
	"shepherd/internal/store"
	"shepherd/internal/store/sqlc"
)

// UpdateUser previously hand-rolled its own pgx.ErrNoRows -> CodeNotFound /
// else CodeInternal mapping instead of calling mapError (rpc_errors.go) for
// the non-ErrNoRows branch, same shape as loadOwnedDestination's pattern
// (rpc_destination.go). This spec pins that: a real, non-ErrNoRows store
// failure must come back as CodeInternal with mapError's exact message
// shape (internal error, not a raw or bespoke message), the same guarantee
// rpc_errors_test.go's TestMapError pins for mapError itself and
// rpc_destination_test.go's "maps a real lookup failure ... to Internal"
// spec pins for loadOwnedDestination.
var _ = Describe("shepherd.mgmt.v1.UserService UpdateUser error mapping", Label("integration"), func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		st     *store.Store
		svc    *mgmtapi.UserService
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		dbURL := sharedPG.IsolatedDB(ctx, GinkgoTB())

		var err error
		st, err = store.New(ctx, &config.DatabaseConfig{URL: dbURL, MaxConns: 5}, slog.Default())
		Expect(err).NotTo(HaveOccurred())

		svc = mgmtapi.NewUserService(st, auth.NewUserStore(st, slog.Default()), slog.Default())
	})

	AfterEach(func() {
		st.Close()
		cancel()
	})

	It("maps a real store failure on UpdateUser to Internal with mapError's message shape, not a false Not Found", func() {
		u, err := st.Queries.CreateUser(ctx, sqlc.CreateUserParams{
			Login: "update-fault-user", Email: "update-fault@example.com", DisplayName: "Update Fault",
			PasswordHash: "hash", IsAppAdmin: true, MustChangePassword: false,
		})
		Expect(err).NotTo(HaveOccurred())

		// Hold an ACCESS EXCLUSIVE lock on users from a separate connection so
		// UpdateUser's UPDATE blocks on it, then cancel that backend -- forcing
		// a real, non-ErrNoRows failure deterministically, without a
		// fault-injection seam in store.go. Mirrors
		// rpc_destination_test.go's "maps a real lookup failure ... to
		// Internal" spec.
		lockConn, err := st.Pool().Acquire(ctx)
		Expect(err).NotTo(HaveOccurred())
		defer lockConn.Release()
		lockTx, err := lockConn.Begin(ctx)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = lockTx.Rollback(ctx) }() //nolint:errcheck // best-effort cleanup; explicit Rollback below is the real one
		_, err = lockTx.Exec(ctx, `LOCK TABLE users IN ACCESS EXCLUSIVE MODE`)
		Expect(err).NotTo(HaveOccurred())

		type result struct {
			resp *connect.Response[mgmtv1.User]
			err  error
		}
		resultCh := make(chan result, 1)
		go func() {
			defer GinkgoRecover()
			// IsAppAdmin: true, Disabled: false keeps guardLastAdmin's "not
			// removing admin rights" fast path (no query, no interference
			// with the held lock) so only UpdateUser's own UPDATE blocks.
			resp, updateErr := svc.UpdateUser(ctx, connect.NewRequest(&mgmtv1.UpdateUserRequest{
				Id: u.ID.String(), Email: "new@example.com", DisplayName: "New Name",
				IsAppAdmin: true, Disabled: false,
			}))
			resultCh <- result{resp, updateErr}
		}()

		var pid int
		Eventually(func() error {
			return st.Pool().QueryRow(ctx,
				`SELECT pid FROM pg_stat_activity
				 WHERE datname = current_database() AND wait_event_type = 'Lock' AND query ILIKE '%UpdateUser%'`,
			).Scan(&pid)
		}, "5s", "20ms").Should(Succeed(), "UpdateUser's query never blocked on the held table lock")
		_, err = st.Pool().Exec(ctx, `SELECT pg_cancel_backend($1)`, pid)
		Expect(err).NotTo(HaveOccurred())

		res := <-resultCh
		Expect(res.resp).To(BeNil())
		Expect(connect.CodeOf(res.err)).To(Equal(connect.CodeInternal), "a real update failure must not be reported as not_found")
		Expect(res.err.Error()).To(Equal("internal: internal error"), "mapError must not leak the raw error to the client")

		Expect(lockTx.Rollback(ctx)).To(Succeed())
	})
})
