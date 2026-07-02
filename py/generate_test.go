package py

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

func TestIsPythonFile(t *testing.T) {
	cfg := newPyConfig()
	cases := map[string]bool{
		"a.py":      true,
		"a.pyi":     false, // not in defaults
		"a.js":      false,
		"a.json":    false,
		"a_test.py": true,
	}
	for name, want := range cases {
		if got := isPythonFile(name, cfg); got != want {
			t.Errorf("isPythonFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsPythonFile_CustomExtensions(t *testing.T) {
	cfg := newPyConfig()
	cfg.extensions = append(cfg.extensions, ".pyi")
	if !isPythonFile("foo.pyi", cfg) {
		t.Errorf("expected .pyi to be recognized after directive")
	}
}

func TestIsTestFile_DefaultPatterns(t *testing.T) {
	cfg := newPyConfig()
	cases := map[string]bool{
		"foo_test.py":    true,
		"test_foo.py":    true,
		"tests/index.py": true,
		"test/main.py":   true,
		"src/foo.py":     false,
		"foo.py":         false,
		"foo_spec.py":    false, // not in default patterns
	}
	for name, want := range cases {
		if got := isTestFile(name, cfg); got != want {
			t.Errorf("isTestFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsTestFile_CustomPatterns(t *testing.T) {
	cfg := newPyConfig()
	cfg.testPatterns = append(cfg.testPatterns, "*_spec.py")
	if !isTestFile("foo_spec.py", cfg) {
		t.Errorf("custom *_spec.py pattern not picked up")
	}
}

func TestMatchTestPattern(t *testing.T) {
	cases := []struct {
		pattern string
		name    string
		want    bool
	}{
		// Single-segment globs.
		{"*_test.py", "foo_test.py", true},
		{"*_test.py", "foo.py", false},
		// `*` does NOT span path separators (doublestar / glob semantics).
		// To match across directories, callers use `**/*_test.py`.
		{"*_test.py", "pkg/foo_test.py", false},
		{"test_*.py", "test_foo.py", true},
		{"test_*.py", "foo_test.py", false},

		// `<dir>/**` — anything under <dir>, no leading parent allowed.
		{"tests/**", "tests/foo.py", true},
		{"tests/**", "tests/sub/foo.py", true},
		{"tests/**", "src/tests/foo.py", false},

		// `**/<file>` — any leading directory, including none.
		{"**/test_*.py", "test_foo.py", true},
		{"**/test_*.py", "pkg/test_foo.py", true},
		{"**/test_*.py", "pkg/sub/test_foo.py", true},
		{"**/test_*.py", "test.py", false},
		{"**/test_*.py", "pkg/sub/foo.py", false},
		{"**/*_test.py", "foo_test.py", true},
		{"**/*_test.py", "pkg/foo_test.py", true},
		{"**/*_test.py", "pkg/sub/foo.py", false},
		{"**/conftest.py", "conftest.py", true},
		{"**/conftest.py", "pkg/conftest.py", true},
		{"**/conftest.py", "pkg/sub/other.py", false},

		// `**/<dir>/**/<file>` — full path-spanning middle.
		{"**/test/**/*.py", "pkg/test/sub/foo.py", true},
		{"**/test/**/*.py", "test/foo.py", true},
		{"**/test/**/*.py", "pkg/test/foo.py", true},
		{"**/test/**/*.py", "pkg/foo.py", false},
		{"**/tests/**/*.py", "tests/integ/foo.py", true},

		// Literal pattern.
		{"foo.py", "foo.py", true},
		{"foo.py", "bar.py", false},
	}
	for _, c := range cases {
		got := matchTestPattern(c.pattern, c.name)
		if got != c.want {
			t.Errorf("matchTestPattern(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestResolveRuleNames(t *testing.T) {
	cases := []struct {
		name     string
		cfg      *pyConfig
		rel      string
		wantLib  string
		wantTest string
	}{
		{
			name:     "default uses package basename",
			cfg:      newPyConfig(),
			rel:      "apps/server",
			wantLib:  "server",
			wantTest: "server_test",
		},
		{
			name:     "deeply nested uses leaf basename",
			cfg:      newPyConfig(),
			rel:      "packages/utils/math/deep",
			wantLib:  "deep",
			wantTest: "deep_test",
		},
		{
			name:     "repo root falls back to literal lib/test",
			cfg:      newPyConfig(),
			rel:      "",
			wantLib:  "lib",
			wantTest: "test",
		},
		{
			name: "directive overrides win",
			cfg: func() *pyConfig {
				c := newPyConfig()
				c.libraryName = "src"
				c.testName = "spec"
				return c
			}(),
			rel:      "packages/foo",
			wantLib:  "src",
			wantTest: "spec",
		},
		{
			name: "package_name placeholder expands",
			cfg: func() *pyConfig {
				c := newPyConfig()
				c.libraryName = "$package_name$_lib"
				c.testName = "$package_name$_unittest"
				return c
			}(),
			rel:      "apps/server",
			wantLib:  "server_lib",
			wantTest: "server_unittest",
		},
		{
			name: "placeholder at repo root falls back to defaults",
			cfg: func() *pyConfig {
				c := newPyConfig()
				c.libraryName = "$package_name$"
				c.testName = "$package_name$_test"
				return c
			}(),
			rel:      "",
			wantLib:  "lib",
			wantTest: "test",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lib, test := resolveRuleNames(c.cfg, c.rel)
			if lib != c.wantLib {
				t.Errorf("lib = %q, want %q", lib, c.wantLib)
			}
			if test != c.wantTest {
				t.Errorf("test = %q, want %q", test, c.wantTest)
			}
		})
	}
}

func TestPkgRelativePath(t *testing.T) {
	cases := []struct {
		workspace, pkg, want string
	}{
		{"apps/server/main.py", "apps/server", "main.py"},
		{"apps/server/utils/h.py", "apps/server", "utils/h.py"},
		{"main.py", "", "main.py"},
		// Defensive: when the spec doesn't share the package prefix (shouldn't
		// happen in practice), we return the workspace-relative path unchanged
		// rather than producing an absolute-looking string.
		{"other/main.py", "apps/server", "other/main.py"},
	}
	for _, c := range cases {
		got := pkgRelativePath(c.workspace, c.pkg)
		if got != c.want {
			t.Errorf("pkgRelativePath(%q, %q) = %q, want %q", c.workspace, c.pkg, got, c.want)
		}
	}
}

func TestPerFileRuleName(t *testing.T) {
	cases := map[string]string{
		"main.py":        "main",
		"helpers.py":     "helpers",
		"utils/h.py":     "h",
		"types.pyi":      "types",
		"foo_test.py":    "foo_test",
		"tests/api_t.py": "api_t",
		"_internal_x.py": "_internal_x",
	}
	for in, want := range cases {
		if got := perFileRuleName(in); got != want {
			t.Errorf("perFileRuleName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAllEmptyInits(t *testing.T) {
	// Emptiness is computed in the rust extractor and arrives via FileImports.
	// These tests stub `results` directly to exercise the Go-side branching;
	// AST coverage lives in crates/import_extractor/src/py.rs unit tests.
	t.Run("lone empty init", func(t *testing.T) {
		specs := []FileSpec{{RelPath: "pkg/__init__.py"}}
		results := map[string]FileImports{"pkg/__init__.py": {IsEmpty: true}}
		if !allEmptyInits([]string{"__init__.py"}, specs, "pkg", results) {
			t.Error("want true for single empty __init__.py")
		}
	})

	t.Run("multiple empty inits in project-mode rollup", func(t *testing.T) {
		// Project mode rolls subtrees up into one library; if every src is
		// an empty __init__.py the rule is still useless and should be skipped.
		specs := []FileSpec{
			{RelPath: "proj/__init__.py"},
			{RelPath: "proj/sub/__init__.py"},
		}
		results := map[string]FileImports{
			"proj/__init__.py":     {IsEmpty: true},
			"proj/sub/__init__.py": {IsEmpty: true},
		}
		if !allEmptyInits([]string{"__init__.py", "sub/__init__.py"}, specs, "proj", results) {
			t.Error("want true when every src is an empty __init__.py")
		}
	})

	t.Run("empty init alongside real code keeps the rule", func(t *testing.T) {
		// Load-bearing: relative imports (`from . import x`) require __init__.py
		// to ship in the same py_library as x.py. So when there are siblings we
		// must keep the rule and keep the __init__.py in srcs.
		specs := []FileSpec{
			{RelPath: "pkg/__init__.py"},
			{RelPath: "pkg/x.py"},
		}
		results := map[string]FileImports{
			"pkg/__init__.py": {IsEmpty: true},
			"pkg/x.py":        {IsEmpty: false},
		}
		if allEmptyInits([]string{"__init__.py", "x.py"}, specs, "pkg", results) {
			t.Error("want false when __init__.py has real-code siblings")
		}
	})

	t.Run("non-empty init", func(t *testing.T) {
		specs := []FileSpec{{RelPath: "pkg/__init__.py"}}
		results := map[string]FileImports{"pkg/__init__.py": {IsEmpty: false}}
		if allEmptyInits([]string{"__init__.py"}, specs, "pkg", results) {
			t.Error("want false for non-empty __init__.py")
		}
	})

	t.Run("non-init lone src", func(t *testing.T) {
		specs := []FileSpec{{RelPath: "pkg/x.py"}}
		results := map[string]FileImports{"pkg/x.py": {IsEmpty: false}}
		if allEmptyInits([]string{"x.py"}, specs, "pkg", results) {
			t.Error("want false for non-init lone source")
		}
	})

	t.Run("empty libSrcs", func(t *testing.T) {
		if allEmptyInits(nil, nil, "pkg", nil) {
			t.Error("want false for empty libSrcs")
		}
	})

	t.Run("missing result is treated as non-empty", func(t *testing.T) {
		// Belt-and-braces: if the parser cache doesn't have the file (e.g. a
		// transient parse error), don't suppress the rule.
		specs := []FileSpec{{RelPath: "pkg/__init__.py"}}
		if allEmptyInits([]string{"__init__.py"}, specs, "pkg", nil) {
			t.Error("want false when result is missing")
		}
	})
}

// TestGenerateAggregateRules_SkipEmptyInitMixed verifies that an empty
// __init__.py sitting alongside real code stays in the library's srcs.
// Stripping it would break `from . import app` style relative imports
// because the package marker would no longer ship with the rule.
func TestGenerateAggregateRules_SkipEmptyInitMixed(t *testing.T) {
	cfg := newPyConfig()
	cfg.skipEmptyInit = true
	specs := []FileSpec{
		{RelPath: "pkg/__init__.py"},
		{RelPath: "pkg/app.py"},
	}
	results := map[string]FileImports{
		"pkg/__init__.py": {IsEmpty: true},
		"pkg/app.py":      {IsEmpty: false},
	}
	res := generateAggregateRules(cfg, nil, "pkg", specs, results, nil, false)
	if len(res.Gen) != 1 {
		t.Fatalf("want 1 rule, got %d", len(res.Gen))
	}
	got := snapshot(res.Gen[0])
	if !reflect.DeepEqual(got.srcs, []string{"__init__.py", "app.py"}) {
		t.Errorf("library srcs = %v, want [__init__.py app.py] (empty __init__.py must remain so relative imports work)", got.srcs)
	}
}

// TestGenerateAggregateRules_SkipEmptyInitProjectRollup covers project-mode
// rollups: when every src in a multi-file rule is an empty __init__.py
// (parent + nested subpackages with nothing real), the directive suppresses
// the rule.
func TestGenerateAggregateRules_SkipEmptyInitProjectRollup(t *testing.T) {
	cfg := newPyConfig()
	cfg.skipEmptyInit = true
	specs := []FileSpec{
		{RelPath: "proj/__init__.py"},
		{RelPath: "proj/sub/__init__.py"},
	}
	results := map[string]FileImports{
		"proj/__init__.py":     {IsEmpty: true},
		"proj/sub/__init__.py": {IsEmpty: true},
	}
	res := generateAggregateRules(cfg, nil, "proj", specs, results, nil, false)
	if len(res.Gen) != 0 {
		t.Errorf("want no rules when every rolled-up src is an empty __init__.py, got %d", len(res.Gen))
	}
}

// TestGenerateAggregateRules_SkipEmptyInitOnly is the simple case where the
// directive suppresses a rule: a package whose sole source is an empty
// __init__.py.
func TestGenerateAggregateRules_SkipEmptyInitOnly(t *testing.T) {
	cfg := newPyConfig()
	cfg.skipEmptyInit = true
	specs := []FileSpec{{RelPath: "pkg/__init__.py"}}
	results := map[string]FileImports{"pkg/__init__.py": {IsEmpty: true}}
	res := generateAggregateRules(cfg, nil, "pkg", specs, results, nil, false)
	if len(res.Gen) != 0 {
		t.Errorf("want no rules, got %d", len(res.Gen))
	}
}

// TestGenerateAggregateRules_SkipEmptyInitTestsOnly covers the project-mode
// rollup case where a `tests/` subdirectory contains only an empty
// `__init__.py` and there are no real sources anywhere in the subtree.
// `tests/__init__.py` matches the default `tests/**` pattern, so it lands in
// testSrcs — and without the empty-init check on testSrcs we'd emit a useless
// `py_test` whose sole src is an empty file (the bug observed across cronjob
// packages with v0.8.0's skip_empty_init).
func TestGenerateAggregateRules_SkipEmptyInitTestsOnly(t *testing.T) {
	cfg := newPyConfig()
	cfg.skipEmptyInit = true
	specs := []FileSpec{{RelPath: "proj/tests/__init__.py"}}
	results := map[string]FileImports{"proj/tests/__init__.py": {IsEmpty: true}}
	res := generateAggregateRules(cfg, nil, "proj", specs, results, nil, false)
	if len(res.Gen) != 0 {
		t.Errorf("want no rules when only test src is empty tests/__init__.py, got %d", len(res.Gen))
	}
}

// TestGenerateAggregateRules_SkipEmptyInitTestsAlongsideLib is the mixed case:
// a real library source plus a `tests/__init__.py`-only test bucket. The
// library rule must still emit; only the test rule is suppressed.
func TestGenerateAggregateRules_SkipEmptyInitTestsAlongsideLib(t *testing.T) {
	cfg := newPyConfig()
	cfg.skipEmptyInit = true
	specs := []FileSpec{
		{RelPath: "proj/app.py"},
		{RelPath: "proj/tests/__init__.py"},
	}
	results := map[string]FileImports{
		"proj/app.py":            {IsEmpty: false},
		"proj/tests/__init__.py": {IsEmpty: true},
	}
	res := generateAggregateRules(cfg, nil, "proj", specs, results, nil, false)
	if len(res.Gen) != 1 {
		t.Fatalf("want 1 rule (library only — empty tests/__init__.py suppressed), got %d", len(res.Gen))
	}
	got := snapshot(res.Gen[0])
	if got.kind != defaultLibraryKind {
		t.Errorf("emitted rule kind = %q, want %q", got.kind, defaultLibraryKind)
	}
	if !reflect.DeepEqual(got.srcs, []string{"app.py"}) {
		t.Errorf("library srcs = %v, want [app.py]", got.srcs)
	}
}

// TestGenerateAggregateRules_SkipEmptyInitRealTestKept guards the inverse:
// when at least one test src is real code, the test rule must keep emitting
// (and the empty `tests/__init__.py` must remain in srcs so package-relative
// imports inside the tests subtree continue to resolve).
func TestGenerateAggregateRules_SkipEmptyInitRealTestKept(t *testing.T) {
	cfg := newPyConfig()
	cfg.skipEmptyInit = true
	specs := []FileSpec{
		{RelPath: "proj/tests/__init__.py"},
		{RelPath: "proj/tests/test_app.py"},
	}
	results := map[string]FileImports{
		"proj/tests/__init__.py": {IsEmpty: true},
		"proj/tests/test_app.py": {IsEmpty: false},
	}
	res := generateAggregateRules(cfg, nil, "proj", specs, results, nil, false)
	if len(res.Gen) != 1 {
		t.Fatalf("want 1 rule (test, real src present), got %d", len(res.Gen))
	}
	got := snapshot(res.Gen[0])
	if got.kind != defaultTestKind {
		t.Errorf("emitted rule kind = %q, want %q", got.kind, defaultTestKind)
	}
	if !reflect.DeepEqual(got.srcs, []string{"tests/__init__.py", "tests/test_app.py"}) {
		t.Errorf("test srcs = %v, want [tests/__init__.py tests/test_app.py]", got.srcs)
	}
}

// TestGeneratePerFileRules_SkipEmptyInitTest covers the file-mode counterpart:
// an empty `__init__.py` matched by a test pattern (e.g. `tests/**` in project
// rollup, or a custom pattern that catches `__init__.py`) must not produce a
// per-file py_test rule under skip_empty_init.
func TestGeneratePerFileRules_SkipEmptyInitTest(t *testing.T) {
	cfg := newPyConfig()
	cfg.skipEmptyInit = true
	cfg.testPatterns = append(cfg.testPatterns, "**/__init__.py")
	specs := []FileSpec{{RelPath: "pkg/__init__.py"}}
	results := map[string]FileImports{"pkg/__init__.py": {IsEmpty: true}}
	res := generatePerFileRules(cfg, "pkg", specs, results)
	if len(res.Gen) != 0 {
		t.Errorf("want no rules in file mode for empty __init__.py matched by test pattern, got %d", len(res.Gen))
	}
}

// TestGenerateAggregateRules_SkipEmptyInitDirectiveOff verifies opt-in
// semantics: with the directive off (default), even empty __init__.py files
// stay in srcs and the rule is emitted.
func TestGenerateAggregateRules_SkipEmptyInitDirectiveOff(t *testing.T) {
	cfg := newPyConfig() // skipEmptyInit defaults to false
	specs := []FileSpec{{RelPath: "pkg/__init__.py"}}
	results := map[string]FileImports{"pkg/__init__.py": {IsEmpty: true}}
	res := generateAggregateRules(cfg, nil, "pkg", specs, results, nil, false)
	if len(res.Gen) != 1 {
		t.Fatalf("want 1 rule when directive off, got %d", len(res.Gen))
	}
	if got := snapshot(res.Gen[0]); !reflect.DeepEqual(got.srcs, []string{"__init__.py"}) {
		t.Errorf("library srcs = %v, want [__init__.py]", got.srcs)
	}
}

func TestGenerateAggregateRules_SourceAnnotationsStayWithOwningRule(t *testing.T) {
	cfg := newPyConfig()
	specs := []FileSpec{
		{RelPath: "pkg/__init__.py"},
		{RelPath: "pkg/test_common.py"},
	}
	results := map[string]FileImports{
		"pkg/__init__.py": {
			Modules: []ImportStatement{{ImportPath: "pkg_init_dep", SourceFile: "pkg/__init__.py"}},
		},
		"pkg/test_common.py": {
			Modules: []ImportStatement{{ImportPath: "pytest", SourceFile: "pkg/test_common.py"}},
			Comments: []string{
				"# gazelle:ignore hidden_runtime",
				"# gazelle:include_dep //manual:runtime",
			},
		},
	}
	res := generateAggregateRules(cfg, nil, "pkg", specs, results, nil, false)

	importsByName := map[string]ImportData{}
	for i, r := range res.Gen {
		data, ok := res.Imports[i].(ImportData)
		if !ok {
			t.Fatalf("imports[%d] has type %T, want ImportData", i, res.Imports[i])
		}
		importsByName[r.Name()] = data
	}

	libImports := importsByName["pkg"]
	if !reflect.DeepEqual(libImports.Imports, results["pkg/__init__.py"].Modules) {
		t.Errorf(":pkg Imports = %v, want %v", libImports.Imports, results["pkg/__init__.py"].Modules)
	}
	if len(libImports.IncludeDeps) != 0 {
		t.Errorf(":pkg IncludeDeps = %v, want none", libImports.IncludeDeps)
	}
	if len(libImports.Ignore) != 0 {
		t.Errorf(":pkg Ignore = %v, want none", libImports.Ignore)
	}

	testImports := importsByName["pkg_test"]
	if !reflect.DeepEqual(testImports.TestImports, results["pkg/test_common.py"].Modules) {
		t.Errorf(":pkg_test TestImports = %v, want %v", testImports.TestImports, results["pkg/test_common.py"].Modules)
	}
	if !reflect.DeepEqual(testImports.IncludeDeps, []string{"//manual:runtime"}) {
		t.Errorf(":pkg_test IncludeDeps = %v, want [//manual:runtime]", testImports.IncludeDeps)
	}
	if !testImports.Ignore["hidden_runtime"] {
		t.Errorf(":pkg_test did not receive source ignore annotation")
	}
}

func TestIsConftestAtPackageRoot(t *testing.T) {
	cases := map[string]bool{
		"conftest.py":         true,
		"sub/conftest.py":     false, // belongs to sub-package's BUILD, not ours
		"tests/conftest.py":   false,
		"foo.py":              false,
		"test_conftest.py":    false,
		"conftest_helpers.py": false,
	}
	for name, want := range cases {
		if got := isConftestAtPackageRoot(name); got != want {
			t.Errorf("isConftestAtPackageRoot(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestGenerateAggregateRules_ConftestExtracted verifies the rules_python-style
// conftest layout: conftest.py is pulled out of the main library's srcs and
// emitted as its own `py_library` named "conftest" with `testonly=True`.
//
// Without this, a test's synthesized `<pkg>.conftest` import would resolve to
// the main library, dragging the entire library in as a test dep — which is
// exactly the duplication the dedicated `:conftest` target avoids.
func TestGenerateAggregateRules_ConftestExtracted(t *testing.T) {
	cfg := newPyConfig()
	specs := []FileSpec{
		{RelPath: "pkg/app.py"},
		{RelPath: "pkg/conftest.py"},
		{RelPath: "pkg/app_test.py"},
	}
	res := generateAggregateRules(cfg, nil, "pkg", specs, nil, nil, false)
	if len(res.Gen) != 3 {
		t.Fatalf("want 3 rules (lib, conftest, test), got %d", len(res.Gen))
	}

	byName := map[string]*ruleSnapshot{}
	for _, r := range res.Gen {
		byName[r.Name()] = snapshot(r)
	}

	lib, ok := byName["pkg"]
	if !ok {
		t.Fatalf("missing main library rule; have %v", keys(byName))
	}
	if !reflect.DeepEqual(lib.srcs, []string{"app.py"}) {
		t.Errorf("library srcs = %v, want [app.py] (conftest.py must NOT be here)", lib.srcs)
	}

	conf, ok := byName["conftest"]
	if !ok {
		t.Fatalf("missing :conftest rule; have %v", keys(byName))
	}
	if conf.kind != defaultLibraryKind {
		t.Errorf(":conftest kind = %q, want %q", conf.kind, defaultLibraryKind)
	}
	if !reflect.DeepEqual(conf.srcs, []string{"conftest.py"}) {
		t.Errorf(":conftest srcs = %v, want [conftest.py]", conf.srcs)
	}
	if !conf.testonly {
		t.Errorf(":conftest must set testonly=True")
	}

	if _, ok := byName["pkg_test"]; !ok {
		t.Errorf("missing test rule; have %v", keys(byName))
	}
}

// TestGenerateAggregateRules_ConftestOnly covers a directory whose only Python
// file is conftest.py — we must still emit the :conftest rule (no main lib,
// no test rule). Mirrors a `tests/` package that exists purely to host shared
// fixtures.
func TestGenerateAggregateRules_ConftestOnly(t *testing.T) {
	cfg := newPyConfig()
	specs := []FileSpec{{RelPath: "tests/conftest.py"}}
	res := generateAggregateRules(cfg, nil, "tests", specs, nil, nil, false)
	if len(res.Gen) != 1 {
		t.Fatalf("want exactly 1 rule, got %d", len(res.Gen))
	}
	conf := snapshot(res.Gen[0])
	if conf.name != "conftest" {
		t.Errorf("rule name = %q, want %q", conf.name, "conftest")
	}
	if !conf.testonly {
		t.Errorf("conftest-only rule must set testonly=True")
	}
}

// TestGenerateAggregateRules_NestedConftestStays makes sure conftest.py living
// in a sub-tree stays in the main library's srcs (it'll be extracted when
// gazelle reaches THAT directory's BUILD file). Only the package-root conftest
// gets the dedicated rule.
func TestGenerateAggregateRules_NestedConftestStays(t *testing.T) {
	cfg := newPyConfig()
	specs := []FileSpec{
		{RelPath: "pkg/app.py"},
		{RelPath: "pkg/sub/conftest.py"},
	}
	res := generateAggregateRules(cfg, nil, "pkg", specs, nil, nil, false)
	for _, r := range res.Gen {
		if r.Name() == "conftest" {
			t.Errorf("nested conftest must not produce a :conftest rule at this level")
		}
	}
	if len(res.Gen) != 1 {
		t.Fatalf("want 1 rule (main lib), got %d", len(res.Gen))
	}
	lib := snapshot(res.Gen[0])
	wantSrcs := []string{"app.py", "sub/conftest.py"}
	if !reflect.DeepEqual(lib.srcs, wantSrcs) {
		t.Errorf("lib srcs = %v, want %v", lib.srcs, wantSrcs)
	}
}

// TestGeneratePerFileRules_ConftestTestonly: in `python_generation_mode file`
// every .py becomes its own rule. conftest.py already gets a `:conftest` rule
// (named after the file's basename), but we must additionally mark it
// testonly so it matches the package-mode behavior.
func TestGeneratePerFileRules_ConftestTestonly(t *testing.T) {
	cfg := newPyConfig()
	specs := []FileSpec{
		{RelPath: "pkg/app.py"},
		{RelPath: "pkg/conftest.py"},
	}
	res := generatePerFileRules(cfg, "pkg", specs, nil)
	var conf *ruleSnapshot
	for _, r := range res.Gen {
		if r.Name() == "conftest" {
			conf = snapshot(r)
		}
	}
	if conf == nil {
		t.Fatalf("file mode must emit a :conftest rule")
	}
	if !conf.testonly {
		t.Errorf("file-mode :conftest must set testonly=True")
	}
}

func TestGenerateAggregateRules_HandRolledTargetsPackageModeOnly(t *testing.T) {
	cfg := newPyConfig()
	file := mustLoadBuildFile(t, "pkg", `
load("@rules_python//python:defs.bzl", "py_library")

py_library(
    name = "models",
    srcs = ["models.py"],
)
`)
	specs := []FileSpec{{RelPath: "pkg/models.py"}}
	results := map[string]FileImports{"pkg/models.py": {}}

	res := generateAggregateRules(cfg, nil, "pkg", specs, results, file, false)

	for _, r := range res.Gen {
		if r.Name() == "models" {
			t.Fatalf("hand-rolled :models target must not be managed outside package generation mode")
		}
	}
}

func TestGenerateAggregateRules_ExplicitManagedSrcBeatsBroadHandRolledPattern(t *testing.T) {
	cfg := newPyConfig()
	file := mustLoadBuildFile(t, "pkg/sub", `
load("@rules_python//python:defs.bzl", "py_library")

py_library(
    name = "catch_all",
    file_patterns = ["**/*.py"],
)

py_library(
    name = "sub",
    srcs = ["__init__.py"],
)
`)
	specs := []FileSpec{
		{RelPath: "pkg/sub/__init__.py"},
		{RelPath: "pkg/sub/worker.py"},
	}
	results := map[string]FileImports{
		"pkg/sub/__init__.py": {
			Modules: []ImportStatement{{ImportPath: "explicit.owner.only", SourceFile: "pkg/sub/__init__.py"}},
		},
		"pkg/sub/worker.py": {
			Modules: []ImportStatement{{ImportPath: "catch.all.owner", SourceFile: "pkg/sub/worker.py"}},
		},
	}

	res := generateAggregateRules(cfg, nil, "pkg/sub", specs, results, file, true)

	byName := map[string]*ruleSnapshot{}
	importsByName := map[string]ImportData{}
	for i, r := range res.Gen {
		byName[r.Name()] = snapshot(r)
		data, ok := res.Imports[i].(ImportData)
		if !ok {
			t.Fatalf("imports[%d] has type %T, want ImportData", i, res.Imports[i])
		}
		importsByName[r.Name()] = data
	}

	lib := byName["sub"]
	if lib == nil {
		t.Fatalf("missing generated :sub rule; have %v", keys(byName))
	}
	if !reflect.DeepEqual(lib.srcs, []string{"__init__.py"}) {
		t.Errorf(":sub srcs = %v, want [__init__.py]", lib.srcs)
	}
	if !reflect.DeepEqual(importsByName["sub"].Imports, results["pkg/sub/__init__.py"].Modules) {
		t.Errorf(":sub imports = %v, want %v", importsByName["sub"].Imports, results["pkg/sub/__init__.py"].Modules)
	}
	if !reflect.DeepEqual(importsByName["catch_all"].Imports, results["pkg/sub/worker.py"].Modules) {
		t.Errorf(":catch_all imports = %v, want %v", importsByName["catch_all"].Imports, results["pkg/sub/worker.py"].Modules)
	}
}

func TestGenerateAggregateRules_ExplicitSiblingFileTargetsOwnTheirSources(t *testing.T) {
	cfg := newPyConfig()
	file := mustLoadBuildFile(t, "pkg/sources", `
load("@rules_python//python:defs.bzl", "py_library")

py_library(
    name = "base",
    srcs = ["base.py"],
)

py_library(
    name = "huggingface",
    srcs = ["huggingface.py"],
)
`)
	specs := []FileSpec{
		{RelPath: "pkg/sources/__init__.py"},
		{RelPath: "pkg/sources/base.py"},
		{RelPath: "pkg/sources/huggingface.py"},
	}
	results := map[string]FileImports{
		"pkg/sources/__init__.py": {
			Modules: []ImportStatement{{ImportPath: "pkg.sources.base", SourceFile: "pkg/sources/__init__.py"}},
		},
		"pkg/sources/base.py": {
			Modules: []ImportStatement{{ImportPath: "pyspark", SourceFile: "pkg/sources/base.py"}},
		},
		"pkg/sources/huggingface.py": {
			Modules: []ImportStatement{{ImportPath: "huggingface_hub", SourceFile: "pkg/sources/huggingface.py"}},
		},
	}

	res := generateAggregateRules(cfg, nil, "pkg/sources", specs, results, file, true)

	byName := map[string]*ruleSnapshot{}
	importsByName := map[string]ImportData{}
	for i, r := range res.Gen {
		byName[r.Name()] = snapshot(r)
		data, ok := res.Imports[i].(ImportData)
		if !ok {
			t.Fatalf("imports[%d] has type %T, want ImportData", i, res.Imports[i])
		}
		importsByName[r.Name()] = data
	}

	sources := byName["sources"]
	if sources == nil {
		t.Fatalf("missing generated :sources rule; have %v", keys(byName))
	}
	if !reflect.DeepEqual(sources.srcs, []string{"__init__.py"}) {
		t.Errorf(":sources srcs = %v, want [__init__.py]", sources.srcs)
	}
	if !reflect.DeepEqual(importsByName["sources"].Imports, results["pkg/sources/__init__.py"].Modules) {
		t.Errorf(":sources imports = %v, want %v", importsByName["sources"].Imports, results["pkg/sources/__init__.py"].Modules)
	}
	if !reflect.DeepEqual(importsByName["base"].Imports, results["pkg/sources/base.py"].Modules) {
		t.Errorf(":base imports = %v, want %v", importsByName["base"].Imports, results["pkg/sources/base.py"].Modules)
	}
	if !reflect.DeepEqual(importsByName["huggingface"].Imports, results["pkg/sources/huggingface.py"].Modules) {
		t.Errorf(":huggingface imports = %v, want %v", importsByName["huggingface"].Imports, results["pkg/sources/huggingface.py"].Modules)
	}
}

func TestGenerateHandRolledRules_EmitsParsedExtraTargets(t *testing.T) {
	cfg := newPyConfig()
	file := mustLoadBuildFile(t, "pkg", `
load("@rules_python//python:defs.bzl", "py_library", "py_test")

py_library(
    name = "pkg",
    srcs = ["pkg.py"],
)

py_library(
    name = "models",
    srcs = ["models.py"],
)

py_test(
    name = "models_test",
    srcs = ["models_test.py"],
)

py_library(
    name = "excluded",
    srcs = ["excluded.py"],
)

py_library(
    name = "empty",
)

filegroup(
    name = "data",
    srcs = ["data.json"],
)
`)
	results := map[string]FileImports{
		"pkg/pkg.py": {
			Modules: []ImportStatement{{ImportPath: "should.not.appear", SourceFile: "pkg/pkg.py"}},
		},
		"pkg/models.py": {
			Modules: []ImportStatement{{ImportPath: "requests", SourceFile: "pkg/models.py"}},
			Comments: []string{
				"# gazelle:ignore ignored",
				"# gazelle:include_dep //manual:runtime",
			},
		},
		"pkg/models_test.py": {
			Modules: []ImportStatement{{ImportPath: "pytest", SourceFile: "pkg/models_test.py"}},
		},
	}

	genRules, genImports := generateHandRolledRules(cfg, nil, "pkg", nil, results, file, map[string]bool{
		"pkg":      true,
		"pkg_test": true,
		"conftest": true,
	})

	if len(genRules) != 2 {
		t.Fatalf("want 2 hand-rolled rules, got %d", len(genRules))
	}
	if len(genImports) != len(genRules) {
		t.Fatalf("imports length = %d, want %d", len(genImports), len(genRules))
	}

	byName := map[string]*ruleSnapshot{}
	importsByName := map[string]ImportData{}
	for i, r := range genRules {
		byName[r.Name()] = snapshot(r)
		data, ok := genImports[i].(ImportData)
		if !ok {
			t.Fatalf("imports[%d] has type %T, want ImportData", i, genImports[i])
		}
		importsByName[r.Name()] = data
	}

	models := byName["models"]
	if models == nil {
		t.Fatalf("missing :models; have %v", keys(byName))
	}
	if models.kind != defaultLibraryKind {
		t.Errorf(":models kind = %q, want %q", models.kind, defaultLibraryKind)
	}
	if !reflect.DeepEqual(models.srcs, []string{"models.py"}) {
		t.Errorf(":models srcs = %v, want [models.py]", models.srcs)
	}
	modelsImports := importsByName["models"]
	if !reflect.DeepEqual(modelsImports.Imports, results["pkg/models.py"].Modules) {
		t.Errorf(":models imports = %v, want %v", modelsImports.Imports, results["pkg/models.py"].Modules)
	}
	if len(modelsImports.TestImports) != 0 {
		t.Errorf(":models TestImports = %v, want none", modelsImports.TestImports)
	}
	if !modelsImports.Ignore["ignored"] {
		t.Errorf(":models did not receive ignore annotations")
	}
	if !reflect.DeepEqual(modelsImports.IncludeDeps, []string{"//manual:runtime"}) {
		t.Errorf(":models IncludeDeps = %v, want [//manual:runtime]", modelsImports.IncludeDeps)
	}

	modelsTest := byName["models_test"]
	if modelsTest == nil {
		t.Fatalf("missing :models_test; have %v", keys(byName))
	}
	if modelsTest.kind != defaultTestKind {
		t.Errorf(":models_test kind = %q, want %q", modelsTest.kind, defaultTestKind)
	}
	if !reflect.DeepEqual(modelsTest.srcs, []string{"models_test.py"}) {
		t.Errorf(":models_test srcs = %v, want [models_test.py]", modelsTest.srcs)
	}
	testImports := importsByName["models_test"]
	if !reflect.DeepEqual(testImports.TestImports, results["pkg/models_test.py"].Modules) {
		t.Errorf(":models_test TestImports = %v, want %v", testImports.TestImports, results["pkg/models_test.py"].Modules)
	}
	if len(testImports.Imports) != 0 {
		t.Errorf(":models_test Imports = %v, want none", testImports.Imports)
	}
	if len(testImports.IncludeDeps) != 0 {
		t.Errorf(":models_test IncludeDeps = %v, want none", testImports.IncludeDeps)
	}
	if len(testImports.Ignore) != 0 {
		t.Errorf(":models_test Ignore = %v, want none", testImports.Ignore)
	}

	for _, unexpected := range []string{"pkg", "excluded", "empty", "data"} {
		if byName[unexpected] != nil {
			t.Errorf("unexpected generated rule %q", unexpected)
		}
	}
}

func TestGenerateHandRolledRules_FilePatternExcludesExplicitSiblingSrcs(t *testing.T) {
	cfg := newPyConfig()
	file := mustLoadBuildFile(t, "pkg/sub", `
load("@rules_python//python:defs.bzl", "py_library")

py_library(
    name = "catch_all",
    file_patterns = ["**/*.py"],
)

py_library(
    name = "sub",
    srcs = ["__init__.py"],
)
`)
	specs := []FileSpec{
		{RelPath: "pkg/sub/__init__.py"},
		{RelPath: "pkg/sub/worker.py"},
	}
	results := map[string]FileImports{
		"pkg/sub/__init__.py": {
			Modules: []ImportStatement{{ImportPath: "explicit.owner.only", SourceFile: "pkg/sub/__init__.py"}},
		},
		"pkg/sub/worker.py": {
			Modules: []ImportStatement{{ImportPath: "catch.all.owner", SourceFile: "pkg/sub/worker.py"}},
		},
	}

	genRules, genImports := generateHandRolledRules(cfg, nil, "pkg/sub", specs, results, file, nil)

	importsByName := map[string]ImportData{}
	for i, r := range genRules {
		data, ok := genImports[i].(ImportData)
		if !ok {
			t.Fatalf("imports[%d] has type %T, want ImportData", i, genImports[i])
		}
		importsByName[r.Name()] = data
	}

	catchAllImports := importsByName["catch_all"].Imports
	wantCatchAllImports := results["pkg/sub/worker.py"].Modules
	if !reflect.DeepEqual(catchAllImports, wantCatchAllImports) {
		t.Errorf(":catch_all imports = %v, want %v", catchAllImports, wantCatchAllImports)
	}

	explicitImports := importsByName["sub"].Imports
	wantExplicitImports := results["pkg/sub/__init__.py"].Modules
	if !reflect.DeepEqual(explicitImports, wantExplicitImports) {
		t.Errorf(":sub imports = %v, want %v", explicitImports, wantExplicitImports)
	}
}

func TestGenerateHandRolledRules_MatchesMappedKinds(t *testing.T) {
	cfg := newPyConfig()
	results := map[string]FileImports{
		"pkg/models.py":      {},
		"pkg/models_test.py": {},
	}
	cases := []struct {
		name     string
		libKind  string
		testKind string
		kindMap  map[string]config.MappedKind
	}{
		{
			name:     "direct mapping",
			libKind:  "pplx_python_library",
			testKind: "pplx_python_test",
			kindMap: map[string]config.MappedKind{
				defaultLibraryKind: {KindName: "pplx_python_library"},
				defaultTestKind:    {KindName: "pplx_python_test"},
			},
		},
		{
			name:     "transitive mapping",
			libKind:  "pplx_final_python_library",
			testKind: "pplx_final_python_test",
			kindMap: map[string]config.MappedKind{
				defaultLibraryKind:    {KindName: "pplx_python_library"},
				"pplx_python_library": {KindName: "pplx_final_python_library"},
				defaultTestKind:       {KindName: "pplx_python_test"},
				"pplx_python_test":    {KindName: "pplx_final_python_test"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := mustLoadBuildFile(t, "pkg", fmt.Sprintf(`
load("//tools:python_defs.bzl", "pplx_final_python_library", "pplx_final_python_test", "pplx_python_library", "pplx_python_test")

%s(
    name = "models",
    srcs = ["models.py"],
)

%s(
    name = "models_test",
    srcs = ["models_test.py"],
)
`, tc.libKind, tc.testKind))
			c := &config.Config{KindMap: tc.kindMap}

			genRules, _ := generateHandRolledRules(cfg, c, "pkg", nil, results, file, nil)

			if len(genRules) != 2 {
				t.Fatalf("want 2 hand-rolled rules, got %d", len(genRules))
			}
			byName := map[string]*ruleSnapshot{}
			for _, r := range genRules {
				byName[r.Name()] = snapshot(r)
			}
			if byName["models"] == nil || byName["models"].kind != defaultLibraryKind {
				t.Errorf(":models = %+v, want generated %s rule", byName["models"], defaultLibraryKind)
			}
			if byName["models_test"] == nil || byName["models_test"].kind != defaultTestKind {
				t.Errorf(":models_test = %+v, want generated %s rule", byName["models_test"], defaultTestKind)
			}
		})
	}
}

func mustLoadBuildFile(t *testing.T, pkg, data string) *rule.File {
	t.Helper()
	file, err := rule.LoadData("BUILD.bazel", pkg, []byte(data))
	if err != nil {
		t.Fatalf("load BUILD data: %v", err)
	}
	return file
}

// ruleSnapshot captures the bits of a *rule.Rule we want to assert on. We
// can't compare *rule.Rule values directly (unexported state), so we read
// out the public attrs we care about.
type ruleSnapshot struct {
	kind     string
	name     string
	srcs     []string
	testonly bool
}

func snapshot(r *rule.Rule) *ruleSnapshot {
	return &ruleSnapshot{
		kind:     r.Kind(),
		name:     r.Name(),
		srcs:     r.AttrStrings("srcs"),
		testonly: r.AttrBool("testonly"),
	}
}

func keys(m map[string]*ruleSnapshot) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestCollectSrcs(t *testing.T) {
	cfg := newPyConfig()
	files := []string{
		"main.py",
		"helper.py",
		"types.py",
		"main_test.py",
		"tests/integration.py",
		"README.md",
		"pyproject.toml",
	}
	libs, tests := collectSrcs(files, cfg)

	wantLibs := []string{"helper.py", "main.py", "types.py"}
	wantTests := []string{"main_test.py", "tests/integration.py"}
	if !reflect.DeepEqual(libs, wantLibs) {
		t.Errorf("libs = %v, want %v", libs, wantLibs)
	}
	if !reflect.DeepEqual(tests, wantTests) {
		t.Errorf("tests = %v, want %v", tests, wantTests)
	}
}
