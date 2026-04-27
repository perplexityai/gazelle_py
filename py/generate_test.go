package py

import (
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
		{"*_test.py", "foo_test.py", true},
		{"*_test.py", "foo.py", false},
		{"test_*.py", "test_foo.py", true},
		{"test_*.py", "foo_test.py", false},
		{"tests/**", "tests/foo.py", true},
		{"tests/**", "tests/sub/foo.py", true},
		{"tests/**", "src/tests/foo.py", false},
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
