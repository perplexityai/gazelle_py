package pureftests

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Copied from py/config.go -- keep in sync.
type labelNormalizationType int

// Copied from py/config.go -- keep in sync.
const (
	// Copied from py/config.go -- keep in sync.
	snakeCaseNormalization labelNormalizationType = iota
	// Copied from py/config.go -- keep in sync.
	pep503Normalization
	// Copied from py/config.go -- keep in sync.
	noneNormalization
)

// Copied from py/resolve.go -- keep in sync.
var pythonImportToDist = map[string]string{
	"cv2":      "opencv-python",
	"PIL":      "pillow",
	"sklearn":  "scikit-learn",
	"yaml":     "pyyaml",
	"bs4":      "beautifulsoup4",
	"OpenSSL":  "pyopenssl",
	"dateutil": "python-dateutil",
}

// Copied from py/resolve.go -- keep in sync.
func normalizeDist(name string, mode labelNormalizationType) string {
	if d, ok := pythonImportToDist[name]; ok {
		name = d
	}
	switch mode {
	case noneNormalization:
		return name
	case pep503Normalization:
		name = strings.ToLower(name)
		// Collapse any run of [-_.] into a single "-".
		var b strings.Builder
		b.Grow(len(name))
		prevSep := false
		for i := 0; i < len(name); i++ {
			ch := name[i]
			if ch == '-' || ch == '_' || ch == '.' {
				if !prevSep {
					b.WriteByte('-')
					prevSep = true
				}
				continue
			}
			b.WriteByte(ch)
			prevSep = false
		}
		return b.String()
	default: // snakeCaseNormalization
		name = strings.ReplaceAll(name, "-", "_")
		name = strings.ReplaceAll(name, ".", "_")
		return strings.ToLower(name)
	}
}

// Copied from py/resolve.go -- keep in sync.
func parseRequirementLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
		return ""
	}
	if i := strings.Index(line, "#"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if i := strings.Index(line, ";"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	for _, sep := range []string{"==", ">=", "<=", "~=", "!=", "<", ">", "[", " "} {
		if i := strings.Index(line, sep); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
	}
	return strings.ToLower(strings.ReplaceAll(line, "-", "_"))
}

// Copied from py/resolve.go -- keep in sync.
func deduplicateAndSort(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		if !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	sort.Strings(out)
	return out
}

// Copied from py/resolve.go -- keep in sync.
// pipLabel renders a `@<repo>//<dist>` label using pattern instead of cfg.pipLinkPattern.
func pipLabel(pattern, distName string) string {
	return strings.ReplaceAll(pattern, "{pkg}", distName)
}

// Copied from py/resolve.go -- keep in sync.
func pipLabelForRepo(pattern, repo, distName string) string {
	if repo == "" {
		return strings.ReplaceAll(pattern, "{pkg}", distName)
	}
	// Replace the leading "@<existing>//" with "@<repo>//" if the user's
	// pattern is a typical "@pip//{pkg}" form. Otherwise just substitute {pkg}.
	if strings.HasPrefix(pattern, "@") {
		if i := strings.Index(pattern, "//"); i > 0 {
			rest := pattern[i+2:]
			rest = strings.ReplaceAll(rest, "{pkg}", distName)
			return "@" + repo + "//" + rest
		}
	}
	return strings.ReplaceAll(pattern, "{pkg}", distName)
}

// Copied from py/resolve.go -- keep in sync.
func parsePipRepo(pattern string) string {
	if !strings.HasPrefix(pattern, "@") {
		return ""
	}
	rest := pattern[1:]
	if i := strings.Index(rest, "/"); i > 0 {
		return rest[:i]
	}
	return ""
}

// Copied from py/generate.go -- keep in sync.
func applyNameConvention(template, pkgBase string) string {
	if template == "" {
		return ""
	}
	if !strings.Contains(template, "$package_name$") {
		return template
	}
	if pkgBase == "" {
		return ""
	}
	return strings.ReplaceAll(template, "$package_name$", pkgBase)
}

// Copied from py/generate.go -- keep in sync.
func matchTestPattern(pattern, name string) bool {
	ok, err := doublestar.Match(pattern, name)
	return ok && err == nil
}

// Copied from py/generate.go -- keep in sync.
func pkgRelativePath(workspaceRel, pkg string) string {
	if pkg == "" {
		return workspaceRel
	}
	if workspaceRel == pkg {
		return filepath.Base(workspaceRel)
	}
	// NOTE: Uses "/" instead of filepath.Separator because Gazelle workspace-relative
	// paths always use forward slashes regardless of host OS. The original source
	// uses filepath.Separator which is a latent bug on Windows -- see py/generate.go.
	prefix := pkg + "/"
	if strings.HasPrefix(workspaceRel, prefix) {
		return strings.TrimPrefix(workspaceRel, prefix)
	}
	return workspaceRel
}

// Copied from py/generate.go -- keep in sync.
func perFileRuleName(srcName string) string {
	base := filepath.Base(srcName)
	for _, ext := range []string{".py", ".pyi"} {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

// Copied from py/generate.go -- keep in sync.
func isInitFile(name string) bool {
	return filepath.Base(name) == "__init__.py"
}

// Copied from py/generate.go -- keep in sync.
func isEmptyPython(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		// On read error, fall back to "not empty" so we don't accidentally
		// drop a rule the user actually wants.
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return false
	}
	return true
}
