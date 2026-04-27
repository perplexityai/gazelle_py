package py

import (
	"bufio"
	"os"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/repo"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// pythonStdlib: top-level modules shipped with CPython. Imports of these (or
// their submodules) get no Bazel dep — the interpreter provides them.
//
// Sourced from `sys.stdlib_module_names` (Python 3.12). Trimmed to the most
// common ones; consumers can extend via `# gazelle:resolve py <mod> <label>`
// to route stdlib-shadowing modules elsewhere.
var pythonStdlib = map[string]bool{
	"__future__": true, "_thread": true, "abc": true, "argparse": true,
	"array": true, "ast": true, "asyncio": true, "atexit": true,
	"base64": true, "binascii": true, "bisect": true, "builtins": true,
	"bz2": true, "calendar": true, "cgi": true, "cmath": true, "cmd": true,
	"code": true, "codecs": true, "collections": true, "colorsys": true,
	"compileall": true, "concurrent": true, "configparser": true,
	"contextlib": true, "contextvars": true, "copy": true, "copyreg": true,
	"csv": true, "ctypes": true, "curses": true, "dataclasses": true,
	"datetime": true, "dbm": true, "decimal": true, "difflib": true,
	"dis": true, "doctest": true, "email": true, "encodings": true,
	"enum": true, "errno": true, "faulthandler": true, "fcntl": true,
	"filecmp": true, "fileinput": true, "fnmatch": true, "fractions": true,
	"ftplib": true, "functools": true, "gc": true, "getopt": true,
	"getpass": true, "gettext": true, "glob": true, "graphlib": true,
	"grp": true, "gzip": true, "hashlib": true, "heapq": true, "hmac": true,
	"html": true, "http": true, "imaplib": true, "importlib": true,
	"inspect": true, "io": true, "ipaddress": true, "itertools": true,
	"json": true, "keyword": true, "linecache": true, "locale": true,
	"logging": true, "lzma": true, "mailbox": true, "marshal": true,
	"math": true, "mimetypes": true, "mmap": true, "multiprocessing": true,
	"netrc": true, "numbers": true, "operator": true, "optparse": true,
	"os": true, "pathlib": true, "pdb": true, "pickle": true,
	"pickletools": true, "pkgutil": true, "platform": true, "plistlib": true,
	"poplib": true, "posix": true, "posixpath": true, "pprint": true,
	"profile": true, "pstats": true, "pty": true, "pwd": true, "py_compile": true,
	"pydoc": true, "queue": true, "quopri": true, "random": true, "re": true,
	"readline": true, "reprlib": true, "resource": true, "runpy": true,
	"sched": true, "secrets": true, "select": true, "selectors": true,
	"shelve": true, "shlex": true, "shutil": true, "signal": true,
	"site": true, "smtplib": true, "sndhdr": true, "socket": true,
	"socketserver": true, "sqlite3": true, "ssl": true, "stat": true,
	"statistics": true, "string": true, "stringprep": true, "struct": true,
	"subprocess": true, "sunau": true, "symtable": true, "sys": true,
	"sysconfig": true, "syslog": true, "tabnanny": true, "tarfile": true,
	"telnetlib": true, "tempfile": true, "termios": true, "textwrap": true,
	"threading": true, "time": true, "timeit": true, "tkinter": true,
	"token": true, "tokenize": true, "tomllib": true, "trace": true,
	"traceback": true, "tracemalloc": true, "tty": true, "turtle": true,
	"types": true, "typing": true, "unicodedata": true, "unittest": true,
	"urllib": true, "uu": true, "uuid": true, "venv": true, "warnings": true,
	"wave": true, "weakref": true, "webbrowser": true, "winreg": true,
	"winsound": true, "wsgiref": true, "xml": true, "xmlrpc": true,
	"zipapp": true, "zipfile": true, "zipimport": true, "zlib": true,
	"zoneinfo": true,
}

// resolvedDeps holds the two categories we attach to a rule.
type resolvedDeps struct {
	internal []string // intra-repo labels
	external []string // pip labels
}

// Resolve converts ImportData (attached during GenerateRules) into Bazel
// labels and writes them onto the rule's `deps` attr.
func (l *pyLang) Resolve(
	c *config.Config,
	ix *resolve.RuleIndex,
	rc *repo.RemoteCache,
	r *rule.Rule,
	rawImportData interface{},
	from label.Label,
) {
	cfg, _ := c.Exts[languageName].(*pyConfig)
	if cfg == nil {
		cfg = newPyConfig()
	}
	importData, ok := rawImportData.(ImportData)
	if !ok {
		return
	}

	switch r.Kind() {
	case cfg.libraryKind:
		resolved := l.resolveImportsToDeps(c, importData.Imports, from, ix, cfg)
		all := append([]string{}, resolved.external...)
		all = append(all, resolved.internal...)
		setOrDelete(r, "deps", all)

	case cfg.testKind:
		// Test rules absorb the test imports plus the surrounding library's
		// imports (the test typically links everything its sibling lib does
		// plus its own deps). The library itself, when present, becomes a
		// dep too — added by the user via map_kind/macro or post-edit; the
		// scaffold leaves that to the consumer to keep behavior conservative.
		testResolved := l.resolveImportsToDeps(c, importData.TestImports, from, ix, cfg)
		srcResolved := l.resolveImportsToDeps(c, importData.Imports, from, ix, cfg)
		all := append([]string{}, testResolved.external...)
		all = append(all, testResolved.internal...)
		all = append(all, srcResolved.external...)
		all = append(all, srcResolved.internal...)
		setOrDelete(r, "deps", all)
	}
}

func setOrDelete(r *rule.Rule, attr string, values []string) {
	values = deduplicateAndSort(values)
	if len(values) > 0 {
		r.SetAttr(attr, values)
	} else {
		r.DelAttr(attr)
	}
}

// resolveImportsToDeps categorizes each import into internal vs external.
func (l *pyLang) resolveImportsToDeps(
	c *config.Config,
	imports []ImportStatement,
	from label.Label,
	ix *resolve.RuleIndex,
	cfg *pyConfig,
) resolvedDeps {
	result := resolvedDeps{}
	seen := make(map[string]bool)

	for _, imp := range imports {
		if seen[imp.ImportPath] {
			continue
		}
		seen[imp.ImportPath] = true

		path := imp.ImportPath

		// Relative imports stay within the package; nothing to add. The
		// extractor encodes `from . import x` / `from .foo import x` with a
		// leading "." prefix.
		if strings.HasPrefix(path, ".") {
			continue
		}

		// Gazelle's `# gazelle:resolve py <import> <label>` directive wins
		// over every other resolution path.
		spec := resolve.ImportSpec{Lang: languageName, Imp: path}
		if dep, ok := resolve.FindRuleWithOverride(c, spec, languageName); ok {
			result.external = append(result.external, dep.Rel(from.Repo, from.Pkg).String())
			continue
		}

		// Stdlib imports — no Bazel dep needed.
		topLevel := strings.SplitN(path, ".", 2)[0]
		if pythonStdlib[topLevel] {
			continue
		}

		// Internal package — walk the rule index from longest path to
		// shortest. `myorg.api.client` will match `myorg.api` then `myorg`.
		if internalLabel := l.lookupInternal(path, from, ix); internalLabel != "" {
			result.internal = append(result.internal, internalLabel)
			continue
		}

		// PyPI package. We map by top-level distribution name; rules_python's
		// pip_parse layout makes `@pip//<dist>` resolvable as long as the dep
		// is in the lockfile. If the user declared deps in pyproject.toml /
		// requirements.txt we gate emission on packageDeps; otherwise we
		// emit the label optimistically.
		distName := normalizeDist(topLevel)
		if len(l.packageDeps) == 0 || l.packageDeps[distName] {
			result.external = append(result.external, pipLabel(cfg, distName))
		}
	}

	result.internal = deduplicateAndSort(result.internal)
	result.external = deduplicateAndSort(result.external)
	return result
}

// lookupInternal walks the RuleIndex from the longest matching dotted prefix
// down, returning the first hit. Empty string means no match.
func (l *pyLang) lookupInternal(importPath string, from label.Label, ix *resolve.RuleIndex) string {
	parts := strings.Split(importPath, ".")
	for i := len(parts); i > 0; i-- {
		test := strings.Join(parts[:i], ".")
		if found := ix.FindRulesByImportWithConfig(nil, resolve.ImportSpec{Lang: languageName, Imp: test}, languageName); len(found) > 0 {
			return found[0].Label.Rel(from.Repo, from.Pkg).String()
		}
		if found := ix.FindRulesByImportWithConfig(nil, resolve.ImportSpec{Lang: languageName, Imp: test + ".*"}, languageName); len(found) > 0 {
			return found[0].Label.Rel(from.Repo, from.Pkg).String()
		}
	}
	return ""
}

// normalizeDist converts a top-level Python module name to its conventional
// PyPI distribution name. PyPI normalization: lowercase, hyphens and dots
// become underscores for the lookup but the published label uses underscores
// (rules_python's pip_parse strips them). We keep underscores here; the
// mapping covers the simple identity case (most pure-Python packages).
//
// Many distributions don't match their import name (e.g. `cv2` ⇄ `opencv-python`,
// `PIL` ⇄ `Pillow`); for those, callers should add a `# gazelle:resolve py
// <import> <label>` override or extend pythonImportToDist below.
var pythonImportToDist = map[string]string{
	"cv2":      "opencv_python",
	"PIL":      "pillow",
	"sklearn":  "scikit_learn",
	"yaml":     "pyyaml",
	"bs4":      "beautifulsoup4",
	"OpenSSL":  "pyopenssl",
	"dateutil": "python_dateutil",
}

func normalizeDist(modName string) string {
	if d, ok := pythonImportToDist[modName]; ok {
		return d
	}
	return strings.ToLower(modName)
}

// pipLabel renders the pip-package label using the configured pattern.
func pipLabel(cfg *pyConfig, distName string) string {
	return strings.ReplaceAll(cfg.pipLinkPattern, "{pkg}", distName)
}

// loadProjectDeps reads pyproject.toml and/or requirements.txt at the repo
// root and seeds packageDeps with declared distribution names. Best-effort —
// neither file is required, and the parser is intentionally simple.
func (l *pyLang) loadProjectDeps(repoRoot string) {
	if len(l.packageDeps) > 0 {
		return
	}

	// pyproject.toml: scan [project] dependencies = [...] block.
	if data, err := os.ReadFile(repoRoot + "/pyproject.toml"); err == nil {
		for _, name := range scanPyProjectDeps(string(data)) {
			l.packageDeps[name] = true
		}
	}

	// requirements.txt / requirements.in: one dep per line; strip extras and
	// version specifiers.
	for _, name := range []string{"requirements.txt", "requirements.in"} {
		f, err := os.Open(repoRoot + "/" + name)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if dep := parseRequirementLine(scanner.Text()); dep != "" {
				l.packageDeps[dep] = true
			}
		}
		f.Close()
	}
}

// scanPyProjectDeps does a regex-free best-effort scan of pyproject.toml's
// `[project] dependencies = [...]` array. Doesn't pretend to be a TOML
// parser — just grabs the obvious literal-string entries.
func scanPyProjectDeps(content string) []string {
	var out []string
	idx := strings.Index(content, "[project]")
	if idx < 0 {
		return nil
	}
	tail := content[idx:]
	depIdx := strings.Index(tail, "dependencies")
	if depIdx < 0 {
		return nil
	}
	tail = tail[depIdx:]
	open := strings.Index(tail, "[")
	close := strings.Index(tail, "]")
	if open < 0 || close < 0 || close < open {
		return nil
	}
	body := tail[open+1 : close]
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ",")
		if len(line) >= 2 && (line[0] == '"' || line[0] == '\'') {
			line = line[1 : len(line)-1]
		}
		if name := parseRequirementLine(line); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// parseRequirementLine extracts the distribution name from one line of a
// requirements.txt / pyproject.toml deps array entry. Strips comments,
// extras (`pkg[extra]`), version specifiers (`pkg==1.0`), and environment
// markers (`pkg ; python_version<'3.10'`). Returns "" for blank/comment lines.
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

// deduplicateAndSort returns a sorted unique copy of items.
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
