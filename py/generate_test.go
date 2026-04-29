package py

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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

func TestIsEmptyPython(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		body    string
		want    bool
	}{
		{"truly_empty.py", "", true},
		{"only_blanks.py", "\n\n   \n", true},
		{"only_comments.py", "# header\n# another\n\n", true},
		{"docstring.py", `"""mod doc"""`, false}, // a docstring is real code
		{"has_pass.py", "pass\n", false},
		{"has_import.py", "import os\n", false},
	}
	for _, c := range cases {
		path := filepath.Join(dir, c.name)
		if err := os.WriteFile(path, []byte(c.body), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := isEmptyPython(path); got != c.want {
			t.Errorf("isEmptyPython(%q body=%q) = %v, want %v", c.name, c.body, got, c.want)
		}
	}
}

func TestIsEmptyInitOnly(t *testing.T) {
	dir := t.TempDir()
	emptyInit := filepath.Join(dir, "__init__.py")
	if err := os.WriteFile(emptyInit, []byte("# blank package marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specs := []FileSpec{{Path: emptyInit, RelPath: "pkg/__init__.py"}}
	if !isEmptyInitOnly([]string{"__init__.py"}, specs, "pkg") {
		t.Errorf("expected empty-only __init__.py to be detected")
	}
	// More than one src disqualifies — even if __init__.py itself is empty.
	if isEmptyInitOnly([]string{"__init__.py", "x.py"}, specs, "pkg") {
		t.Errorf("multi-src package should not be flagged")
	}
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
