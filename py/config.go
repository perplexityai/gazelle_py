package py

// Default values applied when no directive overrides them. These match the
// shape a "small typical" Python package emits with stock rules_python.
const (
	// Empty default means "use the package's directory basename" — see
	// resolveRuleNames in generate.go. This way //apps/server:server shortens
	// to //apps/server, the most natural Bazel idiom.
	defaultLibraryName    = ""
	defaultTestName       = ""
	defaultLibraryKind    = "py_library"
	defaultTestKind       = "py_test"
	defaultPipLinkPattern = "@pip//{pkg}"
)

// labelNormalizationType selects how distribution names are normalized when
// rendering pip labels. Mirrors rules_python's `python_label_normalization`
// directive values.
type labelNormalizationType int

const (
	// snakeCaseNormalization (rules_python's default): lowercase + hyphens to
	// underscores. Matches what rules_python's pip_parse emits today.
	snakeCaseNormalization labelNormalizationType = iota
	// pep503Normalization (PEP 503): lowercase + runs of [-_.] → single "-".
	// Used by some pip-repo flavors that key directly on PEP 503 names.
	pep503Normalization
	// noneNormalization: identity. Useful when the manifest already supplies
	// the exact label form.
	noneNormalization
)

// generationModeType selects how rules are produced per directory. Mirrors
// rules_python's `python_generation_mode` directive values.
type generationModeType int

const (
	// generationModePackage (rules_python's default): one library + optional
	// test rule per directory.
	generationModePackage generationModeType = iota
	// generationModeFile: one library rule per source file. Useful when
	// individual modules need finer-grained dep tracking.
	generationModeFile
	// generationModeProject: one library rule at the directory that set the
	// directive, with sources rolled up across all subdirectories. Children
	// inside the project tree skip rule generation entirely.
	generationModeProject
)

// Default test-file patterns and source-file extensions. Patterns are matched
// against the file path relative to the package directory.
var (
	defaultTestPatterns = []string{"*_test.py", "test_*.py", "tests/**", "test/**"}
	defaultExtensions   = []string{".py"}
	defaultVisibility   = []string{"//visibility:public"}
)

// pyConfig holds per-directory configuration. Gazelle calls Configure() for
// each directory during the walk, building up the config by cloning the
// parent and applying any directives in the directory's BUILD file.
type pyConfig struct {
	enabled bool

	// libraryName / testName are the names of the generated rules.
	libraryName string
	testName    string

	// libraryKind / testKind are the rule kinds emitted. Stock defaults are
	// `py_library` and `py_test`; override via directive when you'd rather
	// emit a different kind directly than rewrite via `# gazelle:map_kind`.
	libraryKind string
	testKind    string

	// visibility is the list of labels emitted on the library rule.
	visibility []string

	// testPatterns: glob-style patterns deciding which files are tests.
	testPatterns []string

	// extensions: file extensions treated as Python source.
	extensions []string

	// pipLinkPattern is the template used for PyPI package labels, e.g.
	// `@pip//{pkg}`. The literal `{pkg}` is replaced with the resolved
	// distribution name (lowercased, hyphens → underscores).
	pipLinkPattern string

	// manifestPath is the workspace-relative path to a gazelle_python.yaml
	// file (the same format rules_python_gazelle_plugin reads). Empty means
	// "no manifest" — the resolver falls back to derivation from
	// pyproject.toml / requirements.txt.
	manifestPath string

	// pythonRoot is the workspace-relative directory considered the root of
	// the Python module tree. All dotted import paths are interpreted relative
	// to this prefix when registering libraries in the RuleIndex. Empty means
	// "the workspace root itself", which is the most common single-project
	// layout. Set via the `python_root` directive on a parent BUILD file when
	// you have multiple Python projects sharing the same Bazel workspace
	// (e.g. `backend/`, `tools/python/`).
	pythonRoot string

	// resolveSiblingImports controls whether bare-module imports
	// (`from app import X`) are resolved as siblings of the importer's
	// package. When true, the resolver also tries `<from.pkg>.<module>`
	// against the rule index, so a sibling `app.py` resolves to the local
	// library even when the test references it as a top-level module name.
	// Default false to match rules_python's default and avoid surprising
	// cross-package matches.
	resolveSiblingImports bool

	// labelNormalization selects how distribution names are normalized when
	// rendering pip labels (see labelNormalizationType). Default snake_case
	// matches rules_python's pip_parse behavior.
	labelNormalization labelNormalizationType

	// generationMode controls per-directory rule emission shape (see
	// generationModeType). Default `package` matches rules_python.
	generationMode generationModeType

	// projectRoot is the workspace-relative directory at which `python_generation_mode
	// project` was last set. When `generationMode == generationModeProject`,
	// rules are only emitted at this directory; subdirectories return empty.
	// Tracked separately from `pythonRoot` (which controls module-path
	// resolution and is unrelated to rule rollup).
	projectRoot string

	// skipEmptyInit, when true, prevents emitting a library rule when every
	// source in the rule is an empty (or comments-only) `__init__.py`. Covers
	// both single-file packages and project-mode rollups of nested empty
	// inits. Mirrors rules_python's `python_skip_empty_init`.
	skipEmptyInit bool
}

// newPyConfig returns a config populated with all defaults.
func newPyConfig() *pyConfig {
	return &pyConfig{
		enabled:            true,
		libraryName:        defaultLibraryName,
		testName:           defaultTestName,
		libraryKind:        defaultLibraryKind,
		testKind:           defaultTestKind,
		visibility:         append([]string(nil), defaultVisibility...),
		testPatterns:       append([]string(nil), defaultTestPatterns...),
		extensions:         append([]string(nil), defaultExtensions...),
		pipLinkPattern:     defaultPipLinkPattern,
		labelNormalization: snakeCaseNormalization,
	}
}

// clone makes a deep copy so child directories inherit but can override
// without mutating the parent.
func (c *pyConfig) clone() *pyConfig {
	cp := *c
	cp.visibility = append([]string(nil), c.visibility...)
	cp.testPatterns = append([]string(nil), c.testPatterns...)
	cp.extensions = append([]string(nil), c.extensions...)
	return &cp
}
