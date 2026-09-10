package spa_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"shepherd/internal/spa"
)

var _ = Describe("Handler", func() {
	var h http.Handler

	BeforeEach(func() {
		h = spa.Handler()
	})

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	It("serves index.html for the root path", func() {
		rec := get("/")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("text/html"))
		Expect(rec.Header().Get("Cache-Control")).To(Equal("no-cache"))
		Expect(rec.Body.String()).NotTo(BeEmpty())
	})

	It("falls back to index.html, 200 no-cache, for an unknown client-side route (spa.go:73-79)", func() {
		root := get("/")
		route := get("/pipelines/abc-123")

		Expect(route.Code).To(Equal(http.StatusOK))
		Expect(route.Header().Get("Content-Type")).To(ContainSubstring("text/html"))
		Expect(route.Header().Get("Cache-Control")).To(Equal("no-cache"))
		Expect(route.Body.String()).To(Equal(root.Body.String()), "the fallback body must equal the real index.html body")
	})

	It("404s an unknown asset path instead of falling back to index.html", func() {
		rec := get("/assets/does-not-exist.js")
		Expect(rec.Code).To(Equal(http.StatusNotFound))
		Expect(rec.Body.String()).To(ContainSubstring("not_found"))
		// NOT asserted: Content-Type. spa.go:62-63 sets it to "application/json"
		// immediately before calling http.Error(w, ...), and http.Error
		// unconditionally overwrites Content-Type to "text/plain; charset=utf-8"
		// (net/http docs: "Error ... sets Content-Type to text/plain..."), so the
		// header actually sent never matches the JSON body. That's a real defect
		// in spa.go, which is outside this step's territory — recorded as a
		// finding rather than fixed or asserted-around here.
	})

	It("serves a real, embedded asset with long-lived immutable caching, not the SPA fallback", func() {
		// The asset name is content-hashed and changes on every web build, so
		// take it from the served index.html rather than pinning a filename.
		m := regexp.MustCompile(`/assets/[A-Za-z0-9_.-]+\.js`).FindString(get("/").Body.String())
		Expect(m).NotTo(BeEmpty(), "index.html references no /assets/*.js")
		rec := get(m)
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Cache-Control")).To(Equal("public, max-age=31536000, immutable"))
	})
})
