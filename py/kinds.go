package py

import (
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// pyKinds describes how Gazelle's merge engine should reconcile our generated
// rules with existing BUILD-file content.
//
//   - NonEmptyAttrs:  attrs that must be non-empty for the rule to survive merge
//   - MergeableAttrs: attrs whose values are merged (union); `# keep` lines
//     in the existing BUILD file are preserved across regenerations
//   - ResolveAttrs:   attrs that are set by Resolve() and replace existing values
//
// Attrs not listed are left untouched if we don't set them, or overwritten if
// we do. This is how manually-set attrs like main, args, or imports survive
// gazelle runs.
//
// We always emit the stock kinds (py_library, py_test) here. When the
// consumer applies `# gazelle:map_kind py_library myrepo_py_library …`,
// gazelle rewrites the kind on disk but still uses these merge rules.
var pyKinds = map[string]rule.KindInfo{
	defaultLibraryKind: {
		NonEmptyAttrs:  map[string]bool{"name": true},
		MergeableAttrs: map[string]bool{"srcs": true},
		ResolveAttrs: map[string]bool{
			"deps": true,
		},
	},
	defaultTestKind: {
		NonEmptyAttrs:  map[string]bool{"name": true},
		MergeableAttrs: map[string]bool{"srcs": true, "data": true},
		ResolveAttrs: map[string]bool{
			"deps": true,
		},
	},
}

// Kinds tells Gazelle which rule types this plugin manages.
func (l *pyLang) Kinds() map[string]rule.KindInfo {
	return pyKinds
}

// Loads declares the .bzl files that provide the rule kinds we generate.
// Gazelle uses these to add `load()` statements at the top of BUILD files.
//
// When `# gazelle:map_kind` rewrites our kind to a custom one, the consumer
// is responsible for ensuring the appropriate load statement exists (gazelle
// looks up by the post-map kind, not by the entries here).
func (l *pyLang) Loads() []rule.LoadInfo {
	return []rule.LoadInfo{
		{
			Name:    "@rules_python//python:defs.bzl",
			Symbols: []string{defaultLibraryKind, defaultTestKind},
		},
	}
}
