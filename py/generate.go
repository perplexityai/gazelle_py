package py

import (
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
func (l *pyLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	cfg, ok := args.Config.Exts[languageName].(*pyConfig)
	if !ok || !cfg.enabled {
		return language.GenerateResult{}
	}

	libName, testName := resolveRuleNames(cfg, args.Rel)
	libSrcs, testSrcs := collectSrcs(args.RegularFiles, cfg)

	// Build the (abs, rel) pair list the parser wants. Rel is what shows up
	// in `result.FileName`, so resolve.go can match imports back to a file
	// without juggling absolute paths.
	var specs []FileSpec
	for _, f := range args.RegularFiles {
		if !isPythonFile(f, cfg) {
			continue
		}
		specs = append(specs, FileSpec{
			Path:    filepath.Join(args.Dir, f),
			RelPath: filepath.Join(args.Rel, f),
		})
	}

	var (
		sourceImports []ImportStatement
		testImports   []ImportStatement
		allComments   []string
	)
	if len(specs) > 0 {
		results, err := l.extractImportsBatch(specs)
		if err != nil {
			// We don't fail the whole gazelle run on a parser error — we just
			// drop this directory's imports. The next run picks them up after
			// the user fixes whatever made the parser unhappy.
			results = nil
		}
		for _, f := range args.RegularFiles {
			if !isPythonFile(f, cfg) {
				continue
			}
			rel := filepath.Join(args.Rel, f)
			r, ok := results[rel]
			if !ok {
				continue
			}
			allComments = append(allComments, r.Comments...)
			if isTestFile(f, cfg) {
				testImports = append(testImports, r.Modules...)
			} else {
				sourceImports = append(sourceImports, r.Modules...)
			}
		}
	}

	annot := parseAnnotations(allComments)

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

// resolveRuleNames returns the (library, test) rule names for a directory,
// applying the directive overrides if set or falling back to package-name-
// derived defaults.
//
// Defaults — given a package at //apps/server (rel = "apps/server"):
//
//	library: "server"      → //apps/server:server (Bazel shortens to //apps/server)
//	test:    "server_test" → //apps/server:server_test
//
// Both can be overridden per-tree via the py_library_name / py_test_name
// directives. At the repo root (rel = ""), where there's no basename to
// derive from, library falls back to "lib" and test to "test".
func resolveRuleNames(cfg *pyConfig, rel string) (libName, testName string) {
	base := filepath.Base(rel)
	if base == "." || base == "" || base == "/" {
		base = ""
	}

	libName = cfg.libraryName
	if libName == "" {
		if base != "" {
			libName = base
		} else {
			libName = "lib"
		}
	}

	testName = cfg.testName
	if testName == "" {
		if base != "" {
			testName = base + "_test"
		} else {
			testName = "test"
		}
	}
	return
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
