package py

import (
	"reflect"
	"testing"

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
		"main.py":          "main",
		"helpers.py":       "helpers",
		"utils/h.py":       "h",
		"types.pyi":        "types",
		"foo_test.py":      "foo_test",
		"tests/api_t.py":   "api_t",
		"_internal_x.py":   "_internal_x",
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
	res := generateAggregateRules(cfg, "pkg", specs, results, annotations{})
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
	res := generateAggregateRules(cfg, "proj", specs, results, annotations{})
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
	res := generateAggregateRules(cfg, "pkg", specs, results, annotations{})
	if len(res.Gen) != 0 {
		t.Errorf("want no rules, got %d", len(res.Gen))
	}
}

// TestGenerateAggregateRules_SkipEmptyInitDirectiveOff verifies opt-in
// semantics: with the directive off (default), even empty __init__.py files
// stay in srcs and the rule is emitted.
func TestGenerateAggregateRules_SkipEmptyInitDirectiveOff(t *testing.T) {
	cfg := newPyConfig() // skipEmptyInit defaults to false
	specs := []FileSpec{{RelPath: "pkg/__init__.py"}}
	results := map[string]FileImports{"pkg/__init__.py": {IsEmpty: true}}
	res := generateAggregateRules(cfg, "pkg", specs, results, annotations{})
	if len(res.Gen) != 1 {
		t.Fatalf("want 1 rule when directive off, got %d", len(res.Gen))
	}
	if got := snapshot(res.Gen[0]); !reflect.DeepEqual(got.srcs, []string{"__init__.py"}) {
		t.Errorf("library srcs = %v, want [__init__.py]", got.srcs)
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
	res := generateAggregateRules(cfg, "pkg", specs, nil, annotations{})
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
	res := generateAggregateRules(cfg, "tests", specs, nil, annotations{})
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
	res := generateAggregateRules(cfg, "pkg", specs, nil, annotations{})
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
	res := generatePerFileRules(cfg, "pkg", specs, nil, annotations{})
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
