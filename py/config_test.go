package py

import (
	"reflect"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/rule"
)

func TestApplyDirective_Bools(t *testing.T) {
	cfg := newPyConfig()
	applyDirective(cfg, rule.Directive{Key: directiveEnabled, Value: "false"})
	if cfg.enabled {
		t.Fatalf("py_enabled false: cfg.enabled = true")
	}
	applyDirective(cfg, rule.Directive{Key: directiveEnabled, Value: "true"})
	if !cfg.enabled {
		t.Fatalf("py_enabled true: cfg.enabled = false")
	}
}

func TestApplyDirective_Strings(t *testing.T) {
	cfg := newPyConfig()
	applyDirective(cfg, rule.Directive{Key: directiveLibraryName, Value: "src"})
	applyDirective(cfg, rule.Directive{Key: directiveTestName, Value: "spec"})
	applyDirective(cfg, rule.Directive{Key: directiveLibraryKind, Value: "my_lib"})
	applyDirective(cfg, rule.Directive{Key: directiveTestKind, Value: "my_test"})
	applyDirective(cfg, rule.Directive{Key: directivePipLinkPattern, Value: "@my_pip//{pkg}"})

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
	applyDirective(cfg, rule.Directive{Key: directiveVisibility, Value: "//foo:__pkg__ //bar:__pkg__"})
	want := []string{"//foo:__pkg__", "//bar:__pkg__"}
	if !reflect.DeepEqual(cfg.visibility, want) {
		t.Errorf("visibility = %v want %v", cfg.visibility, want)
	}
}

func TestApplyDirective_AppendDirectives(t *testing.T) {
	cfg := newPyConfig()
	applyDirective(cfg, rule.Directive{Key: directiveTestPattern, Value: "*_check.py"})
	applyDirective(cfg, rule.Directive{Key: directiveExtension, Value: ".pyi"})
	applyDirective(cfg, rule.Directive{Key: directiveTestData, Value: "//:fixtures"})

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
	applyDirective(cfg, rule.Directive{Key: directiveTestPattern, Value: "*_check.py"})
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
