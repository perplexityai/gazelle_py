package py

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	cfg, _ := c.Exts[languageName].(*pyConfig)
	if cfg == nil {
		cfg = newPyConfig()
	}
	importData, ok := rawImportData.(ImportData)
	if !ok {
		return
	}

	switch r.Kind() {
	case cfg.libraryKind:
		all := l.resolveImports(c, ix, importData, importData.Imports, from, cfg)
		all = append(all, importData.IncludeDeps...)
		setOrDelete(r, "deps", all)

	case cfg.testKind:
		// Test rules absorb the test imports plus the surrounding library's
		// imports (the test typically links everything its sibling lib does
		// plus its own deps).
		modules := append([]ImportStatement{}, importData.Imports...)
		modules = append(modules, importData.TestImports...)

		// Synthesize ancestor conftest imports. Walk from the test's own
		// directory up to the repo root, and for each ancestor that has a
		// `conftest.py`, add a synthetic import for its dotted module path.
		// The normal possible-modules loop then resolves it to whatever
		// `:conftest` library target indexes that path; if no such target
		// exists, the import is silently dropped.
		for _, syn := range conftestImportsFor(c.RepoRoot, from.Pkg) {
			modules = append(modules, syn)
		}

		all := l.resolveImports(c, ix, importData, modules, from, cfg)
		all = append(all, importData.IncludeDeps...)
		setOrDelete(r, "deps", all)
	}
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

// resolveImports walks each import through the possible-modules loop and
// returns a flat, sorted, deduped dep list.
func (l *pyLang) resolveImports(
	c *config.Config,
	ix *resolve.RuleIndex,
	importData ImportData,
	imports []ImportStatement,
	from label.Label,
	cfg *pyConfig,
) []string {
	manifest := loadManifestOnce(filepath.Join(c.RepoRoot, cfg.manifestPath))
	pipRepo := manifest.PipRepoName

	seen := map[string]bool{}
	out := []string{}

	for _, imp := range imports {
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

		dep := l.resolveOne(c, ix, from, path, imp.From, cfg, manifest, pipRepo)
		if dep == "" {
			continue
		}
		if seen[dep] {
			continue
		}
		seen[dep] = true
		out = append(out, dep)
	}

	sort.Strings(out)
	return out
}

// resolveOne implements the possible-modules loop for a single import. Returns
// the resolved dep label, or "" if no resolution applies (stdlib, self-import,
// or a missing third-party package not declared in pyproject/manifest).
func (l *pyLang) resolveOne(
	c *config.Config,
	ix *resolve.RuleIndex,
	from label.Label,
	moduleName string,
	fromPart string,
	cfg *pyConfig,
	manifest *manifestData,
	pipRepo string,
) string {
	parts := strings.Split(moduleName, ".")
	for i := len(parts); i > 0; i-- {
		try := strings.Join(parts[:i], ".")

		// 1. gazelle:resolve directive — explicit user override.
		spec := resolve.ImportSpec{Lang: languageName, Imp: try}
		if dep, ok := resolve.FindRuleWithOverride(c, spec, languageName); ok {
			lbl := dep.Rel(from.Repo, from.Pkg).String()
			if lbl == ":"+from.Name {
				return ""
			}
			return lbl
		}

		// 2. Pip manifest (gazelle_python.yaml).
		if dist, ok := manifest.ModulesMapping[try]; ok {
			repoName := pipRepo
			if repoName == "" {
				repoName = parsePipRepo(cfg.pipLinkPattern)
			}
			return pipLabelForRepo(cfg.pipLinkPattern, repoName, normalizeDist(dist))
		}

		// 3. First-party rule index — exact match.
		if hits := ix.FindRulesByImportWithConfig(c, spec, languageName); len(hits) > 0 {
			if hits[0].IsSelfImport(from) {
				return ""
			}
			return hits[0].Label.Rel(from.Repo, from.Pkg).String()
		}

		// 3b. First-party wildcard (e.g. `pkg.*` for "anything under pkg").
		wild := resolve.ImportSpec{Lang: languageName, Imp: try + ".*"}
		if hits := ix.FindRulesByImportWithConfig(c, wild, languageName); len(hits) > 0 {
			if hits[0].IsSelfImport(from) {
				return ""
			}
			return hits[0].Label.Rel(from.Repo, from.Pkg).String()
		}

		// 4. Stdlib — no dep needed.
		topLevel := strings.SplitN(try, ".", 2)[0]
		if isStdlib(topLevel) {
			return ""
		}
	}

	// Nothing matched at any prefix. Optimistically emit a pip label for the
	// top-level module name unless the user gave us a project-deps file that
	// excludes it.
	topLevel := strings.SplitN(moduleName, ".", 2)[0]
	if isStdlib(topLevel) {
		return ""
	}
	dist := normalizeDist(topLevel)
	if len(l.packageDeps) > 0 && !l.packageDeps[dist] {
		return ""
	}
	return pipLabel(cfg, dist)
}

func setOrDelete(r *rule.Rule, attr string, values []string) {
	values = deduplicateAndSort(values)
	if len(values) > 0 {
		r.SetAttr(attr, values)
	} else {
		r.DelAttr(attr)
	}
}

// pythonImportToDist maps Python import names to PyPI distribution names for
// the common cases where they differ. Users add overrides via
// `# gazelle:resolve py <import> <label>` or by extending gazelle_python.yaml.
var pythonImportToDist = map[string]string{
	"cv2":      "opencv_python",
	"PIL":      "pillow",
	"sklearn":  "scikit_learn",
	"yaml":     "pyyaml",
	"bs4":      "beautifulsoup4",
	"OpenSSL":  "pyopenssl",
	"dateutil": "python_dateutil",
}

// normalizeDist converts a top-level Python module name to its conventional
// PyPI distribution name (PEP 503 normalization: lowercase, hyphens/dots →
// underscores).
func normalizeDist(name string) string {
	if d, ok := pythonImportToDist[name]; ok {
		name = d
	}
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return strings.ToLower(name)
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

// loadProjectDeps reads pyproject.toml and/or requirements.txt at the repo
// root and seeds packageDeps with declared distribution names. Best-effort —
// neither file is required, and the parser is intentionally simple.
func (l *pyLang) loadProjectDeps(repoRoot string) {
	if len(l.packageDeps) > 0 {
		return
	}

	if data, err := os.ReadFile(repoRoot + "/pyproject.toml"); err == nil {
		for _, name := range scanPyProjectDeps(string(data)) {
			l.packageDeps[name] = true
		}
	}

	for _, name := range []string{"requirements.txt", "requirements.in"} {
		f, err := os.Open(repoRoot + "/" + name)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if dep := parseRequirementLine(scanner.Text()); dep != "" {
				l.packageDeps[dep] = true
			}
		}
		f.Close()
	}
}

// scanPyProjectDeps does a regex-free best-effort scan of pyproject.toml's
// `[project] dependencies = [...]` array. Doesn't pretend to be a TOML
// parser — just grabs the obvious literal-string entries.
func scanPyProjectDeps(content string) []string {
	var out []string
	idx := strings.Index(content, "[project]")
	if idx < 0 {
		return nil
	}
	tail := content[idx:]
	depIdx := strings.Index(tail, "dependencies")
	if depIdx < 0 {
		return nil
	}
	tail = tail[depIdx:]
	open := strings.Index(tail, "[")
	close := strings.Index(tail, "]")
	if open < 0 || close < 0 || close < open {
		return nil
	}
	body := tail[open+1 : close]
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ",")
		if len(line) >= 2 && (line[0] == '"' || line[0] == '\'') {
			line = line[1 : len(line)-1]
		}
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
