package py

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/repo"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// pythonStdlib is exposed as a package-level alias for stdlibModules so the
// existing resolve_test.go's TestPythonStdlibCovered keeps working.
var pythonStdlib = stdlibModules

// Resolve converts ImportData (attached during GenerateRules) into Bazel
// labels and writes them onto the rule's `deps` attr.
//
// Resolution uses a "possible modules" loop: for each import, try
// progressively shorter dotted prefixes ("a.b.c.d" → "a.b.c" → "a.b" → "a"),
// and at each level check ALL resolution sources (gazelle:resolve directive,
// pip manifest, first-party rule index, stdlib) before stepping to a shorter
// prefix. A single loop avoids the bug where a broad directive at "a.b" would
// steal an import meant for "a.b.c".
func (l *pyLang) Resolve(
	c *config.Config,
	ix *resolve.RuleIndex,
	rc *repo.RemoteCache,
	r *rule.Rule,
	rawImportData interface{},
	from label.Label,
) {
	importData, ok := rawImportData.(ImportData)
	if !ok {
		return
	}
	if importData.PreserveDeps {
		return
	}
	cfg := importData.config
	if cfg == nil {
		cfg, _ = c.Exts[languageName].(*pyConfig)
	}
	if cfg == nil {
		cfg = newPyConfig()
	}

	switch r.Kind() {
	case cfg.libraryKind:
		all := l.resolveImports(c, ix, importData, importData.Imports, from, cfg, existingDepsForResolve(importData, r))
		all = append(all, importData.IncludeDeps...)
		setOrDelete(r, "deps", all)

	case cfg.testKind:
		// Test deps come only from the test files' own imports — sibling
		// library imports reach the test transitively via the :lib target
		// the test imports by name.
		modules := append([]ImportStatement{}, importData.TestImports...)

		// Synthesize ancestor conftest imports. Walk from the test's own
		// directory up to the repo root, and for each ancestor that has a
		// `conftest.py`, add a synthetic import for its dotted module path.
		// The normal possible-modules loop then resolves it to whatever
		// `:conftest` library target indexes that path; if no such target
		// exists, the import is silently dropped.
		for _, syn := range l.cachedConftestImportsFor(c.RepoRoot, from.Pkg) {
			modules = append(modules, syn)
		}

		all := l.resolveImports(c, ix, importData, modules, from, cfg, existingDepsForResolve(importData, r))
		all = append(all, importData.IncludeDeps...)
		setOrDelete(r, "deps", all)
	}
}

func existingDepsForResolve(importData ImportData, r *rule.Rule) []string {
	if len(importData.ExistingDeps) > 0 {
		return importData.ExistingDeps
	}
	return r.AttrStrings("deps")
}

// conftestImportsFor walks up from `pkg` (workspace-relative) to the repo root
// and returns synthetic imports for every ancestor that has a conftest.py.
// `pytest` discovers these automatically; we mirror that discovery so the
// resolver can attach a `:conftest` dep when the user has split it out.
func conftestImportsFor(repoRoot, pkg string) []ImportStatement {
	var out []ImportStatement
	cur := pkg
	for {
		if cur == "" {
			break
		}
		if _, err := os.Stat(filepath.Join(repoRoot, cur, "conftest.py")); err == nil {
			module := strings.ReplaceAll(cur, "/", ".") + ".conftest"
			out = append(out, ImportStatement{
				ImportPath: module,
				From:       module,
				SourceFile: filepath.Join(cur, "conftest.py"),
			})
		}
		cur = filepath.Dir(cur)
		if cur == "." {
			cur = ""
		}
	}
	return out
}

type conftestCacheKey struct {
	repoRoot string
	pkg      string
}

func (l *pyLang) cachedConftestImportsFor(repoRoot, pkg string) []ImportStatement {
	key := conftestCacheKey{repoRoot: repoRoot, pkg: pkg}

	l.conftestMu.Lock()
	if l.conftestCache == nil {
		l.conftestCache = map[conftestCacheKey][]ImportStatement{}
	}
	if imports, ok := l.conftestCache[key]; ok {
		l.conftestMu.Unlock()
		return append([]ImportStatement(nil), imports...)
	}
	l.conftestMu.Unlock()

	imports := conftestImportsFor(repoRoot, pkg)

	l.conftestMu.Lock()
	l.conftestCache[key] = append([]ImportStatement(nil), imports...)
	l.conftestMu.Unlock()

	return imports
}

// resolveImports walks each import through the possible-modules loop and
// returns a flat, sorted, deduped dep list.
func (l *pyLang) resolveImports(
	c *config.Config,
	ix *resolve.RuleIndex,
	importData ImportData,
	imports []ImportStatement,
	from label.Label,
	cfg *pyConfig,
	existingDeps []string,
) []string {
	ctx := newResolverContext(l, c, ix, from, cfg, existingDeps)

	seen := map[string]bool{}
	out := []string{}

	for _, imp := range dedupeImportStatements(imports) {
		path := imp.ImportPath
		if path == "" || strings.HasPrefix(path, ".") {
			continue
		}
		// `from a.b.conftest import X`: pytest already handles conftest, so
		// adding it as a Bazel dep would create cycles. Drop it.
		if strings.HasSuffix(path, ".conftest") || path == "conftest" {
			// We DO want the synthesized ancestor-conftest imports the test
			// pipeline added — those have a non-empty SourceFile pointing at
			// the actual conftest path. For real `from x.conftest import …`
			// statements, only the ImportPath is set. Differentiate by
			// matching the SourceFile against a conftest.py file.
			if !strings.HasSuffix(imp.SourceFile, "conftest.py") {
				continue
			}
		}
		if isIgnored(path, imp.From, importData.Ignore) {
			continue
		}

		dep := ctx.resolveOne(path, imp.From)
		if dep == "" {
			continue
		}
		ctx.markResolvedDep(dep)
		if seen[dep] {
			continue
		}
		seen[dep] = true
		out = append(out, dep)
	}
	for _, dep := range ctx.preservedExistingPipDeps() {
		if seen[dep] {
			continue
		}
		seen[dep] = true
		out = append(out, dep)
	}

	sort.Strings(out)
	return out
}

type resolverContext struct {
	c               *config.Config
	ix              *resolve.RuleIndex
	from            label.Label
	cfg             *pyConfig
	manifest        *manifestData
	pipRepo         string
	packageDeps     map[string]bool
	existingPipDeps map[string][]string
	usedPipDists    map[string]bool
	memo            map[resolveLookupKey]string
}

type resolveLookupKey struct {
	moduleName string
	fromPart   string
}

type importDedupeKey struct {
	importPath       string
	from             string
	sourceIsConftest bool
}

func newResolverContext(l *pyLang, c *config.Config, ix *resolve.RuleIndex, from label.Label, cfg *pyConfig, deps []string) *resolverContext {
	manifest := loadManifestOnce(filepath.Join(c.RepoRoot, cfg.manifestPath))
	return &resolverContext{
		c:               c,
		ix:              ix,
		from:            from,
		cfg:             cfg,
		manifest:        manifest,
		pipRepo:         effectivePipRepo(cfg, manifest),
		packageDeps:     l.projectDeps(c.RepoRoot, cfg),
		existingPipDeps: existingPipDepsByDist(deps),
		usedPipDists:    map[string]bool{},
		memo:            map[resolveLookupKey]string{},
	}
}

func dedupeImportStatements(imports []ImportStatement) []ImportStatement {
	seen := map[importDedupeKey]bool{}
	out := make([]ImportStatement, 0, len(imports))
	for _, imp := range imports {
		key := importDedupeKey{
			importPath:       imp.ImportPath,
			from:             imp.From,
			sourceIsConftest: strings.HasSuffix(imp.SourceFile, "conftest.py"),
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, imp)
	}
	return out
}

func effectivePipRepo(cfg *pyConfig, manifest *manifestData) string {
	if cfg.pipLinkPatternExplicit {
		return parsePipRepo(cfg.pipLinkPattern)
	}
	return manifest.PipRepoName
}

// resolveOne implements the possible-modules loop for a single import. Returns
// the resolved dep label, or "" if no resolution applies (stdlib, self-import,
// or a missing third-party package not declared in pyproject/manifest).
func (ctx *resolverContext) resolveOne(moduleName string, fromPart string) string {
	key := resolveLookupKey{moduleName: moduleName, fromPart: fromPart}
	if dep, ok := ctx.memo[key]; ok {
		return dep
	}

	dep := ctx.resolveOneUncached(moduleName, fromPart)
	ctx.memo[key] = dep
	return dep
}

func (ctx *resolverContext) resolveOneUncached(moduleName string, fromPart string) string {
	for _, try := range moduleCandidates(moduleName, fromPart) {
		// 1. gazelle:resolve directive — explicit user override.
		spec := resolve.ImportSpec{Lang: languageName, Imp: try}
		if dep, ok := resolve.FindRuleWithOverride(ctx.c, spec, languageName); ok {
			lbl := dep.Rel(ctx.from.Repo, ctx.from.Pkg).String()
			if lbl == ":"+ctx.from.Name {
				return ""
			}
			return lbl
		}

		// 2. Pip manifest (gazelle_python.yaml).
		if dist, ok := ctx.manifest.ModulesMapping[try]; ok {
			distName := normalizeDist(dist, ctx.cfg.labelNormalization)
			if dep := ctx.existingPipDepForDist(distName); dep != "" {
				return dep
			}
			return ctx.pipLabelForDist(distName)
		}

		// 3. First-party rule index — exact match.
		if hits := ctx.ix.FindRulesByImportWithConfig(ctx.c, spec, languageName); len(hits) > 0 {
			if hits[0].IsSelfImport(ctx.from) {
				return ""
			}
			return hits[0].Label.Rel(ctx.from.Repo, ctx.from.Pkg).String()
		}

		// 3b. First-party wildcard (e.g. `pkg.*` for "anything under pkg").
		wild := resolve.ImportSpec{Lang: languageName, Imp: try + ".*"}
		if hits := ctx.ix.FindRulesByImportWithConfig(ctx.c, wild, languageName); len(hits) > 0 {
			if hits[0].IsSelfImport(ctx.from) {
				return ""
			}
			return hits[0].Label.Rel(ctx.from.Repo, ctx.from.Pkg).String()
		}

		// 3c. Sibling-import resolution: when enabled, also try the import
		// path as if it were a sibling of the importer's own package. So
		// `from app import X` from `myapp/app_test.py` matches a local
		// `myapp/app.py` library. Off by default (matches rules_python).
		if ctx.cfg.resolveSiblingImports && ctx.from.Pkg != "" {
			rel := ctx.from.Pkg
			if ctx.cfg.pythonRoot != "" {
				rel = strings.TrimPrefix(rel, ctx.cfg.pythonRoot)
				rel = strings.TrimPrefix(rel, "/")
			}
			fromDotted := strings.ReplaceAll(rel, "/", ".")
			if fromDotted != "" {
				sibKey := fromDotted + "." + try
				sibSpec := resolve.ImportSpec{Lang: languageName, Imp: sibKey}
				if hits := ctx.ix.FindRulesByImportWithConfig(ctx.c, sibSpec, languageName); len(hits) > 0 {
					if hits[0].IsSelfImport(ctx.from) {
						return ""
					}
					return hits[0].Label.Rel(ctx.from.Repo, ctx.from.Pkg).String()
				}
				sibWild := resolve.ImportSpec{Lang: languageName, Imp: sibKey + ".*"}
				if hits := ctx.ix.FindRulesByImportWithConfig(ctx.c, sibWild, languageName); len(hits) > 0 {
					if hits[0].IsSelfImport(ctx.from) {
						return ""
					}
					return hits[0].Label.Rel(ctx.from.Repo, ctx.from.Pkg).String()
				}
			}
		}

		// 4. Stdlib — no dep needed.
		topLevel := strings.SplitN(try, ".", 2)[0]
		if isStdlib(topLevel) {
			return ""
		}
	}

	tried := map[string]bool{}
	for _, try := range moduleCandidates(moduleName, fromPart) {
		tried[try] = true
	}
	// The from-import bound above prevents broad first-party aggregate matches.
	// Pip manifests are different: wheels commonly expose submodules while the
	// manifest records only the import root, e.g. rich.console -> rich.
	for _, try := range pipManifestCandidates(moduleName) {
		if tried[try] {
			continue
		}
		if dist, ok := ctx.manifest.ModulesMapping[try]; ok {
			distName := normalizeDist(dist, ctx.cfg.labelNormalization)
			if dep := ctx.existingPipDepForDist(distName); dep != "" {
				return dep
			}
			return ctx.pipLabelForDist(distName)
		}
	}

	// Nothing matched at any prefix. Optimistically emit a pip label for the
	// top-level module name unless the user gave us a project-deps file that
	// excludes it.
	topLevel := strings.SplitN(moduleName, ".", 2)[0]
	if isStdlib(topLevel) {
		return ""
	}
	dist := normalizeDist(topLevel, ctx.cfg.labelNormalization)
	// packageDeps is normalized at load time as snake_case for back-compat;
	// gate emission against the snake form regardless of the active mode so
	// pyproject-declared deps stay matched even when rendering pep503 labels.
	declared := normalizeDist(topLevel, snakeCaseNormalization)
	if dep := ctx.existingPipDepForDist(dist); dep != "" {
		return dep
	}
	if len(ctx.packageDeps) > 0 && !ctx.packageDeps[declared] {
		return ""
	}
	return ctx.pipLabelForDist(dist)
}

func (ctx *resolverContext) pipLabelForDist(distName string) string {
	repoName := ctx.pipRepo
	if repoName == "" {
		repoName = parsePipRepo(ctx.cfg.pipLinkPattern)
	}
	return pipLabelForRepo(ctx.cfg.pipLinkPattern, repoName, distName)
}

func (ctx *resolverContext) existingPipDepForDist(distName string) string {
	if len(ctx.existingPipDeps) == 0 {
		return ""
	}
	dist := normalizeDist(distName, snakeCaseNormalization)
	deps := ctx.existingPipDeps[dist]
	if len(deps) == 0 {
		return ""
	}
	ctx.usedPipDists[dist] = true
	return deps[0]
}

func existingPipDepsByDist(deps []string) map[string][]string {
	out := map[string][]string{}
	for _, dep := range deps {
		dist, ok := pipDistFromLabel(dep)
		if !ok {
			continue
		}
		out[dist] = append(out[dist], dep)
	}
	return out
}

func (ctx *resolverContext) markResolvedDep(dep string) {
	dist, ok := pipDistFromLabel(dep)
	if !ok {
		return
	}
	ctx.usedPipDists[dist] = true
}

func pipDistFromLabel(dep string) (string, bool) {
	if !strings.HasPrefix(dep, "@") {
		return "", false
	}
	i := strings.Index(dep, "//")
	if i < 0 {
		return "", false
	}
	pkg := dep[i+2:]
	if j := strings.Index(pkg, ":"); j >= 0 {
		pkg = pkg[:j]
	}
	if pkg == "" || strings.Contains(pkg, "/") {
		return "", false
	}
	return normalizeDist(pkg, snakeCaseNormalization), true
}

func (ctx *resolverContext) preservedExistingPipDeps() []string {
	if len(ctx.existingPipDeps) == 0 {
		return nil
	}
	var out []string
	for dist, deps := range ctx.existingPipDeps {
		if !ctx.usedPipDists[dist] {
			continue
		}
		if len(deps) > 1 {
			out = append(out, deps...)
		}
	}
	return out
}

func moduleCandidates(moduleName string, fromPart string) []string {
	if moduleName == "" {
		return nil
	}

	minParts := 1
	if fromPart != "" && !strings.HasPrefix(fromPart, ".") {
		if moduleName == fromPart || strings.HasPrefix(moduleName, fromPart+".") {
			minParts = len(strings.Split(fromPart, "."))
		}
	}

	parts := strings.Split(moduleName, ".")
	candidates := make([]string, 0, len(parts)-minParts+1)
	for i := len(parts); i >= minParts; i-- {
		candidates = append(candidates, strings.Join(parts[:i], "."))
	}
	return candidates
}

func pipManifestCandidates(moduleName string) []string {
	return moduleCandidates(moduleName, "")
}

func setOrDelete(r *rule.Rule, attr string, values []string) {
	values = deduplicateAndSort(values)
	if len(values) > 0 {
		r.SetAttr(attr, values)
	} else {
		r.DelAttr(attr)
	}
}

// pythonImportToDist maps Python import names to canonical PyPI distribution
// names for the common cases where they differ. Values are stored in PEP 503
// form (hyphens, lowercase) — `normalizeDist` converts to the active label
// normalization mode at render time. Users add overrides via
// `# gazelle:resolve py <import> <label>` or by extending gazelle_python.yaml.
var pythonImportToDist = map[string]string{
	"cv2":      "opencv-python",
	"PIL":      "pillow",
	"sklearn":  "scikit-learn",
	"yaml":     "pyyaml",
	"bs4":      "beautifulsoup4",
	"OpenSSL":  "pyopenssl",
	"dateutil": "python-dateutil",
}

// normalizeDist converts a top-level Python module name to its conventional
// PyPI distribution-name label form, dispatching on the active normalization:
//
//   - snake_case (default): lowercase + [-.] → "_"
//   - pep503:               lowercase + runs of [-_.] → single "-"
//   - none:                 identity (after the import→dist map lookup)
func normalizeDist(name string, mode labelNormalizationType) string {
	if d, ok := pythonImportToDist[name]; ok {
		name = d
	}
	switch mode {
	case noneNormalization:
		return name
	case pep503Normalization:
		name = strings.ToLower(name)
		// Collapse any run of [-_.] into a single "-".
		var b strings.Builder
		b.Grow(len(name))
		prevSep := false
		for i := 0; i < len(name); i++ {
			ch := name[i]
			if ch == '-' || ch == '_' || ch == '.' {
				if !prevSep {
					b.WriteByte('-')
					prevSep = true
				}
				continue
			}
			b.WriteByte(ch)
			prevSep = false
		}
		return b.String()
	default: // snakeCaseNormalization
		name = strings.ReplaceAll(name, "-", "_")
		name = strings.ReplaceAll(name, ".", "_")
		return strings.ToLower(name)
	}
}

// pipLabel renders a `@<repo>//<dist>` label using cfg.pipLinkPattern.
func pipLabel(cfg *pyConfig, distName string) string {
	return strings.ReplaceAll(cfg.pipLinkPattern, "{pkg}", distName)
}

// pipLabelForRepo renders a label with an explicit pip repo name. If the
// pattern doesn't include a repo placeholder we fall back to the configured
// pattern unchanged.
func pipLabelForRepo(pattern, repo, distName string) string {
	if repo == "" {
		return strings.ReplaceAll(pattern, "{pkg}", distName)
	}
	// Replace the leading "@<existing>//" with "@<repo>//" if the user's
	// pattern is a typical "@pip//{pkg}" form. Otherwise just substitute {pkg}.
	if strings.HasPrefix(pattern, "@") {
		if i := strings.Index(pattern, "//"); i > 0 {
			rest := pattern[i+2:]
			rest = strings.ReplaceAll(rest, "{pkg}", distName)
			return "@" + repo + "//" + rest
		}
	}
	return strings.ReplaceAll(pattern, "{pkg}", distName)
}

// parsePipRepo extracts "pip" from "@pip//{pkg}" so we can swap it when
// gazelle_python.yaml names a different pip repo.
func parsePipRepo(pattern string) string {
	if !strings.HasPrefix(pattern, "@") {
		return ""
	}
	rest := pattern[1:]
	if i := strings.Index(rest, "/"); i > 0 {
		return rest[:i]
	}
	return ""
}

func (l *pyLang) projectDeps(repoRoot string, cfg *pyConfig) map[string]bool {
	projectRoot := repoRoot
	if cfg.pythonRoot != "" {
		projectRoot = filepath.Join(repoRoot, cfg.pythonRoot)
	}

	l.packageDepsMu.Lock()
	if l.packageDepsByRoot == nil {
		l.packageDepsByRoot = map[string]map[string]bool{}
	}
	if deps, ok := l.packageDepsByRoot[projectRoot]; ok {
		l.packageDepsMu.Unlock()
		return deps
	}
	l.packageDepsMu.Unlock()

	deps := loadProjectDeps(projectRoot)

	l.packageDepsMu.Lock()
	if existing, ok := l.packageDepsByRoot[projectRoot]; ok {
		l.packageDepsMu.Unlock()
		return existing
	}
	l.packageDepsByRoot[projectRoot] = deps
	l.packageDepsMu.Unlock()
	return deps
}

// loadProjectDeps reads pyproject.toml and/or requirements.txt at a Python
// project root and returns declared distribution names. Best-effort — neither
// file is required, and the parser is intentionally simple.
func loadProjectDeps(projectRoot string) map[string]bool {
	deps := map[string]bool{}
	if data, err := os.ReadFile(filepath.Join(projectRoot, "pyproject.toml")); err == nil {
		for _, name := range scanPyProjectDeps(string(data)) {
			deps[name] = true
		}
	}

	for _, name := range []string{"requirements.txt", "requirements.in"} {
		f, err := os.Open(filepath.Join(projectRoot, name))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if dep := parseRequirementLine(scanner.Text()); dep != "" {
				deps[dep] = true
			}
		}
		f.Close()
	}
	return deps
}

// scanPyProjectDeps decodes pyproject.toml's `[project].dependencies` array
// and returns each entry's distribution name (after parseRequirementLine
// strips extras and version specifiers).
//
// We use BurntSushi/toml rather than hand-rolled bracket scanning because
// dependency strings can legitimately contain `]` (extras like
// `celery[redis]`), which a naive `strings.Index(content, "]")` would
// mistake for the array terminator.
func scanPyProjectDeps(content string) []string {
	var doc struct {
		Project struct {
			Dependencies []string `toml:"dependencies"`
		} `toml:"project"`
	}
	if _, err := toml.Decode(content, &doc); err != nil {
		// Best-effort: a malformed pyproject.toml shouldn't fail the
		// gazelle run; we just skip its declared deps.
		return nil
	}
	var out []string
	for _, line := range doc.Project.Dependencies {
		if name := parseRequirementLine(line); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// parseRequirementLine extracts the distribution name from one line of a
// requirements.txt / pyproject.toml deps array entry. Strips comments,
// extras (`pkg[extra]`), version specifiers (`pkg==1.0`), and environment
// markers (`pkg ; python_version<'3.10'`). Returns "" for blank/comment lines.
func parseRequirementLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
		return ""
	}
	if i := strings.Index(line, "#"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if i := strings.Index(line, ";"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	for _, sep := range []string{"==", ">=", "<=", "~=", "!=", "<", ">", "[", " "} {
		if i := strings.Index(line, sep); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
	}
	return strings.ToLower(strings.ReplaceAll(line, "-", "_"))
}

// deduplicateAndSort returns a sorted unique copy of items.
func deduplicateAndSort(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		if !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	sort.Strings(out)
	return out
}
