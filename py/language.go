// Package py implements a Gazelle language extension for Python packages.
//
// It generates stock rules_python rules:
//
//   - py_library (from @rules_python//python:defs.bzl) for libraries
//   - py_test    (from @rules_python//python:defs.bzl) for tests
//
// Callsites that want their own macros wrap the generated kinds via
// # gazelle:map_kind, e.g.
//
//	# gazelle:map_kind py_library myrepo_py_library //tools:py.bzl
//	# gazelle:map_kind py_test    myrepo_py_test    //tools:py.bzl
//
// The plugin operates in Gazelle's three-phase pipeline:
//
//  1. GenerateRules (generate.go): scan .py files, extract imports via
//     the import_extractor cgo FFI, create/update rules.
//  2. Imports (imports.go): register rules in the RuleIndex so other
//     packages can resolve their imports against ours.
//  3. Resolve (resolve.go): convert parsed imports into Bazel deps labels.
//
// All configuration lives in BUILD-file directives (see configure.go); see
// README.md for the full list and examples.
package py

import (
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// languageName is the unique identifier for this Gazelle extension. It must
// match the prefix used in directive keys (py_enabled, py_library_name, …).
const languageName = "py"

// pyLang implements the language.Language interface from Gazelle.
type pyLang struct {
	// packageDeps is a set of all PyPI package distribution names declared
	// at the repo root (e.g. via pyproject.toml [project] dependencies or
	// requirements.txt). Used by the resolver to gate label emission for
	// non-stdlib imports.
	packageDeps map[string]bool
}

// NewLanguage creates a new Python Gazelle language extension.
func NewLanguage() language.Language {
	return &pyLang{
		packageDeps: make(map[string]bool),
	}
}

func (l *pyLang) Name() string { return languageName }

// Embeds returns nil — Python doesn't use Bazel's rule embedding mechanism.
func (l *pyLang) Embeds(r *rule.Rule, from label.Label) []label.Label { return nil }
