package simulate_test

import (
	"go/build"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// W2-S6: internal/simulate is meant to stay a pure transform package (graph
// in, rewritten graph out) with no knowledge of how a run gets claimed,
// persisted, or validated. The DB-polling RunWorker (which does need
// store/config/validate/pgxpool) lives in internal/simulate/worker instead —
// see that package for the corresponding "still depends on everything it
// needs" guard.
var _ = Describe("package dependencies", func() {
	It("the transform package has no database, config or validator dependency", func() {
		pkg, err := build.Import("shepherd/internal/simulate", "", 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(pkg.Imports).NotTo(ContainElements(
			"shepherd/internal/store",
			"shepherd/internal/store/sqlc",
			"shepherd/internal/config",
			"shepherd/internal/validate",
			"github.com/jackc/pgx/v5/pgxpool",
		), "internal/simulate must stay a pure transform package; move DB/config/validator-dependent code into internal/simulate/worker")
	})
})
