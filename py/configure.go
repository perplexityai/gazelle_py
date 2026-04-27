package py

import (
	"flag"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// All directives this plugin recognizes. Keep in sync with README.md.
const (
	directiveEnabled        = "py_enabled"
	directiveLibraryName    = "py_library_name"
	directiveTestName       = "py_test_name"
	directiveLibraryKind    = "py_library_kind"
	directiveTestKind       = "py_test_kind"
	directiveVisibility     = "py_visibility"
	directiveTestPattern    = "py_test_pattern"
	directiveExtension      = "py_extension"
	directivePipLinkPattern = "py_pip_link_pattern"
	directiveTestData       = "py_test_data"
	// directiveManifest points at a gazelle_python.yaml file (relative to repo
	// root) whose `modules_mapping` overrides our internal import → distribution
	// table. Set this when working with rules_python's pip_parse, which is
	// already configured to read the same file.
	directiveManifest = "py_manifest"
)

// RegisterFlags is a no-op — all configuration is via BUILD-file directives.
func (l *pyLang) RegisterFlags(fs *flag.FlagSet, cmd string, c *config.Config) {}

// CheckFlags is a no-op — there are no flags to validate.
func (l *pyLang) CheckFlags(fs *flag.FlagSet, c *config.Config) error { return nil }

// KnownDirectives returns the directive keys this plugin reads. Gazelle
// silently ignores any directive whose key isn't in this list.
func (l *pyLang) KnownDirectives() []string {
	return []string{
		directiveEnabled,
		directiveLibraryName,
		directiveTestName,
		directiveLibraryKind,
		directiveTestKind,
		directiveVisibility,
		directiveTestPattern,
		directiveExtension,
		directivePipLinkPattern,
		directiveTestData,
		directiveManifest,
	}
}

// Configure builds the per-directory config by cloning the parent config and
// applying any directives present in the current BUILD file. At the repo root
// it also loads dep declarations for import resolution.
func (l *pyLang) Configure(c *config.Config, rel string, f *rule.File) {
	var cfg *pyConfig
	if raw, ok := c.Exts[languageName]; ok {
		cfg = raw.(*pyConfig).clone()
	} else {
		cfg = newPyConfig()
	}

	if f != nil {
		for _, d := range f.Directives {
			applyDirective(cfg, d)
		}
	}

	c.Exts[languageName] = cfg

	if rel == "" {
		l.loadProjectDeps(c.RepoRoot)
	}
}

func applyDirective(cfg *pyConfig, d rule.Directive) {
	val := strings.TrimSpace(d.Value)
	switch d.Key {
	case directiveEnabled:
		cfg.enabled = parseBool(val, cfg.enabled)
	case directiveLibraryName:
		if val != "" {
			cfg.libraryName = val
		}
	case directiveTestName:
		if val != "" {
			cfg.testName = val
		}
	case directiveLibraryKind:
		if val != "" {
			cfg.libraryKind = val
		}
	case directiveTestKind:
		if val != "" {
			cfg.testKind = val
		}
	case directiveVisibility:
		if val != "" {
			cfg.visibility = splitFields(val)
		}
	case directiveTestPattern:
		if val != "" {
			cfg.testPatterns = appendUnique(cfg.testPatterns, val)
		}
	case directiveExtension:
		if val != "" {
			cfg.extensions = appendUnique(cfg.extensions, val)
		}
	case directivePipLinkPattern:
		if val != "" {
			cfg.pipLinkPattern = val
		}
	case directiveTestData:
		if val != "" {
			cfg.testData = appendUnique(cfg.testData, val)
		}
	case directiveManifest:
		if val != "" {
			cfg.manifestPath = val
		}
	}
}

func parseBool(val string, fallback bool) bool {
	switch strings.ToLower(val) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return fallback
}

func splitFields(s string) []string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func appendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}
