package py

import (
	"flag"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// All directives this plugin recognizes. Keep in sync with README.md.
//
// Directive keys mirror rules_python's gazelle plugin so consumers can
// switch between the two without rewriting their BUILD-file directives.
// One exception: rules_python uses `python_extension` for the on/off
// toggle; we keep that meaning and use `python_source_extension` for
// our file-extensions list (where rules_python hardcodes `.py`/`.pyi`).
const (
	directiveEnabled         = "python_extension"
	directiveLibraryName     = "python_library_naming_convention"
	directiveTestName        = "python_test_naming_convention"
	directiveLibraryKind     = "python_library_kind"
	directiveTestKind        = "python_test_kind"
	directiveVisibility      = "python_visibility"
	directiveTestPattern     = "python_test_file_pattern"
	directiveSourceExtension = "python_source_extension"
	directiveLabelConvention = "python_label_convention"
	directiveTestData        = "python_test_data"
	// directiveManifest points at a gazelle_python.yaml file (relative to repo
	// root) whose `modules_mapping` overrides our internal import → distribution
	// table. Set this when working with rules_python's pip_parse, which is
	// already configured to read the same file.
	directiveManifest = "python_manifest_file_name"
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
		directiveSourceExtension,
		directiveLabelConvention,
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
		// rules_python takes "enabled"/"disabled" verbatim; we additionally
		// accept the bool forms (true/false/1/0/yes/no/on/off) for ergonomics.
		switch strings.ToLower(val) {
		case "enabled":
			cfg.enabled = true
		case "disabled":
			cfg.enabled = false
		default:
			cfg.enabled = parseBool(val, cfg.enabled)
		}
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
	case directiveSourceExtension:
		if val != "" {
			cfg.extensions = appendUnique(cfg.extensions, val)
		}
	case directiveLabelConvention:
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
