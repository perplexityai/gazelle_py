package py

// ImportStatement is a single import found in a Python file. We track the
// source file so error messages can point at the exact file that introduced a
// particular dependency.
type ImportStatement struct {
	ImportPath string // module specifier (e.g., "os", "myorg.api.client")
	SourceFile string // file containing the import (e.g., "packages/foo/src/main.py")
}

// extractImportsBatch sends a batch of file paths through the cgo FFI and
// returns the parsed imports keyed by file path. Per-call batching keeps
// Rust's rayon parallelism alive across all files in the batch.
func (l *pyLang) extractImportsBatch(filePaths []string) (map[string][]ImportStatement, error) {
	result, err := extractImports(filePaths)
	if err != nil {
		return nil, err
	}

	imports := make(map[string][]ImportStatement, len(result))
	for file, paths := range result {
		stmts := make([]ImportStatement, 0, len(paths))
		for _, p := range paths {
			stmts = append(stmts, ImportStatement{
				ImportPath: p,
				SourceFile: file,
			})
		}
		imports[file] = stmts
	}
	return imports, nil
}
