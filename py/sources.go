package py

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

func filterPythonSources(srcs []string, cfg *pyConfig) []string {
	out := make([]string, 0, len(srcs))
	for _, src := range srcs {
		src = filepath.ToSlash(src)
		if isPythonFile(src, cfg) {
			out = append(out, src)
		}
	}
	sort.Strings(out)
	return out
}

func matchesAnyPattern(patterns []string, src string) bool {
	for _, pattern := range patterns {
		ok, err := doublestar.Match(filepath.ToSlash(pattern), src)
		if ok && err == nil {
			return true
		}
	}
	return false
}

func importPathForSource(pkg string, src string) (string, bool) {
	src = filepath.ToSlash(src)
	if !strings.HasSuffix(src, ".py") {
		return "", false
	}

	module := strings.TrimSuffix(src, ".py")
	parts := strings.Split(module, "/")
	if len(parts) > 0 && parts[len(parts)-1] == "__init__" {
		parts = parts[:len(parts)-1]
	}

	var suffix string
	if len(parts) > 0 {
		suffix = strings.Join(parts, ".")
	}
	switch {
	case pkg == "" && suffix == "":
		return "", false
	case pkg == "":
		return suffix, true
	case suffix == "":
		return pkg, true
	default:
		return pkg + "." + suffix, true
	}
}
