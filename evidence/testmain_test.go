package evidence

import (
	"os"
	"testing"

	"github.com/afterglow-labs/afterglow-cybersecurity/internal/dbtest"
)

// TestMain routes the evidence test suite through dbtest.Main, which refuses to
// run against the live database and redirects every downstream resolver at the
// throwaway ARTEX_TEST_DSN database (holding the shared suite advisory lock).
func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}
