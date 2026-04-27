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

	// testData is added to every emitted test rule's `data` attr.
	testData []string
}

// newPyConfig returns a config populated with all defaults.
func newPyConfig() *pyConfig {
	return &pyConfig{
		enabled:        true,
		libraryName:    defaultLibraryName,
		testName:       defaultTestName,
		libraryKind:    defaultLibraryKind,
		testKind:       defaultTestKind,
		visibility:     append([]string(nil), defaultVisibility...),
		testPatterns:   append([]string(nil), defaultTestPatterns...),
		extensions:     append([]string(nil), defaultExtensions...),
		pipLinkPattern: defaultPipLinkPattern,
	}
}

// clone makes a deep copy so child directories inherit but can override
// without mutating the parent.
func (c *pyConfig) clone() *pyConfig {
	cp := *c
	cp.visibility = append([]string(nil), c.visibility...)
	cp.testPatterns = append([]string(nil), c.testPatterns...)
	cp.extensions = append([]string(nil), c.extensions...)
	cp.testData = append([]string(nil), c.testData...)
	return &cp
}
