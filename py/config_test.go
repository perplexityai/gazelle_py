package py

import (
	"reflect"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/rule"
)

func TestApplyDirective_Enabled(t *testing.T) {
	cfg := newPyConfig()
	// rules_python's verbatim values.
	applyDirective(cfg, rule.Directive{Key: directiveEnabled, Value: "disabled"}, "")
	if cfg.enabled {
		t.Fatalf("python_extension disabled: cfg.enabled = true")
	}
	applyDirective(cfg, rule.Directive{Key: directiveEnabled, Value: "enabled"}, "")
	if !cfg.enabled {
		t.Fatalf("python_extension enabled: cfg.enabled = false")
	}
	// Bool ergonomic aliases.
	applyDirective(cfg, rule.Directive{Key: directiveEnabled, Value: "false"}, "")
	if cfg.enabled {
		t.Fatalf("python_extension false: cfg.enabled = true")
	}
	applyDirective(cfg, rule.Directive{Key: directiveEnabled, Value: "true"}, "")
	if !cfg.enabled {
		t.Fatalf("python_extension true: cfg.enabled = false")
	}
}

func TestApplyDirective_Strings(t *testing.T) {
	cfg := newPyConfig()
	applyDirective(cfg, rule.Directive{Key: directiveLibraryName, Value: "src"}, "")
	applyDirective(cfg, rule.Directive{Key: directiveTestName, Value: "spec"}, "")
	applyDirective(cfg, rule.Directive{Key: directiveLibraryKind, Value: "my_lib"}, "")
	applyDirective(cfg, rule.Directive{Key: directiveTestKind, Value: "my_test"}, "")
	applyDirective(cfg, rule.Directive{Key: directiveLabelConvention, Value: "@my_pip//{pkg}"}, "")

	if cfg.libraryName != "src" {
		t.Errorf("libraryName = %q", cfg.libraryName)
	}
	if cfg.testName != "spec" {
		t.Errorf("testName = %q", cfg.testName)
	}
	if cfg.libraryKind != "my_lib" {
		t.Errorf("libraryKind = %q", cfg.libraryKind)
	}
	if cfg.testKind != "my_test" {
		t.Errorf("testKind = %q", cfg.testKind)
	}
	if cfg.pipLinkPattern != "@my_pip//{pkg}" {
		t.Errorf("pipLinkPattern = %q", cfg.pipLinkPattern)
	}
}

func TestApplyDirective_Visibility(t *testing.T) {
	cfg := newPyConfig()
	applyDirective(cfg, rule.Directive{Key: directiveVisibility, Value: "//foo:__pkg__ //bar:__pkg__"}, "")
	want := []string{"//foo:__pkg__", "//bar:__pkg__"}
	if !reflect.DeepEqual(cfg.visibility, want) {
		t.Errorf("visibility = %v want %v", cfg.visibility, want)
	}
}

func TestApplyDirective_AppendDirectives(t *testing.T) {
	cfg := newPyConfig()
	applyDirective(cfg, rule.Directive{Key: directiveTestPattern, Value: "*_check.py"}, "")
	applyDirective(cfg, rule.Directive{Key: directiveSourceExtension, Value: ".pyi"}, "")
	applyDirective(cfg, rule.Directive{Key: directiveTestData, Value: "//:fixtures"}, "")

	if !contains(cfg.testPatterns, "*_check.py") {
		t.Errorf("testPatterns missing *_check.py: %v", cfg.testPatterns)
	}
	if !contains(cfg.extensions, ".pyi") {
		t.Errorf("extensions missing .pyi: %v", cfg.extensions)
	}
	if !contains(cfg.testData, "//:fixtures") {
		t.Errorf("testData missing //:fixtures: %v", cfg.testData)
	}

	// Re-applying the same value should not duplicate.
	applyDirective(cfg, rule.Directive{Key: directiveTestPattern, Value: "*_check.py"}, "")
	count := 0
	for _, p := range cfg.testPatterns {
		if p == "*_check.py" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence of *_check.py, got %d", count)
	}
}

func TestApplyDirective_PythonRoot(t *testing.T) {
	cfg := newPyConfig()
	// python_root takes the rel of the BUILD file it's defined in, not whatever
	// value the user types after the directive.
	applyDirective(cfg, rule.Directive{Key: directivePythonRoot, Value: ""}, "backend")
	if cfg.pythonRoot != "backend" {
		t.Fatalf("python_root: cfg.pythonRoot = %q, want %q", cfg.pythonRoot, "backend")
	}
}

func TestApplyDirective_ResolveSiblingImports(t *testing.T) {
	cfg := newPyConfig()
	applyDirective(cfg, rule.Directive{Key: directiveResolveSiblingImports, Value: "true"}, "")
	if !cfg.resolveSiblingImports {
		t.Fatalf("python_resolve_sibling_imports true: cfg.resolveSiblingImports = false")
	}
	applyDirective(cfg, rule.Directive{Key: directiveResolveSiblingImports, Value: "false"}, "")
	if cfg.resolveSiblingImports {
		t.Fatalf("python_resolve_sibling_imports false: cfg.resolveSiblingImports = true")
	}
}

func TestApplyDirective_GenerationMode(t *testing.T) {
	cases := []struct {
		val      string
		rel      string
		wantMode generationModeType
		wantRoot string
	}{
		{"package", "anywhere", generationModePackage, ""},
		{"file", "anywhere", generationModeFile, ""},
		{"project", "backend", generationModeProject, "backend"},
		{"PROJECT", "tools/py", generationModeProject, "tools/py"},
		// Switching back to package wipes the captured project root.
		{"package", "anywhere", generationModePackage, ""},
	}
	cfg := newPyConfig()
	for _, c := range cases {
		applyDirective(cfg, rule.Directive{Key: directiveGenerationMode, Value: c.val}, c.rel)
		if cfg.generationMode != c.wantMode {
			t.Errorf("python_generation_mode=%q rel=%q: mode = %v, want %v", c.val, c.rel, cfg.generationMode, c.wantMode)
		}
		if cfg.projectRoot != c.wantRoot {
			t.Errorf("python_generation_mode=%q rel=%q: projectRoot = %q, want %q", c.val, c.rel, cfg.projectRoot, c.wantRoot)
		}
	}
}

func TestApplyDirective_SkipEmptyInit(t *testing.T) {
	cfg := newPyConfig()
	applyDirective(cfg, rule.Directive{Key: directiveSkipEmptyInit, Value: "true"}, "")
	if !cfg.skipEmptyInit {
		t.Fatalf("python_skip_empty_init true: cfg.skipEmptyInit = false")
	}
	applyDirective(cfg, rule.Directive{Key: directiveSkipEmptyInit, Value: "false"}, "")
	if cfg.skipEmptyInit {
		t.Fatalf("python_skip_empty_init false: cfg.skipEmptyInit = true")
	}
}

func TestApplyDirective_TestPatternCommaListReplaces(t *testing.T) {
	cfg := newPyConfig()
	// Comma-separated value REPLACES the defaults (rules_python semantics).
	applyDirective(cfg, rule.Directive{Key: directiveTestPattern, Value: "*_spec.py, integration/**"}, "")
	want := []string{"*_spec.py", "integration/**"}
	if !reflect.DeepEqual(cfg.testPatterns, want) {
		t.Errorf("comma-list replace: testPatterns = %v, want %v", cfg.testPatterns, want)
	}
	// Bare single value still APPENDS, preserving the prior plugin behavior.
	cfg = newPyConfig()
	before := append([]string(nil), cfg.testPatterns...)
	applyDirective(cfg, rule.Directive{Key: directiveTestPattern, Value: "*_check.py"}, "")
	if len(cfg.testPatterns) != len(before)+1 {
		t.Errorf("bare value should append: got %v from %v", cfg.testPatterns, before)
	}
}

func TestApplyDirective_LabelNormalization(t *testing.T) {
	cases := map[string]labelNormalizationType{
		"snake_case": snakeCaseNormalization,
		"pep503":     pep503Normalization,
		"none":       noneNormalization,
		"PEP503":     pep503Normalization, // case-insensitive
	}
	for val, want := range cases {
		cfg := newPyConfig()
		applyDirective(cfg, rule.Directive{Key: directiveLabelNormalization, Value: val}, "")
		if cfg.labelNormalization != want {
			t.Errorf("python_label_normalization=%q: got %v, want %v", val, cfg.labelNormalization, want)
		}
	}
}

func TestClone_Independent(t *testing.T) {
	parent := newPyConfig()
	parent.libraryName = "lib"
	parent.testPatterns = append(parent.testPatterns, "*_check.py")

	child := parent.clone()
	child.libraryName = "src"
	child.testPatterns = append(child.testPatterns, "**/__tests__/**")

	if parent.libraryName != "lib" {
		t.Errorf("parent libraryName mutated: %q", parent.libraryName)
	}
	if contains(parent.testPatterns, "**/__tests__/**") {
		t.Errorf("parent testPatterns mutated: %v", parent.testPatterns)
	}
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
