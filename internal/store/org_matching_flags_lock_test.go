package store_test

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"shepherd/internal/store"
	"shepherd/internal/store/sqlc"
)

// GetOrgByIDForUpdate backs UpdateOrg's fix for a TOCTOU race (PR-144 review
// §2/§10d follow-up): reading an org's pre-write matching-flag values and
// writing the update now happen inside the same transaction, with the read
// taking a row lock. Without that lock, two concurrent UpdateOrg calls on
// the same org could each read the same pre-write "before", and whichever
// commits second would diff its own result against a now-stale snapshot --
// missing a real flag flip and skipping the serve-cache invalidation for it.
// This test proves the lock itself works: a second transaction's
// GetOrgByIDForUpdate on the same org blocks until the first transaction
// that acquired it commits.
var _ = Describe("GetOrgByIDForUpdate row locking", Label("integration"), func() {
	var (
		pool  *pgxpool.Pool
		orgID pgtype.UUID
	)

	BeforeEach(func(ctx context.Context) {
		url := sharedPG.IsolatedDB(ctx, GinkgoTB())
		Expect(store.MigrateUp(ctx, url)).To(Succeed())
		var err error
		pool, err = pgxpool.New(ctx, url)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(pool.Close)

		err = pool.QueryRow(ctx,
			`INSERT INTO orgs (name, display_name, admin_group_id) VALUES ('lock-test', 'lock-test', 'g') RETURNING id`,
		).Scan(&orgID)
		Expect(err).NotTo(HaveOccurred())
	})

	It("blocks a second reader until the first transaction commits", func(ctx context.Context) {
		tx1, err := pool.Begin(ctx)
		Expect(err).NotTo(HaveOccurred())
		q1 := sqlc.New(tx1)
		_, err = q1.GetOrgByIDForUpdate(ctx, orgID)
		Expect(err).NotTo(HaveOccurred())

		tx2, err := pool.Begin(ctx)
		Expect(err).NotTo(HaveOccurred())
		q2 := sqlc.New(tx2)

		done := make(chan struct{})
		go func() {
			defer close(done)
			q2.GetOrgByIDForUpdate(ctx, orgID) //nolint:errcheck // only the blocking behavior below is under test; the query's own result is irrelevant
		}()

		select {
		case <-done:
			Fail("second reader returned before the first transaction committed -- FOR UPDATE lock is not being held")
		case <-time.After(300 * time.Millisecond):
			// Still blocked, as expected.
		}

		Expect(tx1.Commit(ctx)).To(Succeed())

		select {
		case <-done:
			// Unblocked as expected.
		case <-time.After(2 * time.Second):
			Fail("second reader did not unblock after the first transaction committed")
		}

		Expect(tx2.Rollback(ctx)).To(Succeed())
	})
})
