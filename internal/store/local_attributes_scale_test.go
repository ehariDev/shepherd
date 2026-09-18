package store_test

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"shepherd/internal/config"
	"shepherd/internal/store"
	"shepherd/internal/store/sqlc"
)

// queryCounter is a minimal pgx.QueryTracer that counts Query/QueryRow/Exec
// calls issued over one pool -- proves ListLatestLocalAttributesByOrg is one
// round trip regardless of collector count, LABEL-MATCHING-PLAN.md §6 Phase
// 2 step 1's explicitly called-out N+1 risk at scale.
type queryCounter struct {
	mu    sync.Mutex
	count int
}

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
	return ctx
}

func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *queryCounter) Reset() {
	c.mu.Lock()
	c.count = 0
	c.mu.Unlock()
}

func (c *queryCounter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

var _ = Describe("ListLatestLocalAttributesByOrg", Label("integration"), func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		st     *store.Store
		orgID  pgtype.UUID
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		dbURL := sharedPG.IsolatedDB(ctx, GinkgoTB())
		Expect(store.MigrateUp(ctx, dbURL)).To(Succeed())
		var err error
		st, err = store.New(ctx, &config.DatabaseConfig{URL: dbURL, MaxConns: 5})
		Expect(err).NotTo(HaveOccurred())

		o, err := st.Queries.CreateOrg(ctx, sqlc.CreateOrgParams{
			Name: "local-attrs-org", DisplayName: "Local Attrs Org", AdminGroupID: "admin-grp",
		})
		Expect(err).NotTo(HaveOccurred())
		orgID = o.ID
	})

	AfterEach(func() {
		st.Close()
		cancel()
	})

	newCollector := func(clusterName, role string) pgtype.UUID {
		cluster, err := st.Queries.UpsertCluster(ctx, clusterName)
		Expect(err).NotTo(HaveOccurred())
		Expect(st.Queries.ClaimCluster(ctx, sqlc.ClaimClusterParams{ID: cluster.ID, OrgID: orgID})).To(Succeed())
		collector, err := st.Queries.UpsertCollector(ctx, sqlc.UpsertCollectorParams{ClusterID: cluster.ID, Role: role})
		Expect(err).NotTo(HaveOccurred())
		return collector.ID
	}

	instance := func(id string, collectorID pgtype.UUID, attrs string, lastSeen time.Time) {
		_, err := st.Queries.UpsertCollectorInstance(ctx, sqlc.UpsertCollectorInstanceParams{
			ID: id, CollectorID: collectorID, Name: id, LocalAttributes: json.RawMessage(attrs),
		})
		Expect(err).NotTo(HaveOccurred())
		// UpsertCollectorInstance always stamps last_seen=now(); a distinct
		// last_seen per instance is needed to exercise "most recent wins", so
		// set it explicitly afterward.
		_, err = st.Pool().Exec(ctx, `UPDATE collector_instances SET last_seen = $2 WHERE id = $1`, id, lastSeen)
		Expect(err).NotTo(HaveOccurred())
	}

	It("returns the most-recently-reporting live instance's attributes per collector", func() {
		collectorID := newCollector("last-seen-wins-cluster", "metrics")
		older := time.Now().Add(-time.Hour)
		newer := time.Now()
		instance("older-instance", collectorID, `{"team":"payments"}`, older)
		instance("newer-instance", collectorID, `{"team":"platform"}`, newer)

		rows, err := st.Queries.ListLatestLocalAttributesByOrg(ctx, orgID)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(1))
		Expect(rows[0].CollectorID).To(Equal(collectorID))
		Expect(rows[0].LocalAttributes).To(MatchJSON(`{"team":"platform"}`))
	})

	It("excludes an unregistered instance even if it was the most recently seen", func() {
		collectorID := newCollector("unregistered-cluster", "metrics")
		instance("live-instance", collectorID, `{"team":"platform"}`, time.Now().Add(-time.Hour))
		instance("gone-instance", collectorID, `{"team":"payments"}`, time.Now())
		_, err := st.Pool().Exec(ctx, `UPDATE collector_instances SET unregistered_at = now() WHERE id = $1`, "gone-instance")
		Expect(err).NotTo(HaveOccurred())

		rows, err := st.Queries.ListLatestLocalAttributesByOrg(ctx, orgID)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(1))
		Expect(rows[0].LocalAttributes).To(MatchJSON(`{"team":"platform"}`))
	})

	It("scopes to the requested org only", func() {
		inOrgCollector := newCollector("scoped-cluster", "metrics")
		instance("in-org-instance", inOrgCollector, `{"team":"platform"}`, time.Now())

		otherOrg, err := st.Queries.CreateOrg(ctx, sqlc.CreateOrgParams{
			Name: "other-org", DisplayName: "Other Org", AdminGroupID: "other-admin-grp",
		})
		Expect(err).NotTo(HaveOccurred())
		otherCluster, err := st.Queries.UpsertCluster(ctx, "other-org-cluster")
		Expect(err).NotTo(HaveOccurred())
		Expect(st.Queries.ClaimCluster(ctx, sqlc.ClaimClusterParams{ID: otherCluster.ID, OrgID: otherOrg.ID})).To(Succeed())
		otherCollector, err := st.Queries.UpsertCollector(ctx, sqlc.UpsertCollectorParams{ClusterID: otherCluster.ID, Role: "metrics"})
		Expect(err).NotTo(HaveOccurred())
		instance("other-org-instance", otherCollector.ID, `{"team":"other"}`, time.Now())

		rows, err := st.Queries.ListLatestLocalAttributesByOrg(ctx, orgID)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(1))
		Expect(rows[0].CollectorID).To(Equal(inOrgCollector))
	})

	It("omits a collector with no live reporting instance, without erroring", func() {
		newCollector("no-instances-cluster", "metrics")
		rows, err := st.Queries.ListLatestLocalAttributesByOrg(ctx, orgID)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(BeEmpty())
	})
})

var _ = Describe("ListLatestLocalAttributesByOrg at scale", Label("integration"), func() {
	// LABEL-MATCHING-PLAN.md §6 Phase 2 step 1's explicitly called-out N+1
	// risk: this must be one round trip regardless of collector count, not
	// one query per collector. ~1000 collectors, each with one live
	// instance, seeded via a single bulk SQL statement (not 1000 sqlc calls)
	// so the test itself stays fast -- only the query being measured needs
	// to be cheap, not the seeding.
	It("issues exactly one query for ~1000 collectors, not one per collector", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dbURL := sharedPG.IsolatedDB(ctx, GinkgoTB())
		Expect(store.MigrateUp(ctx, dbURL)).To(Succeed())

		counter := &queryCounter{}
		poolCfg, err := pgxpool.ParseConfig(dbURL)
		Expect(err).NotTo(HaveOccurred())
		poolCfg.ConnConfig.Tracer = counter
		pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
		Expect(err).NotTo(HaveOccurred())
		defer pool.Close()
		queries := sqlc.New(pool)

		var orgID pgtype.UUID
		Expect(pool.QueryRow(ctx,
			`INSERT INTO orgs (name, display_name, admin_group_id) VALUES ('scale-org', 'Scale Org', 'admin-grp') RETURNING id`,
		).Scan(&orgID)).To(Succeed())

		const collectorCount = 1000
		_, err = pool.Exec(ctx, `
			WITH new_clusters AS (
				INSERT INTO clusters (id, name, org_id)
				SELECT gen_random_uuid(), 'scale-cluster-' || gs, $1
				FROM generate_series(1, $2) AS gs
				RETURNING id
			), new_collectors AS (
				INSERT INTO collectors (id, cluster_id, role)
				SELECT gen_random_uuid(), id, 'metrics' FROM new_clusters
				RETURNING id
			)
			INSERT INTO collector_instances (id, collector_id, name, local_attributes, last_seen)
			SELECT gen_random_uuid()::text, id, 'instance-' || id, jsonb_build_object('team', 'platform'), now()
			FROM new_collectors
		`, orgID, collectorCount)
		Expect(err).NotTo(HaveOccurred())

		counter.Reset()
		rows, err := queries.ListLatestLocalAttributesByOrg(ctx, orgID)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(collectorCount))
		Expect(counter.Value()).To(Equal(1), "ListLatestLocalAttributesByOrg must be a single round trip regardless of collector count")
	})
})
