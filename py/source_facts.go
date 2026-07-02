package py

import "path/filepath"

// sourceFacts is the generation-facing view of parsed Python files in one
// Bazel package. It keeps parser details local and exposes facts by
// package-relative source path.
type sourceFacts struct {
	rel     string
	results map[string]FileImports
	relBy   map[string]string
}

func newSourceFacts(rel string, specs []FileSpec, results map[string]FileImports) *sourceFacts {
	relBy := make(map[string]string, len(specs))
	for _, s := range specs {
		relBy[pkgRelativePath(s.RelPath, rel)] = s.RelPath
	}
	return &sourceFacts{
		rel:     rel,
		results: results,
		relBy:   relBy,
	}
}

func (f *sourceFacts) importsFor(srcs []string) ([]ImportStatement, bool) {
	var imps []ImportStatement
	for _, src := range srcs {
		r, ok := f.resultFor(src)
		if !ok {
			return nil, false
		}
		imps = append(imps, r.Modules...)
	}
	return imps, true
}

func (f *sourceFacts) annotationsFor(srcs []string) annotations {
	annot := annotations{ignore: map[string]bool{}}
	seenDeps := map[string]bool{}
	for _, src := range srcs {
		r, ok := f.resultFor(src)
		if !ok {
			continue
		}
		for module := range r.Annotations.ignore {
			annot.ignore[module] = true
		}
		for _, dep := range r.Annotations.includeDep {
			if seenDeps[dep] {
				continue
			}
			seenDeps[dep] = true
			annot.includeDep = append(annot.includeDep, dep)
		}
	}
	return annot
}

func (f *sourceFacts) allEmptyInits(srcs []string) bool {
	if len(srcs) == 0 {
		return false
	}
	for _, src := range srcs {
		if !isInitFile(src) {
			return false
		}
		r, ok := f.resultFor(src)
		if !ok || !r.IsEmpty {
			return false
		}
	}
	return true
}

func (f *sourceFacts) resultFor(src string) (FileImports, bool) {
	if relPath, ok := f.relBy[filepath.ToSlash(src)]; ok {
		r, ok := f.results[relPath]
		return r, ok
	}
	r, ok := f.results[filepath.Join(f.rel, src)]
	return r, ok
}
