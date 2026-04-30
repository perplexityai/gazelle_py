package py

import (
	"reflect"
	"sort"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// TestImports_LibrarySpecs covers the normal py_library case: we index the
// dotted package path, the wildcard, and one per-src spec for each .py.
// __init__.py is intentionally excluded.
func TestImports_LibrarySpecs(t *testing.T) {
	l := &pyLang{}
	c := &config.Config{Exts: map[string]interface{}{languageName: newPyConfig()}}
	f := rule.EmptyFile("pkg/sub/BUILD.bazel", "pkg/sub")

	r := rule.NewRule(defaultLibraryKind, "sub")
	r.SetAttr("srcs", []string{"a.py", "b.py", "__init__.py"})

	got := importPaths(l.Imports(c, r, f))
	want := []string{"pkg.sub", "pkg.sub.*", "pkg.sub.a", "pkg.sub.b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Imports() = %v, want %v", got, want)
	}
}

// TestImports_ConftestNarrowSpec is the load-bearing assertion for the
// dedicated `:conftest` rule: it must register ONLY the conftest module
// (`pkg.conftest`), never the package-wide `pkg` / `pkg.*` specs that the
// real library at the same package already owns. Otherwise the rule index
// would have two providers for `pkg` and resolution would race.
func TestImports_ConftestNarrowSpec(t *testing.T) {
	l := &pyLang{}
	c := &config.Config{Exts: map[string]interface{}{languageName: newPyConfig()}}
	f := rule.EmptyFile("pkg/sub/BUILD.bazel", "pkg/sub")

	r := rule.NewRule(defaultLibraryKind, conftestTargetName)
	r.SetAttr("srcs", []string{conftestFilename})

	got := importPaths(l.Imports(c, r, f))
	want := []string{"pkg.sub.conftest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("conftest Imports() = %v, want %v (must NOT contain pkg.sub or pkg.sub.*)", got, want)
	}
}

// TestImports_TestKindNotIndexed: test rules don't get indexed. Cross-package
// imports of test code are rare, and indexing would create lib↔test cycles
// when both rules sit in the same directory.
func TestImports_TestKindNotIndexed(t *testing.T) {
	l := &pyLang{}
	c := &config.Config{Exts: map[string]interface{}{languageName: newPyConfig()}}
	f := rule.EmptyFile("pkg/BUILD.bazel", "pkg")

	r := rule.NewRule(defaultTestKind, "pkg_test")
	r.SetAttr("srcs", []string{"foo_test.py"})

	if specs := l.Imports(c, r, f); len(specs) != 0 {
		t.Errorf("test rules should not be indexed, got %d specs", len(specs))
	}
}

func importPaths(specs []resolve.ImportSpec) []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.Imp)
	}
	sort.Strings(out)
	return out
}
