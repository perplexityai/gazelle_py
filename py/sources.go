package py

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/bmatcuk/doublestar/v4"
)

func rulePythonSourceFilesFromSpecs(cfg *pyConfig, r *rule.Rule, specs []FileSpec, rel string) ([]string, bool) {
	if r.Attr("srcs") != nil {
		return filterPythonSources(r.AttrStrings("srcs"), cfg), true
	}

	patterns := r.AttrStrings("file_patterns")
	if len(patterns) == 0 {
		return nil, false
	}

	return rulePythonSourceFilesFromPatterns(cfg, patterns, r.AttrStrings("ignore_patterns"), func(pattern string) ([]string, error) {
		var matches []string
		for _, s := range specs {
			src := filepath.ToSlash(pkgRelativePath(s.RelPath, rel))
			ok, err := doublestar.Match(filepath.ToSlash(pattern), src)
			if err != nil {
				return nil, err
			}
			if ok {
				matches = append(matches, src)
			}
		}
		return matches, nil
	}), true
}

func rulePythonSourceFilesFromDisk(cfg *pyConfig, r *rule.Rule, repoRoot string, pkg string) ([]string, bool) {
	if r.Attr("srcs") != nil {
		return filterPythonSources(r.AttrStrings("srcs"), cfg), true
	}

	patterns := r.AttrStrings("file_patterns")
	if len(patterns) == 0 {
		return nil, false
	}

	pkgDir := filepath.Join(repoRoot, pkg)
	return rulePythonSourceFilesFromPatterns(cfg, patterns, r.AttrStrings("ignore_patterns"), func(pattern string) ([]string, error) {
		return doublestar.Glob(os.DirFS(pkgDir), filepath.ToSlash(pattern))
	}), true
}

func rulePythonSourceFilesFromPatterns(
	cfg *pyConfig,
	patterns []string,
	ignorePatterns []string,
	filesForPattern func(pattern string) ([]string, error),
) []string {
	seen := map[string]bool{}
	var srcs []string
	for _, pattern := range patterns {
		matches, err := filesForPattern(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			src := filepath.ToSlash(match)
			if !isPythonFile(src, cfg) || matchesAnyPattern(ignorePatterns, src) {
				continue
			}
			if seen[src] {
				continue
			}
			seen[src] = true
			srcs = append(srcs, src)
		}
	}
	sort.Strings(srcs)
	return srcs
}

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
