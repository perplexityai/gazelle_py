package py

import "strings"

// annotations are parsed from `# gazelle:...` lines inside Python source files.
// They live alongside imports and direct the resolver to skip or pin certain
// modules.
//
//	# gazelle:ignore foo,bar
//	# gazelle:ignore baz
//	# gazelle:include_dep //some:label
type annotations struct {
	ignore     map[string]bool // module names to skip during resolution
	includeDep []string        // labels to always add to deps
}

// parseAnnotations walks comment lines and extracts gazelle directives.
// Recognized prefixes: `# gazelle:ignore`, `# gazelle:include_dep`. Anything
// else is left for gazelle's own directive parsing (which works on BUILD
// files, not Python sources, so this is the only chance to read them).
func parseAnnotations(comments []string) annotations {
	a := annotations{ignore: map[string]bool{}}
	for _, c := range comments {
		c = strings.TrimSpace(c)
		switch {
		case strings.HasPrefix(c, "# gazelle:ignore"):
			parts := strings.Fields(c)
			if len(parts) < 3 {
				continue
			}
			for _, m := range parts[2:] {
				for _, sub := range strings.Split(m, ",") {
					sub = strings.TrimSpace(sub)
					if sub != "" {
						a.ignore[sub] = true
					}
				}
			}
		case strings.HasPrefix(c, "# gazelle:include_dep"):
			parts := strings.Fields(c)
			if len(parts) < 3 {
				continue
			}
			a.includeDep = append(a.includeDep, parts[2])
		}
	}
	return a
}

// isIgnored returns true when an import (and any of its dotted prefixes, plus
// its `from` part) appears in the ignore set. The prefix walk lets a single
// `# gazelle:ignore a.b` line cover `a.b.c.D`, `a.b.x.y`, etc., without forcing
// users to enumerate every leaf import.
func isIgnored(moduleName, from string, ignore map[string]bool) bool {
	if ignore[moduleName] {
		return true
	}
	if from != "" && ignore[from] {
		return true
	}
	parts := strings.Split(moduleName, ".")
	for i := len(parts); i > 0; i-- {
		if ignore[strings.Join(parts[:i], ".")] {
			return true
		}
	}
	return false
}
