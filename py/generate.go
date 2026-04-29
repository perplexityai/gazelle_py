package py

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// ImportData carries parsed imports + annotations from GenerateRules to
// Resolve. Gazelle runs GenerateRules during the directory walk (before the
// RuleIndex is complete) and Resolve afterwards, so we stash everything we'll
// need later. The two import slices are kept apart because Resolve attaches
// them to different rules (library vs test).
type ImportData struct {
	Imports     []ImportStatement // source-file imports
	TestImports []ImportStatement // test-file imports
	Ignore      map[string]bool   // module names to skip during resolution
	IncludeDeps []string          // labels to always add to deps
}

// GenerateRules walks a directory's files, partitions them into source vs.
// test, parses imports via the cgo-bound import_extractor, and emits library +
// test rules. The merge engine reconciles the result with the existing BUILD
// content using KindInfo from kinds.go.
//
// Generation shape is selected by `python_generation_mode`:
//   - package (default): one library + one test rule per directory.
//   - file:              one library/test rule per .py file.
//   - project:           at the directory the directive was set on, roll up
//     every .py file under the subtree into one library/test rule. In
//     subdirectories within that project root, generate nothing.
func (l *pyLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	cfg, ok := args.Config.Exts[languageName].(*pyConfig)
	if !ok || !cfg.enabled {
		return language.GenerateResult{}
	}

	// Project mode: only the directory that introduced the directive emits
	// rules. Children inside that subtree return empty.
	if cfg.generationMode == generationModeProject && args.Rel != cfg.projectRoot {
		return language.GenerateResult{}
	}

	specs := pythonFileSpecs(cfg, args.Dir, args.Rel, args.RegularFiles)
	if cfg.generationMode == generationModeProject {
		// Walk the subtree below this directory and append every .py we find
		// (excluding nested project subdirs and BUILD-pinned subtrees, which
		// Bazel would refuse to glob into).
		walkSpecs, err := walkProjectFiles(cfg, args.Dir, args.Rel)
		if err == nil {
			specs = append(specs, walkSpecs...)
		}
	}

	results, allComments := l.parseSpecs(specs)
	annot := parseAnnotations(allComments)

	switch cfg.generationMode {
	case generationModeFile:
		return generatePerFileRules(cfg, args.Rel, specs, results, annot)
	default:
		return generateAggregateRules(cfg, args.Rel, specs, results, annot)
	}
}

// pythonFileSpecs maps the per-directory regular files to FileSpecs the
// parser expects. RelPath is filepath.Join(rel, name); the parser's
// returned `FileName` matches that, letting Resolve match imports back to
// files without juggling absolute paths.
func pythonFileSpecs(cfg *pyConfig, dir, rel string, files []string) []FileSpec {
	var specs []FileSpec
	for _, f := range files {
		if !isPythonFile(f, cfg) {
			continue
		}
		specs = append(specs, FileSpec{
			Path:    filepath.Join(dir, f),
			RelPath: filepath.Join(rel, f),
		})
	}
	return specs
}

// walkProjectFiles enumerates every Python source under `rootDir` (excluding
// the directory itself, which the caller has already handled) and returns
// FileSpecs with RelPath rooted at the project's package directory `rootRel`.
// Used by `python_generation_mode project` to roll up subtree sources.
func walkProjectFiles(cfg *pyConfig, rootDir, rootRel string) ([]FileSpec, error) {
	var specs []FileSpec
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == rootDir {
			return nil
		}
		if d.IsDir() {
			// Don't descend into directories that have their own BUILD file
			// — Bazel would treat those as separate packages and the glob
			// wouldn't reach across the boundary.
			if hasBuildFile(path) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !isPythonFile(name, cfg) {
			return nil
		}
		relFromRoot, _ := filepath.Rel(rootDir, path)
		specs = append(specs, FileSpec{
			Path:    path,
			RelPath: filepath.Join(rootRel, relFromRoot),
		})
		return nil
	})
	return specs, err
}

func hasBuildFile(dir string) bool {
	for _, name := range []string{"BUILD.bazel", "BUILD"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// parseSpecs runs the import extractor over `specs` and returns the parsed
// results keyed by RelPath plus a flat list of every comment encountered
// (used by parseAnnotations for `# gazelle:ignore` / `# gazelle:include_dep`).
func (l *pyLang) parseSpecs(specs []FileSpec) (map[string]FileImports, []string) {
	if len(specs) == 0 {
		return nil, nil
	}
	results, err := l.extractImportsBatch(specs)
	if err != nil {
		// We don't fail the whole gazelle run on a parser error — we just
		// drop this directory's imports. The next run picks them up after
		// the user fixes whatever made the parser unhappy.
		return nil, nil
	}
	var allComments []string
	for _, s := range specs {
		if r, ok := results[s.RelPath]; ok {
			allComments = append(allComments, r.Comments...)
		}
	}
	return results, allComments
}

// generateAggregateRules emits one library + one test rule covering every
// spec passed in. Used by both `package` and `project` generation modes —
// the caller decides which specs to gather.
func generateAggregateRules(cfg *pyConfig, rel string, specs []FileSpec, results map[string]FileImports, annot annotations) language.GenerateResult {
	libName, testName := resolveRuleNames(cfg, rel)

	var libSrcs, testSrcs []string
	var sourceImports, testImports []ImportStatement
	for _, s := range specs {
		// Sources are listed relative to the package the rule lives in.
		// `rel` is the package's workspace-relative path; trimming it
		// turns "apps/server/utils/x.py" into "utils/x.py" inside
		// //apps/server's BUILD file.
		srcName := pkgRelativePath(s.RelPath, rel)
		isTest := isTestFile(srcName, cfg)
		if isTest {
			testSrcs = append(testSrcs, srcName)
		} else {
			libSrcs = append(libSrcs, srcName)
		}
		if r, ok := results[s.RelPath]; ok {
			if isTest {
				testImports = append(testImports, r.Modules...)
			} else {
				sourceImports = append(sourceImports, r.Modules...)
			}
		}
	}
	sort.Strings(libSrcs)
	sort.Strings(testSrcs)

	if cfg.skipEmptyInit && isEmptyInitOnly(libSrcs, specs, rel) {
		libSrcs = nil
	}

	if len(libSrcs) == 0 && len(testSrcs) == 0 {
		return language.GenerateResult{}
	}

	var genRules []*rule.Rule
	var genImports []interface{}

	if len(libSrcs) > 0 {
		r := rule.NewRule(cfg.libraryKind, libName)
		r.SetAttr("srcs", libSrcs)
		if len(cfg.visibility) > 0 {
			r.SetAttr("visibility", cfg.visibility)
		}
		genRules = append(genRules, r)
		genImports = append(genImports, ImportData{
			Imports:     sourceImports,
			Ignore:      annot.ignore,
			IncludeDeps: annot.includeDep,
		})
	}

	if len(testSrcs) > 0 {
		r := rule.NewRule(cfg.testKind, testName)
		r.SetAttr("srcs", testSrcs)
		if len(cfg.testData) > 0 {
			r.SetAttr("data", cfg.testData)
		}
		// py_test requires a `main` attr — pick the first test file
		// alphabetically. The merge engine preserves a manually-set main on
		// subsequent runs, so users can override after the first generation.
		r.SetAttr("main", testSrcs[0])
		genRules = append(genRules, r)
		genImports = append(genImports, ImportData{
			Imports:     sourceImports,
			TestImports: testImports,
			Ignore:      annot.ignore,
			IncludeDeps: annot.includeDep,
		})
	}

	return language.GenerateResult{
		Gen:     genRules,
		Imports: genImports,
	}
}

// generatePerFileRules emits one rule per source file: a library rule named
// after the file (e.g. `helpers.py` → `helpers`) for non-test files, and the
// configured test kind for test files. Selected by `python_generation_mode file`.
func generatePerFileRules(cfg *pyConfig, rel string, specs []FileSpec, results map[string]FileImports, annot annotations) language.GenerateResult {
	// Sort by the in-package relative path so emitted rules are stable.
	sortedSpecs := append([]FileSpec(nil), specs...)
	sort.Slice(sortedSpecs, func(i, j int) bool {
		return sortedSpecs[i].RelPath < sortedSpecs[j].RelPath
	})

	var (
		genRules   []*rule.Rule
		genImports []interface{}
		libSpecs   []FileSpec
	)
	// Tests first time around: collect lib specs to expose to test rules so
	// each test rule sees the full library import set as well as its own.
	var allLibImports []ImportStatement
	for _, s := range sortedSpecs {
		srcName := pkgRelativePath(s.RelPath, rel)
		if isTestFile(srcName, cfg) {
			continue
		}
		libSpecs = append(libSpecs, s)
		if r, ok := results[s.RelPath]; ok {
			allLibImports = append(allLibImports, r.Modules...)
		}
	}

	for _, s := range libSpecs {
		srcName := pkgRelativePath(s.RelPath, rel)
		if cfg.skipEmptyInit && isInitFile(srcName) && isEmptyPython(s.Path) {
			continue
		}
		ruleName := perFileRuleName(srcName)
		r := rule.NewRule(cfg.libraryKind, ruleName)
		r.SetAttr("srcs", []string{srcName})
		if len(cfg.visibility) > 0 {
			r.SetAttr("visibility", cfg.visibility)
		}
		genRules = append(genRules, r)
		var imports []ImportStatement
		if pr, ok := results[s.RelPath]; ok {
			imports = pr.Modules
		}
		genImports = append(genImports, ImportData{
			Imports:     imports,
			Ignore:      annot.ignore,
			IncludeDeps: annot.includeDep,
		})
	}

	for _, s := range sortedSpecs {
		srcName := pkgRelativePath(s.RelPath, rel)
		if !isTestFile(srcName, cfg) {
			continue
		}
		ruleName := perFileRuleName(srcName)
		// rules_python's file mode uses the library naming convention for
		// tests too (since each file is its own unit). Suffix with _test
		// when the bare basename collides with a sibling library rule.
		if !strings.HasSuffix(ruleName, "_test") {
			ruleName += "_test"
		}
		r := rule.NewRule(cfg.testKind, ruleName)
		r.SetAttr("srcs", []string{srcName})
		if len(cfg.testData) > 0 {
			r.SetAttr("data", cfg.testData)
		}
		r.SetAttr("main", srcName)
		genRules = append(genRules, r)
		var testMods []ImportStatement
		if pr, ok := results[s.RelPath]; ok {
			testMods = pr.Modules
		}
		genImports = append(genImports, ImportData{
			Imports:     allLibImports,
			TestImports: testMods,
			Ignore:      annot.ignore,
			IncludeDeps: annot.includeDep,
		})
	}

	if len(genRules) == 0 {
		return language.GenerateResult{}
	}
	return language.GenerateResult{
		Gen:     genRules,
		Imports: genImports,
	}
}

// pkgRelativePath drops the package prefix from a workspace-relative path.
// "apps/server/utils/x.py" within package "apps/server" → "utils/x.py".
func pkgRelativePath(workspaceRel, pkg string) string {
	if pkg == "" {
		return workspaceRel
	}
	if workspaceRel == pkg {
		return filepath.Base(workspaceRel)
	}
	prefix := pkg + string(filepath.Separator)
	if strings.HasPrefix(workspaceRel, prefix) {
		return strings.TrimPrefix(workspaceRel, prefix)
	}
	return workspaceRel
}

func perFileRuleName(srcName string) string {
	base := filepath.Base(srcName)
	for _, ext := range []string{".py", ".pyi"} {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

func isInitFile(name string) bool {
	return filepath.Base(name) == "__init__.py"
}

// isEmptyInitOnly returns true when the package's only library source is an
// empty (or comments-only) __init__.py — the trigger for `python_skip_empty_init`.
func isEmptyInitOnly(libSrcs []string, specs []FileSpec, rel string) bool {
	if len(libSrcs) != 1 || libSrcs[0] != "__init__.py" {
		return false
	}
	for _, s := range specs {
		if pkgRelativePath(s.RelPath, rel) == "__init__.py" {
			return isEmptyPython(s.Path)
		}
	}
	return false
}

// isEmptyPython returns true when a .py file contains no code: only blank
// lines and comments. A single `pass` statement still counts as code.
func isEmptyPython(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		// On read error, fall back to "not empty" so we don't accidentally
		// drop a rule the user actually wants.
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return false
	}
	return true
}

// resolveRuleNames returns the (library, test) rule names for a directory,
// applying the directive overrides if set or falling back to package-name-
// derived defaults.
//
// Defaults — given a package at //apps/server (rel = "apps/server"):
//
//	library: "server"      → //apps/server:server (Bazel shortens to //apps/server)
//	test:    "server_test" → //apps/server:server_test
//
// Both can be overridden per-tree via the python_library_naming_convention /
// python_test_naming_convention directives. The directive value can include
// the rules_python `$package_name$` placeholder, which expands to the package
// basename. At the repo root (rel = ""), where there's no basename to derive
// from, library falls back to "lib" and test to "test".
func resolveRuleNames(cfg *pyConfig, rel string) (libName, testName string) {
	base := filepath.Base(rel)
	if base == "." || base == "" || base == "/" {
		base = ""
	}

	libName = applyNameConvention(cfg.libraryName, base)
	if libName == "" {
		if base != "" {
			libName = base
		} else {
			libName = "lib"
		}
	}

	testName = applyNameConvention(cfg.testName, base)
	if testName == "" {
		if base != "" {
			testName = base + "_test"
		} else {
			testName = "test"
		}
	}
	return
}

// applyNameConvention substitutes the rules_python `$package_name$` placeholder
// in a naming-convention template. Returns "" when the directive isn't set, or
// when the placeholder would expand to an empty package name (repo root): in
// that case resolveRuleNames falls back to its literal "lib"/"test" defaults.
func applyNameConvention(template, pkgBase string) string {
	if template == "" {
		return ""
	}
	if !strings.Contains(template, "$package_name$") {
		return template
	}
	if pkgBase == "" {
		return ""
	}
	return strings.ReplaceAll(template, "$package_name$", pkgBase)
}

// isPythonFile checks the configured extensions list.
func isPythonFile(name string, cfg *pyConfig) bool {
	for _, ext := range cfg.extensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// isTestFile matches the file path against any of the configured test
// patterns. Patterns may contain `**` (matches across directories) and `*`
// (matches within a path segment).
func isTestFile(name string, cfg *pyConfig) bool {
	for _, pat := range cfg.testPatterns {
		if matchTestPattern(pat, name) {
			return true
		}
	}
	return false
}

// matchTestPattern is a small glob matcher supporting `*` (path segment) and
// `**` (path-spanning). We avoid filepath.Match because it doesn't support `**`.
func matchTestPattern(pattern, name string) bool {
	// Fast path for prefix-style patterns ("tests/**", "test/**").
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return name == prefix || strings.HasPrefix(name, prefix+"/")
	}
	// `*_test.py` style: substring match on suffix.
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(name, strings.TrimPrefix(pattern, "*"))
	}
	// `test_*.py` style: prefix match.
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	}
	// `prefix*suffix` patterns:
	if i := strings.Index(pattern, "*"); i > 0 && i < len(pattern)-1 {
		prefix := pattern[:i]
		suffix := pattern[i+1:]
		return strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) && len(name) >= len(prefix)+len(suffix)
	}
	return name == pattern
}

// collectSrcs partitions the directory's files into library and test srcs,
// each sorted for deterministic BUILD output. We keep __init__.py in the
// library bucket so it travels with the package.
func collectSrcs(regularFiles []string, cfg *pyConfig) (libFiles, testFiles []string) {
	for _, f := range regularFiles {
		if !isPythonFile(f, cfg) {
			continue
		}
		if isTestFile(f, cfg) {
			testFiles = append(testFiles, f)
		} else {
			libFiles = append(libFiles, f)
		}
	}
	sort.Strings(libFiles)
	sort.Strings(testFiles)
	return
}
