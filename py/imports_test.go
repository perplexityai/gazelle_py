package py

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// TestImports_LibrarySpecs covers the normal py_library case: we index the
// package module from __init__.py and one concrete spec for each .py file.
// We intentionally do not register a package wildcard since that lets a broad
// target claim files owned by narrower targets.
func TestImports_LibrarySpecs(t *testing.T) {
	l := &pyLang{}
	c := &config.Config{Exts: map[string]interface{}{languageName: newPyConfig()}}
	f := rule.EmptyFile("pkg/sub/BUILD.bazel", "pkg/sub")

	r := rule.NewRule(defaultLibraryKind, "sub")
	r.SetAttr("srcs", []string{"a.py", "b.py", "__init__.py"})

	got := importPaths(l.Imports(c, r, f))
	want := []string{"pkg.sub", "pkg.sub.a", "pkg.sub.b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Imports() = %v, want %v", got, want)
	}
}

func TestImports_MappedLibraryKind(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "pplx/python/apps/asi/tests")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "helpers.py"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "__init__.py"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	l := &pyLang{}
	c := &config.Config{
		RepoRoot: root,
		Exts:     map[string]interface{}{languageName: newPyConfig()},
		KindMap: map[string]config.MappedKind{
			defaultLibraryKind: {KindName: "pplx_python_library"},
		},
	}
	f := rule.EmptyFile("pplx/python/apps/asi/tests/BUILD.bazel", "pplx/python/apps/asi/tests")

	r := rule.NewRule("pplx_python_library", "helpers")
	r.SetAttr("file_patterns", []string{"helpers.py"})

	got := importPaths(l.Imports(c, r, f))
	want := []string{
		"pplx.python.apps.asi.tests.helpers",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mapped library Imports() = %v, want %v", got, want)
	}
}

func TestImports_FilePatternCatchAllRegistersConcreteModules(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "pplx/evals/cli")
	for _, dir := range []string{pkgDir, filepath.Join(pkgDir, "asi"), filepath.Join(pkgDir, "tests")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"__init__.py", "asi/__init__.py", "asi/auth.py", "tests/test_auth.py"} {
		if err := os.WriteFile(filepath.Join(pkgDir, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	l := &pyLang{}
	c := &config.Config{RepoRoot: root, Exts: map[string]interface{}{languageName: newPyConfig()}}
	f := rule.EmptyFile("pplx/evals/cli/BUILD.bazel", "pplx/evals/cli")

	r := rule.NewRule(defaultLibraryKind, "cli_lib")
	r.SetAttr("file_patterns", []string{"**/*.py"})
	r.SetAttr("ignore_patterns", []string{"tests/**/*.py"})

	got := importPaths(l.Imports(c, r, f))
	want := []string{
		"pplx.evals.cli",
		"pplx.evals.cli.asi",
		"pplx.evals.cli.asi.auth",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("file-pattern library Imports() = %v, want %v", got, want)
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
