// Driver for fixture-style gazelle tests.
//
// Each subdirectory of testdata/ is treated as one test case. We run the
// project's gazelle_binary inside it and compare the generated BUILD.bazel
// against BUILD.out, plus optional expected{Stdout,Stderr,ExitCode}.txt files.
// Heavy lifting lives in `bazel-gazelle/testtools.TestGazelleGenerationOnPath`,
// which is the same harness gazelle uses for its own language plugins.
//
// Conventions match rules_python's gazelle/python/testdata so existing fixture
// authors recognize the layout, but the runner uses gazelle's stock helper
// instead of rules_python's hand-rolled python_test.go.
package gazelle_test_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bazelbuild/bazel-gazelle/testtools"
	"github.com/bazelbuild/rules_go/go/runfiles"
)

var gazelleBinaryPath = flag.String(
	"gazelle_binary",
	"",
	"rlocationpath of the gazelle_binary under test (passed in by the gazelle_tests macro).",
)

// testdataRelative is where fixtures live relative to the workspace root. Used
// only for the UPDATE_SNAPSHOTS hint testtools prints on failure.
const testdataRelative = "py/gazelle_test/testdata"

func TestFixtures(t *testing.T) {
	if *gazelleBinaryPath == "" {
		t.Fatal("-gazelle_binary flag is required (set by the gazelle_tests macro)")
	}
	gazelleBin, err := runfiles.Rlocation(*gazelleBinaryPath)
	if err != nil {
		t.Fatalf("resolve gazelle binary: %v", err)
	}

	testdataRoot, err := runfiles.Rlocation(filepath.Join(
		os.Getenv("TEST_WORKSPACE"), testdataRelative))
	if err != nil {
		t.Fatalf("resolve testdata root: %v", err)
	}

	entries, err := os.ReadDir(testdataRoot)
	if err != nil {
		t.Fatalf("read testdata root %q: %v", testdataRoot, err)
	}

	any := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		any = true
		testtools.TestGazelleGenerationOnPath(t, &testtools.TestGazelleGenerationArgs{
			Name:                 e.Name(),
			TestDataPathAbsolute: filepath.Join(testdataRoot, e.Name()),
			TestDataPathRelative: testdataRelative,
			GazelleBinaryPath:    gazelleBin,
			Timeout:              30 * time.Second,
		})
	}
	if !any {
		t.Fatalf("no fixture directories under %q", testdataRoot)
	}
}
