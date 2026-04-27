package py

import (
	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// Fix performs post-merge processing. We don't currently rewrite anything.
func (l *pyLang) Fix(c *config.Config, f *rule.File) {}
