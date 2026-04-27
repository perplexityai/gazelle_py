package py

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// ImportData carries parsed imports from GenerateRules to Resolve. Gazelle
// runs GenerateRules during the directory walk (before the RuleIndex is
// complete) and Resolve afterwards, so we stash everything we'll need here.
type ImportData struct {
	Imports     []ImportStatement // source-file imports
	TestImports []ImportStatement // test-file imports
}

// GenerateRules walks a directory's files, partitions them into source vs.
// test, parses imports via the Rust subprocess, and emits library + test
// rules. The merge engine reconciles the result with the existing BUILD
// content using KindInfo from kinds.go.
func (l *pyLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	cfg, ok := args.Config.Exts[languageName].(*pyConfig)
	if !ok || !cfg.enabled {
		return language.GenerateResult{}
	}

	libName, testName := resolveRuleNames(cfg, args.Rel)
	libSrcs, testSrcs := collectSrcs(args.RegularFiles, cfg)

	var pyFiles []string
	for _, f := range args.RegularFiles {
		if isPythonFile(f, cfg) {
			pyFiles = append(pyFiles, filepath.Join(args.Dir, f))
		}
	}

	var sourceImports, testImports []ImportStatement
	allImports := map[string][]ImportStatement{}
	if len(pyFiles) > 0 {
		allImports, _ = l.extractImportsBatch(pyFiles)
		for _, f := range args.RegularFiles {
			if !isPythonFile(f, cfg) {
				continue
			}
			fullPath := filepath.Join(args.Dir, f)
			imps := allImports[fullPath]
			if isTestFile(f, cfg) {
				testImports = append(testImports, imps...)
			} else {
				sourceImports = append(sourceImports, imps...)
			}
		}
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
		genImports = append(genImports, ImportData{Imports: sourceImports})
	}

	if len(testSrcs) > 0 {
		r := rule.NewRule(cfg.testKind, testName)
		r.SetAttr("srcs", testSrcs)
		if len(cfg.testData) > 0 {
			r.SetAttr("data", cfg.testData)
		}
		// py_test requires a `main` attr (the entry script). Pick the first
		// test file alphabetically; users can override after the first run
		// — the merge engine preserves a manually-set main across runs.
		if len(testSrcs) > 0 {
			r.SetAttr("main", testSrcs[0])
		}
		genRules = append(genRules, r)
		genImports = append(genImports, ImportData{
			Imports:     sourceImports,
			TestImports: testImports,
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
	// `test_*.py` (literal `*` in middle) doesn't get special treatment;
	// `prefix*suffix` patterns:
	if i := strings.Index(pattern, "*"); i > 0 && i < len(pattern)-1 {
		prefix := pattern[:i]
		suffix := pattern[i+1:]
		return strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) && len(name) >= len(prefix)+len(suffix)
	}
	return name == pattern
}

// collectSrcs partitions the directory's files into library and test srcs,
// each sorted for deterministic BUILD output. Skips __init__.py — it is
// emitted as part of `srcs` on the package's own library rule (callers add
// it to the directory's regular files; we treat it like any other .py).
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
