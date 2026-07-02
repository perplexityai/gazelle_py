package py

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

func BenchmarkPackageSourceOwnershipBroadPatterns(b *testing.B) {
	cfg := newPyConfig()
	specs := make([]FileSpec, 0, 1000)
	for i := 0; i < 900; i++ {
		specs = append(specs, FileSpec{RelPath: fmt.Sprintf("pkg/src/mod_%04d.py", i)})
	}
	for i := 0; i < 100; i++ {
		specs = append(specs, FileSpec{RelPath: fmt.Sprintf("pkg/tests/test_mod_%04d.py", i)})
	}

	file := rule.EmptyFile("pkg/BUILD.bazel", "pkg")
	for i := 0; i < 100; i++ {
		r := rule.NewRule(defaultLibraryKind, fmt.Sprintf("lib_%03d", i))
		r.SetAttr("file_patterns", []string{"**/*.py"})
		r.SetAttr("ignore_patterns", []string{"tests/**/*.py"})
		file.Rules = append(file.Rules, r)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ownership := newSpecPackageSourceOwnership(cfg, nil, "pkg", specs, file, nil)
		_ = ownership.handOwnedSources()
	}
}

func BenchmarkImportsDiskFilePatterns(b *testing.B) {
	cfg := newPyConfig()
	root := b.TempDir()
	pkgDir := filepath.Join(root, "pkg")
	for i := 0; i < 900; i++ {
		mustWriteBenchmarkFile(b, filepath.Join(pkgDir, "src", fmt.Sprintf("mod_%04d.py", i)))
	}
	for i := 0; i < 100; i++ {
		mustWriteBenchmarkFile(b, filepath.Join(pkgDir, "tests", fmt.Sprintf("test_mod_%04d.py", i)))
	}

	l := &pyLang{packageDepsByRoot: map[string]map[string]bool{}}
	c := &config.Config{RepoRoot: root, Exts: map[string]interface{}{languageName: cfg}}
	file := rule.EmptyFile("pkg/BUILD.bazel", "pkg")
	for i := 0; i < 20; i++ {
		r := rule.NewRule(defaultLibraryKind, fmt.Sprintf("lib_%03d", i))
		r.SetAttr("file_patterns", []string{"**/*.py"})
		r.SetAttr("ignore_patterns", []string{"tests/**/*.py"})
		file.Rules = append(file.Rules, r)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, r := range file.Rules {
			_ = l.Imports(c, r, file)
		}
	}
}

func BenchmarkResolveImportsRepeated(b *testing.B) {
	cfg := newPyConfig()
	l := &pyLang{packageDepsByRoot: map[string]map[string]bool{}}
	c := &config.Config{RepoRoot: b.TempDir(), Exts: map[string]interface{}{languageName: cfg}}
	(&resolve.Configurer{}).RegisterFlags(flag.NewFlagSet("bench", flag.ContinueOnError), "", c)
	ix := resolve.NewRuleIndex(nil)
	ix.Finish()
	from := label.New("", "pkg", "pkg")

	imports := make([]ImportStatement, 0, 3000)
	for i := 0; i < 1000; i++ {
		imports = append(imports,
			ImportStatement{ImportPath: "pydantic.BaseModel", From: "pydantic"},
			ImportStatement{ImportPath: "pplx.common.core.deep.module.Symbol", From: "pplx.common.core.deep.module"},
			ImportStatement{ImportPath: "typing.Optional", From: "typing"},
		)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = l.resolveImports(c, ix, ImportData{}, imports, from, cfg, nil)
	}
}

func mustWriteBenchmarkFile(b *testing.B, path string) {
	b.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("import typing\n"), 0o644); err != nil {
		b.Fatal(err)
	}
}
