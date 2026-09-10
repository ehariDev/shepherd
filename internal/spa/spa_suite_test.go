package spa_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSPA(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SPA Suite")
}
