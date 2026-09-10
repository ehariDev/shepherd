package mgmtapi_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"shepherd/internal/auth"
	"shepherd/internal/config"
	"shepherd/internal/store"
	"shepherd/internal/store/sqlc"
)

// rest_roles_test.go proves the REST shim's authoring routes (pipeline
// writes/validate, wizards, visual, simulate) accept an org-editor session,
// not only org-admin — the REST-shim half of D5/D6's org-editor alignment
// (rpc_interceptor.go already gates the same Connect procedures at
// auth.RoleOrgEditor; role_matrix_test.go is the Connect-side proof, and
// simulate_run_rest_test.go carries the REST run-API's RBAC/tenant-isolation
// cases, which this file does not repeat).
var _ = Describe("REST shim — org-editor may author, org-reader may not", Label("integration"), func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		st           *store.Store
		server       *httptest.Server
		orgID        string
		editorCookie *http.Cookie
		readerCookie *http.Cookie
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		dbURL := sharedPG.IsolatedDB(ctx, GinkgoTB())

		var err error
		st, err = store.New(ctx, &config.DatabaseConfig{URL: dbURL, MaxConns: 5}, slog.Default())
		Expect(err).NotTo(HaveOccurred())

		o, err := st.Queries.CreateOrg(ctx, sqlc.CreateOrgParams{
			Name: "rest-roles-org", DisplayName: "REST Roles Org",
			AdminGroupID:  "rest-roles-admin-grp",
			EditorGroupID: pgtype.Text{String: "rest-roles-editor-grp", Valid: true},
			ReaderGroupID: pgtype.Text{String: "rest-roles-reader-grp", Valid: true},
		})
		Expect(err).NotTo(HaveOccurred())
		orgID = o.ID.String()

		editorCookie = &http.Cookie{Name: "shepherd_session", Value: newTestSession(ctx, st, "rest-roles-editor-grp")}
		readerCookie = &http.Cookie{Name: "shepherd_session", Value: newTestSession(ctx, st, "rest-roles-reader-grp")}

		cfg := &config.Config{
			Auth: config.AuthConfig{InsecureCookies: true},
			Simulator: config.SimulatorConfig{
				Enabled: true, MaxNonTerminalPerOrg: 3,
				Duration: 30 * time.Second, MaxDuration: 120 * time.Second,
			},
		}
		authHandler := auth.NewLocalAdmin(cfg, st, slog.Default())
		server = httptest.NewServer(newRESTRouter(st, authHandler, cfg, nil))
	})

	AfterEach(func() {
		server.Close()
		st.Close()
		cancel()
	})

	// W3-4: SimulateService settles at org-editor on every layer (Connect,
	// REST, proto comments, docs) — this is the REST proof.
	It("lets an org editor run a relabel simulation over REST", func() {
		resp := postJSON(server, "/orgs/"+orgID+"/simulate/relabel",
			map[string]any{"rules": []any{}, "sample_targets": []any{}}, editorCookie)
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		Expect(resp.StatusCode).NotTo(Equal(http.StatusForbidden))
	})

	It("denies simulate/relabel over REST for an org reader", func() {
		resp := postJSON(server, "/orgs/"+orgID+"/simulate/relabel",
			map[string]any{"rules": []any{}, "sample_targets": []any{}}, readerCookie)
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
	})

	// W3-3: pipeline writes/validate and WizardService follow the same
	// org-editor floor over REST that Connect already enforces.
	It("lets an org editor validate a pipeline over REST", func() {
		resp := postJSON(server, "/orgs/"+orgID+"/pipelines/validate",
			map[string]any{"name": "p", "contents": ""}, editorCookie)
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		Expect(resp.StatusCode).NotTo(Equal(http.StatusForbidden))
	})

	It("denies pipelines/validate over REST for an org reader", func() {
		resp := postJSON(server, "/orgs/"+orgID+"/pipelines/validate",
			map[string]any{"name": "p", "contents": ""}, readerCookie)
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
	})

	It("lets an org editor list wizards over REST", func() {
		resp := getRequest(server, "/orgs/"+orgID+"/wizards", editorCookie)
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		Expect(resp.StatusCode).NotTo(Equal(http.StatusForbidden))
	})

	It("denies GET /wizards over REST for an org reader", func() {
		resp := getRequest(server, "/orgs/"+orgID+"/wizards", readerCookie)
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
	})

	// W3-3: VisualService (except GraphView, already org-reader) follows
	// the org-editor floor too.
	It("lets an org editor validate a visual graph over REST", func() {
		resp := postJSON(server, "/orgs/"+orgID+"/visual/validate", map[string]any{}, editorCookie)
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		Expect(resp.StatusCode).NotTo(Equal(http.StatusForbidden))
	})

	It("denies visual/validate over REST for an org reader", func() {
		resp := postJSON(server, "/orgs/"+orgID+"/visual/validate", map[string]any{}, readerCookie)
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
	})
})
