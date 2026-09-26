package store_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"shepherd/internal/config"
	"shepherd/internal/store"
	"shepherd/internal/store/sqlc"
)

// GetLatestLocalAttributesByCollector is org_id-scoped for the same reason
// loadOwnedCollector (internal/mgmtapi/rpc_fleet.go) checks collector
// ownership before every by-id read: a UUID alone is not an authorization
// boundary. This proves the scoping actually holds at the query level, not
// just at today's two call sites (which happen to already validate ownership
// before calling in) -- a defense-in-depth guarantee, not a currently
// exploitable gap.
var _ = Describe("GetLatestLocalAttributesByCollector", Label("integration"), func() {
	var (
		ctx context.Context
		st  *store.Store
	)

	BeforeEach(func() {
		ctx = context.Background()
		dbURL := sharedPG.IsolatedDB(ctx, GinkgoTB())
		Expect(store.MigrateUp(ctx, dbURL)).To(Succeed())
		var err error
		st, err = store.New(ctx, &config.DatabaseConfig{URL: dbURL, MaxConns: 5})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(st.Close)
	})

	newOrgAndCollector := func(orgName, clusterName string) (pgtype.UUID, pgtype.UUID) {
		o, err := st.Queries.CreateOrg(ctx, sqlc.CreateOrgParams{
			Name: orgName, DisplayName: orgName, AdminGroupID: "admin-grp",
		})
		Expect(err).NotTo(HaveOccurred())
		cluster, err := st.Queries.UpsertCluster(ctx, clusterName)
		Expect(err).NotTo(HaveOccurred())
		Expect(st.Queries.ClaimCluster(ctx, sqlc.ClaimClusterParams{ID: cluster.ID, OrgID: o.ID})).To(Succeed())
		collector, err := st.Queries.UpsertCollector(ctx, sqlc.UpsertCollectorParams{ClusterID: cluster.ID, Role: "metrics"})
		Expect(err).NotTo(HaveOccurred())
		return o.ID, collector.ID
	}

	instance := func(id string, collectorID pgtype.UUID, attrs string) {
		_, err := st.Queries.UpsertCollectorInstance(ctx, sqlc.UpsertCollectorInstanceParams{
			ID: id, CollectorID: collectorID, Name: id, LocalAttributes: json.RawMessage(attrs),
		})
		Expect(err).NotTo(HaveOccurred())
	}

	It("returns the collector's attributes when it belongs to the given org", func() {
		orgID, collectorID := newOrgAndCollector("attrs-org", "attrs-cluster")
		instance("i1", collectorID, `{"team":"platform"}`)

		raw, err := st.Queries.GetLatestLocalAttributesByCollector(ctx, sqlc.GetLatestLocalAttributesByCollectorParams{
			CollectorID: collectorID, OrgID: orgID,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(raw).To(MatchJSON(`{"team":"platform"}`))
	})

	It("returns no rows when the collector belongs to a different org", func() {
		_, collectorID := newOrgAndCollector("owner-org", "owner-cluster")
		instance("i1", collectorID, `{"team":"platform"}`)
		otherOrgID, _ := newOrgAndCollector("other-org", "other-cluster")

		_, err := st.Queries.GetLatestLocalAttributesByCollector(ctx, sqlc.GetLatestLocalAttributesByCollectorParams{
			CollectorID: collectorID, OrgID: otherOrgID,
		})
		Expect(err).To(MatchError(pgx.ErrNoRows),
			"a collector's attributes must never be readable under another org's id")
	})

	It("excludes an instance that went inactive without a clean unregister", func() {
		orgID, collectorID := newOrgAndCollector("stale-org", "stale-cluster")
		instance("stale-instance", collectorID, `{"team":"platform"}`)
		_, err := st.Pool().Exec(ctx,
			`UPDATE collector_instances SET remote_config_status = 'inactive', last_seen = $2 WHERE id = $1`,
			"stale-instance", time.Now())
		Expect(err).NotTo(HaveOccurred())

		_, err = st.Queries.GetLatestLocalAttributesByCollector(ctx, sqlc.GetLatestLocalAttributesByCollectorParams{
			CollectorID: collectorID, OrgID: orgID,
		})
		Expect(err).To(MatchError(pgx.ErrNoRows))
	})
})
