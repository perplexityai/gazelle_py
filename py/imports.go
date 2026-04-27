package py

import (
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// Imports returns the import specs that a rule provides; gazelle stores these
// in a reverse index that maps import paths to Bazel labels.
//
// For a library at //packages/foo, we register the dotted module path
// derived from the package directory (e.g. "packages.foo"). This lets
// Resolve() answer queries like `import packages.foo` → //packages/foo.
//
// Test rules don't export reusable modules, so they don't appear in the index.
func (l *pyLang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
	cfg, _ := c.Exts[languageName].(*pyConfig)
	if cfg == nil {
		cfg = newPyConfig()
	}

	if r.Kind() != cfg.libraryKind {
		return nil
	}

	pkg := strings.ReplaceAll(f.Pkg, "/", ".")
	if pkg == "" {
		return nil
	}
	return []resolve.ImportSpec{
		{Lang: languageName, Imp: pkg},
		{Lang: languageName, Imp: pkg + ".*"},
	}
}
