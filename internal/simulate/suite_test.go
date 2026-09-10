package simulate_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The transform, policy and fixture specs in this package need no database:
// the RunWorker specs that did moved to internal/simulate/worker (W2-S6),
// so this suite no longer starts a shared Postgres container.
func TestSimulate(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Simulate Suite")
}
