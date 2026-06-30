package py

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// Imports is called during the "index" phase for every rule. It returns
// import paths the rule provides — gazelle stores these in a reverse index
// that Resolve() queries when picking deps.
//
// For a library at //packages/foo we register only concrete modules the rule
// actually owns, for example:
//   - "packages.foo" for srcs/__init__.py
//   - "packages.foo.bar" for srcs/bar.py
//   - "packages.foo.sub" for srcs/sub/__init__.py
//
// Test rules are not indexed: importing into a test target from outside is a
// rare pattern and generating index specs for tests creates library→test
// cycles when a library and its tests sit in the same directory.
func (l *pyLang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
	cfg, _ := c.Exts[languageName].(*pyConfig)
	if cfg == nil {
		cfg = newPyConfig()
	}

	if !mappedKinds(c, cfg.libraryKind)[r.Kind()] {
		return nil
	}

	// Strip the python_root prefix so dotted import paths are interpreted
	// relative to it. With pythonRoot="backend", `backend/api/` indexes as
	// `api` (and `api.*`) so source code's `from api.x import …` resolves.
	rel := f.Pkg
	if cfg.pythonRoot != "" {
		rel = strings.TrimPrefix(rel, cfg.pythonRoot)
		rel = strings.TrimPrefix(rel, "/")
	}
	pkg := strings.ReplaceAll(rel, "/", ".")
	if pkg == "" {
		return nil
	}

	srcs, ok := rulePythonSourceFilesFromDisk(cfg, r, c.RepoRoot, f.Pkg)
	if !ok {
		return nil
	}
	if r.Attr("srcs") == nil {
		srcs = excludeExplicitSiblingSources(cfg, c, r, f, srcs)
	}

	seen := map[string]bool{}
	var specs []resolve.ImportSpec
	for _, src := range srcs {
		imp, ok := importPathForSource(pkg, src)
		if !ok || seen[imp] {
			continue
		}
		seen[imp] = true
		specs = append(specs, resolve.ImportSpec{Lang: languageName, Imp: imp})
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Imp < specs[j].Imp
	})
	return specs
}

func excludeExplicitSiblingSources(cfg *pyConfig, c *config.Config, r *rule.Rule, f *rule.File, srcs []string) []string {
	if f == nil || len(srcs) == 0 {
		return srcs
	}

	explicit := explicitSiblingPythonSources(cfg, c, r, f)
	if len(explicit) == 0 {
		return srcs
	}

	out := make([]string, 0, len(srcs))
	for _, src := range srcs {
		if explicit[filepath.ToSlash(src)] {
			continue
		}
		out = append(out, src)
	}
	return out
}

func explicitSiblingPythonSources(cfg *pyConfig, c *config.Config, r *rule.Rule, f *rule.File) map[string]bool {
	libKinds := mappedKinds(c, cfg.libraryKind)
	testKinds := mappedKinds(c, cfg.testKind)
	explicit := map[string]bool{}

	for _, sibling := range f.Rules {
		if sibling.Name() == r.Name() && sibling.Kind() == r.Kind() {
			continue
		}
		if !libKinds[sibling.Kind()] && !testKinds[sibling.Kind()] {
			continue
		}
		if sibling.Attr("srcs") == nil {
			continue
		}
		for _, src := range filterPythonSources(sibling.AttrStrings("srcs"), cfg) {
			explicit[filepath.ToSlash(src)] = true
		}
	}
	return explicit
}
