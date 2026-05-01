package py

// ImportStatement is a single import found in a Python file. Line and From let
// us produce useful diagnostics ("file X line N imports Y") and feed the
// possible-modules loop in resolve.go (which walks both the full name and the
// `from` part).
type ImportStatement struct {
	ImportPath       string // full dotted module path (e.g. "os.path.join")
	From             string // the "from" part of `from a.b import c` (else empty)
	SourceFile       string // workspace-relative file containing the import
	LineNumber       uint32 // 1-indexed line number for diagnostics
	TypeCheckingOnly bool   // true if inside `if TYPE_CHECKING:` block
}

// FileImports holds everything the parser extracted for a single file. Comments
// drive `# gazelle:ignore` / `# gazelle:include_dep` annotation parsing in the
// generator; HasMain is captured for completeness even though we don't currently
// use it to emit `py_binary` rules. IsEmpty is true when the AST has no
// top-level statements (whitespace and/or comments only) and feeds
// `python_skip_empty_init` rule suppression.
type FileImports struct {
	FileName string // workspace-relative path (e.g. "pkg/foo.py")
	Modules  []ImportStatement
	Comments []string
	HasMain  bool
	IsEmpty  bool
}

// extractImportsBatch sends a batch of (abs, rel) file specs through the cgo
// FFI. The returned map is keyed by the workspace-relative path so callers
// can look up results without juggling absolute paths.
func (l *pyLang) extractImportsBatch(specs []FileSpec) (map[string]FileImports, error) {
	results, err := extractImports(specs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]FileImports, len(results))
	for _, r := range results {
		out[r.FileName] = r
	}
	return out, nil
}

// FileSpec mirrors the proto PyFileSpec on the Go side.
type FileSpec struct {
	Path    string // absolute (or runfiles-resolvable) path; the parser reads bytes here
	RelPath string // workspace-relative path stamped into the output's FileName
}
