// Package repocheck holds repository-shape guards: Ginkgo specs that parse the
// Makefile, the GitHub workflows and other committed configuration and assert
// the properties CI relies on. They run in CI's guards job, so a regression in
// build tooling fails a PR the same way a code regression does.
//
// Every spec here was written red first against the tree it guards; the spec
// comment records what the tree looked like when it failed.
package repocheck_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRepocheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "repocheck suite")
}
