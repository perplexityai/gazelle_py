package py

import (
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// Imports is called during the "index" phase for every rule. It returns
// import paths the rule provides — gazelle stores these in a reverse index
// that Resolve() queries when picking deps.
//
// For a library at //packages/foo we register:
//   - the dotted module path of the package directory ("packages.foo")
//   - a wildcard ("packages.foo.*") so callers that import a not-yet-indexed
//     submodule still resolve here
//   - a per-srcs path for each .py file in the rule
//     ("packages.foo.bar" for srcs/bar.py)
//
// Test rules are not indexed: importing into a test target from outside is a
// rare pattern and generating index specs for tests creates library→test
// cycles when a library and its tests sit in the same directory.
func (l *pyLang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
	cfg, _ := c.Exts[languageName].(*pyConfig)
	if cfg == nil {
		cfg = newPyConfig()
	}

	if r.Kind() != cfg.libraryKind {
		return nil
	}
	// `:conftest` library targets exist purely to be picked up by the
	// conftest-synthesis path in resolve.go — indexing them under the
	// package's module path would shadow the real library's specs.
	if r.Name() == "conftest" {
		return nil
	}

	pkg := strings.ReplaceAll(f.Pkg, "/", ".")
	if pkg == "" {
		return nil
	}

	specs := []resolve.ImportSpec{
		{Lang: languageName, Imp: pkg},
		{Lang: languageName, Imp: pkg + ".*"},
	}

	for _, s := range r.AttrStrings("srcs") {
		if s == "" || s == "__init__.py" {
			continue
		}
		mod := strings.TrimSuffix(s, ".py")
		mod = strings.ReplaceAll(mod, "/", ".")
		specs = append(specs, resolve.ImportSpec{
			Lang: languageName,
			Imp:  pkg + "." + mod,
		})
	}

	return specs
}
