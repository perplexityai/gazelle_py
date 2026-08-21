package py

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/rule"
	bzl "github.com/bazelbuild/buildtools/build"
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

func (e *sourcePatternExpander) contains(source string) bool {
	source = filepath.ToSlash(source)
	i := sort.SearchStrings(e.sources, source)
	return i < len(e.sources) && e.sources[i] == source
}

type packageSourceOwnership struct {
	cfg         *pyConfig
	c           *config.Config
	file        *rule.File
	managed     map[string]bool
	expander    *sourcePatternExpander
	binaryKinds map[string]bool
	libKinds    map[string]bool
	testKinds   map[string]bool

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
		binaryKinds:    mappedKinds(c, defaultBinaryKind),
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
			if !isResourceOwnerRule(r) && !o.isPythonSourceOwner(r) {
				continue
			}
			srcs = o.knownLiteralPythonSources(r)
		}
		for _, src := range srcs {
			source := normalizeLocalSource(src)
			if source != "" && o.expander.contains(source) {
				owned[source] = true
			}
		}
	}
	return owned
}

func (o *packageSourceOwnership) sourcesForRule(r *rule.Rule) ([]string, bool) {
	if srcs, ok := o.sourcesByRule[r]; ok {
		return append([]string(nil), srcs...), true
	}

	main, mainIsLiteral := literalStringAttr(r, "main")
	if !mainIsLiteral {
		return nil, false
	}

	var srcs []string
	switch {
	case r.Attr("srcs") != nil:
		explicit, ok := literalStringListAttr(r, "srcs")
		if !ok || len(explicit) == 0 {
			return nil, false
		}
		srcs = o.normalizeKnownLocalPythonSources(filterPythonSources(explicit, o.cfg))
	case r.Attr("main") != nil:
		srcs = o.normalizeKnownLocalPythonSources(filterPythonSources([]string{main}, o.cfg))
	case r.Attr("file_patterns") != nil:
		patterns, ok := literalStringListAttr(r, "file_patterns")
		if !ok || len(patterns) == 0 {
			return nil, false
		}
		ignorePatterns, ok := literalStringListAttr(r, "ignore_patterns")
		if !ok {
			return nil, false
		}
		srcs = o.expander.expand(patterns, ignorePatterns)
		srcs = o.excludeExplicitSiblingSources(r, srcs)
	default:
		return nil, false
	}
	if o.isPythonBinaryRule(r) && r.Attr("main") != nil {
		main = normalizeLocalSource(main)
		if isPythonFile(main, o.cfg) && !containsSource(srcs, main) {
			srcs = append(srcs, main)
			sort.Strings(srcs)
		}
	}
	if o.isPythonBinaryRule(r) && len(srcs) == 0 {
		return nil, false
	}

	o.sourcesByRule[r] = append([]string(nil), srcs...)
	return append([]string(nil), srcs...), true
}

func containsSource(sources []string, candidate string) bool {
	candidate = normalizeLocalSource(candidate)
	if candidate == "" {
		return false
	}
	for _, source := range sources {
		if normalizeLocalSource(source) == candidate {
			return true
		}
	}
	return false
}

func (o *packageSourceOwnership) isPythonRule(r *rule.Rule) (bool, bool) {
	isLib := o.libKinds[r.Kind()]
	isTest := o.testKinds[r.Kind()]
	return isLib || isTest, isTest
}

func (o *packageSourceOwnership) isPythonBinaryRule(r *rule.Rule) bool {
	return o.binaryKinds[r.Kind()]
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
	if o.isConfiguredPythonMainOwner(r) {
		return true
	}
	return strings.Contains(r.Kind(), "test") && (r.Attr("srcs") != nil || r.Attr("file_patterns") != nil)
}

func (o *packageSourceOwnership) isConfiguredPythonMainOwner(r *rule.Rule) bool {
	main, ok := literalStringAttr(r, "main")
	main = normalizeLocalSource(main)
	return !o.isPythonBinaryRule(r) &&
		o.cfg.mainOwnerKinds[r.Kind()] &&
		r.Attr("main") != nil &&
		ok &&
		o.expander.contains(main) &&
		!o.mainOwnedByLocalLibraryDependency(r)
}

func (o *packageSourceOwnership) mainOwnedByLocalLibraryDependency(r *rule.Rule) bool {
	if o.file == nil {
		return false
	}
	main, ok := literalStringAttr(r, "main")
	if !ok {
		return false
	}
	// Direct labels are safe ownership evidence even when the surrounding list
	// also contains a computed element. Do not use the partial list for rewrites.
	deps, _ := literalStringListAttr(r, "deps")
	main = filepath.ToSlash(main)
	for _, dep := range deps {
		name, ok := localDependencyName(dep, o.file.Pkg)
		if !ok {
			continue
		}
		for _, candidate := range o.file.Rules {
			isPython, isTest := o.isPythonRule(candidate)
			if candidate.Name() != name || !isPython || isTest {
				continue
			}
			srcs, ok := o.sourcesForRule(candidate)
			if ok && containsSource(srcs, main) {
				return true
			}
		}
	}
	return false
}

func localDependencyName(dep string, pkg string) (string, bool) {
	if strings.HasPrefix(dep, ":") {
		return strings.TrimPrefix(dep, ":"), true
	}
	packageLabel := "//" + pkg
	if pkg != "" && dep == packageLabel {
		return path.Base(pkg), true
	}
	prefix := packageLabel + ":"
	if strings.HasPrefix(dep, prefix) {
		return strings.TrimPrefix(dep, prefix), true
	}
	return "", false
}

func (o *packageSourceOwnership) configuredPythonMainSources() map[string]bool {
	owned := map[string]bool{}
	if o.file == nil {
		return owned
	}
	for _, r := range o.file.Rules {
		if !o.isConfiguredPythonMainOwner(r) {
			continue
		}
		main, _ := literalStringAttr(r, "main")
		owned[normalizeLocalSource(main)] = true
	}
	return owned
}

func (o *packageSourceOwnership) sourcesOwnedByRule(r *rule.Rule) ([]string, bool) {
	if isResourceOwnerRule(r) {
		return o.resourcePythonSourcesOwnedByRule(r)
	}
	if !o.isPythonSourceOwner(r) {
		return nil, false
	}
	if isPythonTestPackageRule(r) && r.Attr("srcs") == nil && r.Attr("file_patterns") == nil && r.Attr("main") == nil {
		return o.expander.all(), true
	}
	isMainOwner := o.isConfiguredPythonMainOwner(r)
	srcs, ok := o.sourcesForRule(r)
	if !isMainOwner {
		return srcs, ok
	}

	main, _ := literalStringAttr(r, "main")
	main = filepath.ToSlash(main)
	if !ok {
		srcs = o.knownLiteralPythonSources(r)
		if !containsSource(srcs, main) {
			srcs = append(srcs, normalizeLocalSource(main))
			sort.Strings(srcs)
		}
		return srcs, true
	}
	for _, src := range srcs {
		if filepath.ToSlash(src) == main {
			return srcs, true
		}
	}
	return append(srcs, main), true
}

func isPythonTestPackageRule(r *rule.Rule) bool {
	return strings.Contains(r.Kind(), "test_package")
}

func isResourceOwnerRule(r *rule.Rule) bool {
	return r.Kind() == "filegroup"
}

func (o *packageSourceOwnership) resourcePythonSourcesOwnedByRule(r *rule.Rule) ([]string, bool) {
	candidates, _ := literalStringListAttr(r, "srcs")
	if len(candidates) == 0 {
		return nil, false
	}
	seen := map[string]bool{}
	var sources []string
	for _, candidate := range candidates {
		source := normalizeLocalSource(candidate)
		if source == "" || !o.expander.contains(source) || seen[source] {
			continue
		}
		seen[source] = true
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources, true
}

func (o *packageSourceOwnership) knownLiteralPythonSources(r *rule.Rule) []string {
	candidates, _ := literalStringListAttr(r, "srcs")
	seen := map[string]bool{}
	var sources []string
	for _, candidate := range candidates {
		source := normalizeLocalSource(candidate)
		if source == "" || !o.expander.contains(source) || seen[source] {
			continue
		}
		seen[source] = true
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

func (o *packageSourceOwnership) normalizeKnownLocalPythonSources(sources []string) []string {
	normalized := make([]string, len(sources))
	for i, source := range sources {
		local := normalizeLocalSource(source)
		if local != "" && o.expander.contains(local) {
			normalized[i] = local
			continue
		}
		normalized[i] = filepath.ToSlash(source)
	}
	sort.Strings(normalized)
	return normalized
}

func (o *packageSourceOwnership) preservesSourceAttrs(name string, isTest bool) bool {
	r := o.existingPythonRule(name, isTest)
	if r == nil {
		return false
	}
	patterns, ok := literalStringListAttr(r, "file_patterns")
	return ok && len(patterns) > 0
}

func (o *packageSourceOwnership) existingExplicitRuleSources(name string, isTest bool) ([]string, bool) {
	r := o.existingPythonRule(name, isTest)
	if r == nil || r.Attr("srcs") == nil {
		return nil, false
	}
	srcs, ok := literalStringListAttr(r, "srcs")
	if !ok {
		return nil, false
	}
	return filterPythonSources(srcs, o.cfg), true
}

func (o *packageSourceOwnership) existingRuleDeps(name string, isTest bool) ([]string, bool) {
	r := o.existingPythonRule(name, isTest)
	if r == nil || r.Attr("deps") == nil {
		return nil, false
	}
	deps, ok := literalStringListAttr(r, "deps")
	if !ok {
		return nil, false
	}
	return deps, true
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

func (o *packageSourceOwnership) leavesExistingRuleUnmanaged(name string, isTest bool) bool {
	r := o.existingPythonRule(name, isTest)
	if r == nil {
		return false
	}
	for _, attr := range []string{"srcs", "file_patterns", "ignore_patterns", "deps"} {
		if _, complete := literalStringListAttr(r, attr); !complete {
			return true
		}
	}
	_, mainIsLiteral := literalStringAttr(r, "main")
	return !mainIsLiteral
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
		if isResourceOwnerRule(sibling) {
			srcs, ok := o.resourcePythonSourcesOwnedByRule(sibling)
			if !ok {
				continue
			}
			for _, src := range srcs {
				explicit[filepath.ToSlash(src)] = true
			}
			continue
		}
		if ok, _ := o.isPythonRule(sibling); !ok {
			continue
		}
		if sibling.Attr("srcs") == nil {
			continue
		}
		for _, source := range o.knownLiteralPythonSources(sibling) {
			explicit[source] = true
		}
	}
	o.explicitByRule[r] = explicit
	return explicit
}

// literalStringListAttr returns direct string elements and reports whether
// the attribute is absent or consists entirely of strings. It never evaluates
// nested expressions such as glob, select, concatenation, or identifiers.
func literalStringListAttr(r *rule.Rule, name string) ([]string, bool) {
	expr := r.Attr(name)
	if expr == nil {
		return nil, true
	}
	list, ok := expr.(*bzl.ListExpr)
	if !ok {
		return nil, false
	}
	values := make([]string, len(list.List))
	complete := true
	n := 0
	for _, item := range list.List {
		value, ok := item.(*bzl.StringExpr)
		if !ok {
			complete = false
			continue
		}
		values[n] = value.Value
		n++
	}
	return values[:n], complete
}

// literalStringAttr reports absent attributes as complete and rejects every
// non-string expression without evaluating it.
func literalStringAttr(r *rule.Rule, name string) (string, bool) {
	expr := r.Attr(name)
	if expr == nil {
		return "", true
	}
	value, ok := expr.(*bzl.StringExpr)
	if !ok {
		return "", false
	}
	return value.Value, true
}

func normalizeLocalSource(source string) string {
	source = filepath.ToSlash(source)
	if source == "" || strings.HasPrefix(source, "//") || strings.HasPrefix(source, "@") {
		return ""
	}
	source = strings.TrimPrefix(source, ":")
	source = strings.TrimPrefix(source, "./")
	if source == "" || strings.HasPrefix(source, "/") {
		return ""
	}
	for _, segment := range strings.Split(source, "/") {
		if segment == ".." {
			return ""
		}
	}
	return source
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
