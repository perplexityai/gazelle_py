package py

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/bmatcuk/doublestar/v4"
)

type sourceFilesCacheKey struct {
	repoRoot   string
	pkg        string
	extensions string
}

type sourcePatternExpander struct {
	cfg     *pyConfig
	sources []string
	cache   map[string][]string
}

func newSourcePatternExpander(cfg *pyConfig, sources []string) *sourcePatternExpander {
	srcs := append([]string(nil), sources...)
	sort.Strings(srcs)
	return &sourcePatternExpander{
		cfg:     cfg,
		sources: srcs,
		cache:   map[string][]string{},
	}
}

func (e *sourcePatternExpander) expand(patterns []string, ignorePatterns []string) []string {
	key := strings.Join(patterns, "\x00") + "\x01" + strings.Join(ignorePatterns, "\x00")
	if srcs, ok := e.cache[key]; ok {
		return append([]string(nil), srcs...)
	}

	seen := map[string]bool{}
	var srcs []string
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(pattern)
		for _, src := range e.sources {
			ok, err := doublestar.Match(pattern, src)
			if err != nil || !ok || matchesAnyPattern(ignorePatterns, src) {
				continue
			}
			if seen[src] {
				continue
			}
			seen[src] = true
			srcs = append(srcs, src)
		}
	}
	sort.Strings(srcs)
	e.cache[key] = append([]string(nil), srcs...)
	return srcs
}

func (e *sourcePatternExpander) all() []string {
	return append([]string(nil), e.sources...)
}

type packageSourceOwnership struct {
	cfg       *pyConfig
	c         *config.Config
	file      *rule.File
	managed   map[string]bool
	expander  *sourcePatternExpander
	libKinds  map[string]bool
	testKinds map[string]bool

	sourcesByRule  map[*rule.Rule][]string
	explicitByRule map[*rule.Rule]map[string]bool
}

func newSpecPackageSourceOwnership(cfg *pyConfig, c *config.Config, rel string, specs []FileSpec, file *rule.File, managed map[string]bool) *packageSourceOwnership {
	sources := make([]string, 0, len(specs))
	for _, s := range specs {
		src := filepath.ToSlash(pkgRelativePath(s.RelPath, rel))
		if isPythonFile(src, cfg) {
			sources = append(sources, src)
		}
	}
	return newPackageSourceOwnership(cfg, c, file, managed, sources)
}

func newDiskPackageSourceOwnership(l *pyLang, cfg *pyConfig, c *config.Config, file *rule.File) *packageSourceOwnership {
	var sources []string
	if c != nil && file != nil {
		sources = l.cachedPackagePythonSources(cfg, c.RepoRoot, file.Pkg)
	}
	return newPackageSourceOwnership(cfg, c, file, nil, sources)
}

func newPackageSourceOwnership(cfg *pyConfig, c *config.Config, file *rule.File, managed map[string]bool, sources []string) *packageSourceOwnership {
	return &packageSourceOwnership{
		cfg:            cfg,
		c:              c,
		file:           file,
		managed:        managed,
		expander:       newSourcePatternExpander(cfg, sources),
		libKinds:       mappedKinds(c, cfg.libraryKind),
		testKinds:      mappedKinds(c, cfg.testKind),
		sourcesByRule:  map[*rule.Rule][]string{},
		explicitByRule: map[*rule.Rule]map[string]bool{},
	}
}

func (o *packageSourceOwnership) handOwnedSources() map[string]bool {
	owned := map[string]bool{}
	if o.file == nil {
		return owned
	}
	for _, r := range o.file.Rules {
		if o.isManagedExistingRule(r) {
			continue
		}
		srcs, ok := o.sourcesOwnedByRule(r)
		if !ok {
			continue
		}
		for _, src := range srcs {
			owned[filepath.ToSlash(src)] = true
		}
	}
	return owned
}

func (o *packageSourceOwnership) sourcesForRule(r *rule.Rule) ([]string, bool) {
	if srcs, ok := o.sourcesByRule[r]; ok {
		return append([]string(nil), srcs...), true
	}

	var srcs []string
	switch {
	case r.Attr("srcs") != nil:
		srcs = filterPythonSources(r.AttrStrings("srcs"), o.cfg)
	case r.Attr("main") != nil:
		srcs = filterPythonSources([]string{r.AttrString("main")}, o.cfg)
	case len(r.AttrStrings("file_patterns")) > 0:
		srcs = o.expander.expand(r.AttrStrings("file_patterns"), r.AttrStrings("ignore_patterns"))
		srcs = o.excludeExplicitSiblingSources(r, srcs)
	default:
		return nil, false
	}

	o.sourcesByRule[r] = append([]string(nil), srcs...)
	return append([]string(nil), srcs...), true
}

func (o *packageSourceOwnership) isPythonRule(r *rule.Rule) (bool, bool) {
	isLib := o.libKinds[r.Kind()]
	isTest := o.testKinds[r.Kind()]
	return isLib || isTest, isTest
}

func (o *packageSourceOwnership) isManagedExistingRule(r *rule.Rule) bool {
	if o.managed == nil || !o.managed[r.Name()] {
		return false
	}
	ok, _ := o.isPythonRule(r)
	return ok
}

func (o *packageSourceOwnership) isPythonSourceOwner(r *rule.Rule) bool {
	if ok, _ := o.isPythonRule(r); ok {
		return true
	}
	if isPythonTestPackageRule(r) {
		return true
	}
	return strings.Contains(r.Kind(), "test") && (r.Attr("srcs") != nil || len(r.AttrStrings("file_patterns")) > 0)
}

func (o *packageSourceOwnership) sourcesOwnedByRule(r *rule.Rule) ([]string, bool) {
	if isResourceOwnerRule(r) {
		return o.resourcePythonSourcesOwnedByRule(r)
	}
	if !o.isPythonSourceOwner(r) {
		return nil, false
	}
	if isPythonTestPackageRule(r) && r.Attr("srcs") == nil && len(r.AttrStrings("file_patterns")) == 0 && r.Attr("main") == nil {
		return o.expander.all(), true
	}
	return o.sourcesForRule(r)
}

func isPythonTestPackageRule(r *rule.Rule) bool {
	return strings.Contains(r.Kind(), "test_package")
}

func isResourceOwnerRule(r *rule.Rule) bool {
	return r.Kind() == "filegroup"
}

func (o *packageSourceOwnership) resourcePythonSourcesOwnedByRule(r *rule.Rule) ([]string, bool) {
	patterns := r.AttrStrings("srcs")
	var excludes []string
	if len(patterns) == 0 {
		glob, ok := rule.ParseGlobExpr(r.Attr("srcs"))
		if !ok {
			return nil, false
		}
		patterns = glob.Patterns
		excludes = glob.Excludes
	}
	return o.expander.expand(patterns, excludes), true
}

func (o *packageSourceOwnership) preservesSourceAttrs(name string, isTest bool) bool {
	r := o.existingPythonRule(name, isTest)
	return r != nil && r.Attr("srcs") == nil && len(r.AttrStrings("file_patterns")) > 0
}

func (o *packageSourceOwnership) existingExplicitRuleSources(name string, isTest bool) ([]string, bool) {
	r := o.existingPythonRule(name, isTest)
	if r == nil || r.Attr("srcs") == nil {
		return nil, false
	}
	return filterPythonSources(r.AttrStrings("srcs"), o.cfg), true
}

func (o *packageSourceOwnership) existingRuleDeps(name string, isTest bool) ([]string, bool) {
	r := o.existingPythonRule(name, isTest)
	if r == nil || r.Attr("deps") == nil {
		return nil, false
	}
	return r.AttrStrings("deps"), true
}

func (o *packageSourceOwnership) existingRuleSources(name string, isTest bool) ([]string, bool) {
	r := o.existingPythonRule(name, isTest)
	if r == nil {
		return nil, false
	}
	return o.sourcesForRule(r)
}

func (o *packageSourceOwnership) existingPythonRule(name string, isTest bool) *rule.Rule {
	if o.file == nil {
		return nil
	}
	kinds := o.libKinds
	if isTest {
		kinds = o.testKinds
	}
	for _, r := range o.file.Rules {
		if r.Name() != name || !kinds[r.Kind()] {
			continue
		}
		return r
	}
	return nil
}

func (o *packageSourceOwnership) excludeExplicitSiblingSources(r *rule.Rule, srcs []string) []string {
	if o.file == nil || len(srcs) == 0 {
		return srcs
	}

	explicit := o.explicitSiblingSources(r)
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

func (o *packageSourceOwnership) explicitSiblingSources(r *rule.Rule) map[string]bool {
	if explicit, ok := o.explicitByRule[r]; ok {
		return explicit
	}

	explicit := map[string]bool{}
	for _, sibling := range o.file.Rules {
		if sibling.Name() == r.Name() && sibling.Kind() == r.Kind() {
			continue
		}
		if ok, _ := o.isPythonRule(sibling); !ok {
			continue
		}
		if sibling.Attr("srcs") == nil {
			continue
		}
		for _, src := range filterPythonSources(sibling.AttrStrings("srcs"), o.cfg) {
			explicit[filepath.ToSlash(src)] = true
		}
	}
	o.explicitByRule[r] = explicit
	return explicit
}

func (l *pyLang) cachedPackagePythonSources(cfg *pyConfig, repoRoot string, pkg string) []string {
	key := sourceFilesCacheKey{
		repoRoot:   repoRoot,
		pkg:        pkg,
		extensions: strings.Join(cfg.extensions, "\x00"),
	}

	l.sourceFilesMu.Lock()
	if l.sourceFilesCache == nil {
		l.sourceFilesCache = map[sourceFilesCacheKey][]string{}
	}
	if srcs, ok := l.sourceFilesCache[key]; ok {
		l.sourceFilesMu.Unlock()
		return append([]string(nil), srcs...)
	}
	l.sourceFilesMu.Unlock()

	srcs := listPackagePythonSources(cfg, filepath.Join(repoRoot, pkg))

	l.sourceFilesMu.Lock()
	l.sourceFilesCache[key] = append([]string(nil), srcs...)
	l.sourceFilesMu.Unlock()

	return srcs
}

func listPackagePythonSources(cfg *pyConfig, pkgDir string) []string {
	var sources []string
	err := filepath.WalkDir(pkgDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(pkgDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if isPythonFile(rel, cfg) {
			sources = append(sources, rel)
		}
		return nil
	})
	if err != nil {
		return nil
	}
	sort.Strings(sources)
	return sources
}
