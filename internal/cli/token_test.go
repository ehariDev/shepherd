package cli

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
)

// validFixedUUID is a syntactically valid UUID for the --id flag; it never
// needs to reach the database in these cases because the dev-mode guard
// (or, for the last case, an unreachable database) is what the test proves.
const validFixedUUID = "11111111-1111-4111-8111-111111111111"

var _ = Describe("token create flag parsing", func() {
	BeforeEach(func() {
		// Reset the package-level flag vars AND the underlying pflag.Flag
		// state between cases: tokenCreateCmd is a package-level *cobra.Command
		// reused across every case, and pflag does not reset a flag's Changed
		// bit or Value on a fresh Parse — only flags actually present in the
		// next argv get touched, so a value set by one case would otherwise
		// leak into the next.
		tokenName = ""
		tokenSecret = ""
		tokenID = ""
		tokenCreateCmd.Flags().VisitAll(func(f *pflag.Flag) {
			f.Changed = false
			_ = f.Value.Set(f.DefValue) //nolint:errcheck // resetting a flag to its own default cannot fail
		})

		GinkgoT().Setenv("SHEPHERD_DEV_ALLOW_STATIC_TOKEN", "")
		// A reachable-looking config so a case that gets past the dev-mode
		// guard fails at the database connection (fast: connection refused on
		// a port nothing listens on) rather than at config validation — that
		// distinguishes "the guard let it through" from "config.Load errored
		// first" for the last case below.
		GinkgoT().Setenv("SHEPHERD_DATABASE_URL", "postgres://user:pass@127.0.0.1:1/db?sslmode=disable")
		GinkgoT().Setenv("SHEPHERD_SECURITY_ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	})

	runTokenCreate := func(args ...string) error {
		rootCmd.SilenceUsage = true
		rootCmd.SilenceErrors = true
		rootCmd.SetArgs(append([]string{"token", "create"}, args...))
		return rootCmd.Execute()
	}

	It("requires --name", func() {
		err := runTokenCreate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("name"))
	})

	It("refuses --secret outside dev mode (token.go:63)", func() {
		err := runTokenCreate("--name", "x", "--secret", "fixed-secret")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--secret flag is only allowed when SHEPHERD_DEV_ALLOW_STATIC_TOKEN=true"))
	})

	It("refuses --id outside dev mode", func() {
		err := runTokenCreate("--name", "x", "--id", validFixedUUID)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--id flag is only allowed when SHEPHERD_DEV_ALLOW_STATIC_TOKEN=true"))
	})

	It("allows --secret and --id once SHEPHERD_DEV_ALLOW_STATIC_TOKEN=true, and reaches the database instead of the guard", func() {
		GinkgoT().Setenv("SHEPHERD_DEV_ALLOW_STATIC_TOKEN", "true")
		err := runTokenCreate("--name", "x", "--secret", "fixed-secret", "--id", validFixedUUID)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring("only allowed"))
		Expect(err.Error()).To(ContainSubstring("connecting to database"))
	})
})
