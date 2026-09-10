package mgmtapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"shepherd/internal/auth"
	"shepherd/internal/config"
	"shepherd/internal/store"
	"shepherd/internal/store/sqlc"
)

// Regression coverage for W3-8: GetMe's role resolution must apply the same
// empty-group guard authorizeOrgAccess's hasGroup already does (authz.go).
// Before ResolveOrgRole existed, GetMe did its own inline
// slices.Contains(sess.GroupIDs, org.AdminGroupID) with no guard, so an org
// whose admin_group_id is "" (never configured) reported every session
// carrying an empty string in its groups claim as an admin of that org —
// offering admin actions in the UI that the server (authorizeOrgAccess)
// would then correctly refuse.
var _ = Describe("MeService GetMe role resolution", Label("integration"), func() {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		st          *store.Store
		authHandler *auth.Handler
		server      *httptest.Server
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		dbURL := sharedPG.IsolatedDB(ctx, GinkgoTB())

		var err error
		st, err = store.New(ctx, &config.DatabaseConfig{URL: dbURL, MaxConns: 5}, slog.Default())
		Expect(err).NotTo(HaveOccurred())

		cfg := &config.Config{Auth: config.AuthConfig{InsecureCookies: true}}
		authHandler = auth.NewLocalAdmin(cfg, st, slog.Default())
		server = httptest.NewServer(newRPCWiringRouter(st, authHandler, cfg))
	})

	AfterEach(func() {
		server.Close()
		st.Close()
		cancel()
	})

	postConnect := func(procedure string, body map[string]any, cookie *http.Cookie) *http.Response {
		buf, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		req, err := http.NewRequest(http.MethodPost, server.URL+procedure, strings.NewReader(string(buf)))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	decodeBody := func(resp *http.Response) map[string]any {
		defer resp.Body.Close() //nolint:errcheck // test cleanup
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		var payload map[string]any
		Expect(json.Unmarshal(body, &payload)).To(Succeed())
		return payload
	}

	It("does not report admin for an org whose admin_group_id is empty, to a session carrying an empty group", func() {
		org, err := st.Queries.CreateOrg(ctx, sqlc.CreateOrgParams{
			Name: "me-roles-empty-admin-group", DisplayName: "Empty Admin Group", AdminGroupID: "",
		})
		Expect(err).NotTo(HaveOccurred())

		groupsJSON, err := json.Marshal([]string{""})
		Expect(err).NotTo(HaveOccurred())
		sessionID := fmt.Sprintf("me-roles-session-%d", time.Now().UnixNano())
		_, err = st.Queries.CreateSession(ctx, sqlc.CreateSessionParams{
			ID: sessionID, UserOid: "me-roles-user-oid", Email: "me-roles-user@example.com", DisplayName: "Me Roles User",
			GroupIds:   groupsJSON,
			IsAppAdmin: false,
			ExpiresAt:  pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
			Source:     "test",
		})
		Expect(err).NotTo(HaveOccurred())
		cookie := &http.Cookie{Name: "shepherd_session", Value: sessionID}

		resp := postConnect("/shepherd.mgmt.v1.MeService/GetMe", map[string]any{}, cookie)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		payload := decodeBody(resp)

		// protojson omits an empty repeated field entirely rather than
		// emitting `[]`, so "orgs" may be absent from the payload when this
		// session clears no org at all — that absence is itself a pass here,
		// not a shape to assert on.
		orgsRaw, present := payload["orgs"]
		if !present {
			return
		}
		orgs, ok := orgsRaw.([]any)
		Expect(ok).To(BeTrue(), "expected orgs, when present, to be an array")
		for _, e := range orgs {
			entry, ok := e.(map[string]any)
			Expect(ok).To(BeTrue())
			if entry["id"] == org.ID.String() {
				Fail(fmt.Sprintf("org with empty admin_group_id must not appear in the membership list for a session with no real group, got role %q", entry["role"]))
			}
		}
	})
})
