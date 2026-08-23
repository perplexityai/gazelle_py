package py

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
	bzl "github.com/bazelbuild/buildtools/build"
	"github.com/bmatcuk/doublestar/v4"
)

// conftestFilename is the file name pytest collects as a fixture/setup
// module; we mirror its discovery rules. conftestTargetName is the rule
// name we emit for the dedicated `py_library` that wraps it (matching
// rules_python's gazelle plugin).
const (
	conftestFilename     = "conftest.py"
	conftestTargetName   = "conftest"
	binaryEntrypointFile = "__main__.py"
)

// ImportData carries parsed imports + annotations from GenerateRules to
// Resolve. Gazelle runs GenerateRules during the directory walk (before the
// RuleIndex is complete) and Resolve afterwards, so we stash everything we'll
// need later. The two import slices are kept apart because Resolve attaches
// them to different rules (library vs test).
type ImportData struct {
	Imports      []ImportStatement // source-file imports
	TestImports  []ImportStatement // test-file imports
	Ignore       map[string]bool   // module names to skip during resolution
	IncludeDeps  []string          // labels to always add to deps
	PreserveDeps bool              // keep existing deps when source or dependency analysis is incomplete
	ExistingDeps []string          // deps already present on the merged target
	config       *pyConfig         // package config snapshot captured before Gazelle's resolve phase
}

type pythonLibraryOwner struct {
	name    string
	sources map[string]bool
}

type preservedExpr struct {
	expr bzl.Expr
}

var _ rule.BzlExprValue = preservedExpr{}
var _ rule.Merger = preservedExpr{}

func (p preservedExpr) BzlExpr() bzl.Expr {
	return p.expr
}

// Merge keeps opaque hand-written expressions out of Gazelle's list merger.
func (p preservedExpr) Merge(other bzl.Expr) bzl.Expr {
	if other != nil {
		return other
	}
	return p.expr
}

// GenerateRules walks a directory's files, partitions them into source vs.
// test, parses imports via the cgo-bound import_extractor, and emits library,
// binary, and test rules. The merge engine reconciles them with existing BUILD
// content using KindInfo from kinds.go.
//
// Generation shape is selected by `python_generation_mode`:
//   - package (default): one library + one test rule per directory.
//   - file:              one library/test rule per .py file.
//   - project:           at the directory the directive was set on, roll up
//     every .py file under the subtree into one library/test rule. In
//     subdirectories within that project root, generate nothing.
//
// Every active mode also emits binaries for detected Python entrypoints.
func (l *pyLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	cfg, ok := args.Config.Exts[languageName].(*pyConfig)
	if !ok || !cfg.enabled {
		return language.GenerateResult{}
	}
	if cfg.generationMode == generationModeOff {
		return language.GenerateResult{}
	}

	// Project mode: only the directory that introduced the directive emits
	// rules. Children inside that subtree return empty.
	if cfg.generationMode == generationModeProject && args.Rel != cfg.projectRoot {
		return language.GenerateResult{}
	}

	var specs []FileSpec
	if cfg.generationMode == generationModeProject {
		// Walk the entire project subtree (including the directive's own
		// directory) so the rolled-up rule sees every .py file. Subdirs that
		// already have BUILD files mark Bazel-package boundaries and are
		// skipped — Bazel refuses to glob across them anyway.
		walkSpecs, err := walkProjectFiles(cfg, args.Dir, args.Rel)
		if err == nil {
			specs = walkSpecs
		}
	} else {
		specs = pythonFileSpecs(cfg, args.Dir, args.Rel, args.RegularFiles)
	}

	results := l.parseSpecs(specs)

	switch cfg.generationMode {
	case generationModeFile:
		return generatePerFileRules(cfg, args.Config, args.Rel, specs, results, args.File)
	case generationModeProject:
		return generateAggregateRules(cfg, args.Config, args.Rel, specs, results, args.File, false)
	default:
		return generateAggregateRules(cfg, args.Config, args.Rel, specs, results, args.File, true)
	}
}

// pythonFileSpecs maps the per-directory regular files to FileSpecs the
// parser expects. RelPath is filepath.Join(rel, name); the parser's
// returned `FileName` matches that, letting Resolve match imports back to
// files without juggling absolute paths.
func pythonFileSpecs(cfg *pyConfig, dir, rel string, files []string) []FileSpec {
	var specs []FileSpec
	for _, f := range files {
		if !isPythonFile(f, cfg) {
			continue
		}
		specs = append(specs, FileSpec{
			Path:    filepath.Join(dir, f),
			RelPath: filepath.Join(rel, f),
		})
	}
	return specs
}

// walkProjectFiles enumerates every Python source under `rootDir` (excluding
// the directory itself, which the caller has already handled) and returns
// FileSpecs with RelPath rooted at the project's package directory `rootRel`.
// Used by `python_generation_mode project` to roll up subtree sources.
func walkProjectFiles(cfg *pyConfig, rootDir, rootRel string) ([]FileSpec, error) {
	var specs []FileSpec
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == rootDir {
			return nil
		}
		if d.IsDir() {
			// Don't descend into directories that have their own BUILD file
			// — Bazel would treat those as separate packages and the glob
			// wouldn't reach across the boundary.
			if hasBuildFile(path) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !isPythonFile(name, cfg) {
			return nil
		}
		relFromRoot, _ := filepath.Rel(rootDir, path)
		specs = append(specs, FileSpec{
			Path:    path,
			RelPath: filepath.Join(rootRel, relFromRoot),
		})
		return nil
	})
	return specs, err
}

func hasBuildFile(dir string) bool {
	for _, name := range []string{"BUILD.bazel", "BUILD"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// parseSpecs runs the import extractor over `specs` and returns the parsed
// results keyed by RelPath.
func (l *pyLang) parseSpecs(specs []FileSpec) map[string]FileImports {
	if len(specs) == 0 {
		return nil
	}
	results, err := l.extractImportsBatch(specs)
	if err != nil {
		// We don't fail the whole gazelle run on a parser error — we just
		// drop this directory's imports. The next run picks them up after
		// the user fixes whatever made the parser unhappy.
		return nil
	}
	return results
}

// generateAggregateRules emits one library + one test rule covering every
// spec passed in, plus binaries for executable modules. Used by both `package`
// and `project` generation modes — the caller decides which specs to gather.
//
// `conftest.py` at the package's own root (not nested under a subdirectory)
// is extracted into a dedicated `py_library` named `conftest` with
// `testonly=True`, mirroring rules_python's gazelle plugin. Tests pick it up
// transitively through the ancestor-conftest synthesis in resolve.go.
func generateAggregateRules(cfg *pyConfig, c *config.Config, rel string, specs []FileSpec, results map[string]FileImports, file *rule.File, manageHandRolled bool) language.GenerateResult {
	libName, testName := resolveRuleNames(cfg, rel)
	managed := map[string]bool{
		libName:            true,
		testName:           true,
		conftestTargetName: true,
	}
	facts := newSourceFacts(rel, specs, results)
	ownership := newSpecPackageSourceOwnership(cfg, c, rel, specs, file, managed)
	leaveLibUnmanaged := ownership.hasComputedSourceAttrs(libName, false)
	leaveTestUnmanaged := ownership.hasComputedSourceAttrs(testName, true)
	handOwned := ownership.handOwnedSources()
	binaryOwned := existingBinarySources(ownership, file)
	existingLibSources := existingSourceSet(ownership, libName, false)
	existingTestSources := existingSourceSet(ownership, testName, true)

	var libSrcs, testSrcs []string
	var conftestImports []ImportStatement
	hasConftest := false
	for _, s := range specs {
		// Sources are listed relative to the package the rule lives in.
		// `rel` is the package's workspace-relative path; trimming it
		// turns "apps/server/utils/x.py" into "utils/x.py" inside
		// //apps/server's BUILD file.
		srcName := pkgRelativePath(s.RelPath, rel)
		srcKey := filepath.ToSlash(srcName)
		if handOwned[srcKey] {
			continue
		}
		if isConftestAtPackageRoot(srcName) {
			hasConftest = true
			if r, ok := results[s.RelPath]; ok {
				conftestImports = append(conftestImports, r.Modules...)
			}
			continue
		}
		isTest := isTestFile(srcName, cfg)
		if binaryOwned[srcKey] {
			isTest = false
		}
		if existingTestSources[srcKey] {
			isTest = true
		} else if existingLibSources[srcKey] {
			isTest = false
		}
		if isTest {
			testSrcs = append(testSrcs, srcName)
		} else {
			libSrcs = append(libSrcs, srcName)
		}
	}
	sort.Strings(libSrcs)
	sort.Strings(testSrcs)
	if !leaveLibUnmanaged && !ownership.preservesSourceAttrs(libName, false) {
		libSrcs = refreshManagedSources(ownership, libName, false, libSrcs, facts, handOwned)
	}
	if !leaveTestUnmanaged && !ownership.preservesSourceAttrs(testName, true) {
		testSrcs = refreshManagedSources(ownership, testName, true, testSrcs, facts, handOwned)
	}

	skipLib := cfg.skipEmptyInit && facts.allEmptyInits(libSrcs)
	skipTest := cfg.skipEmptyInit && facts.allEmptyInits(testSrcs)

	var plans []rulePlan
	var packageLibrary *pythonLibraryOwner

	if len(libSrcs) > 0 && !skipLib && !leaveLibUnmanaged {
		r := rule.NewRule(cfg.libraryKind, libName)
		importSrcs := libSrcs
		if ownership.preservesSourceAttrs(libName, false) {
			if explicitSrcs, ok := ownership.existingExplicitRuleSources(libName, false); ok {
				importSrcs = explicitSrcs
				r.SetAttr("srcs", explicitSrcs)
			} else if srcs, ok := ownership.existingRuleSources(libName, false); ok {
				importSrcs = srcs
			}
		} else {
			r.SetAttr("srcs", libSrcs)
		}
		if len(cfg.visibility) > 0 {
			r.SetAttr("visibility", cfg.visibility)
		}
		data := withExistingDeps(importDataForSources(facts, importSrcs, false), ownership, libName, false)
		if data.PreserveDeps {
			preserveExistingDeps(r, ownership, libName, false)
		}
		plans = append(plans, rulePlan{rule: r, imports: data})
		if libraryTargetNameAvailable(ownership, libName) {
			packageLibrary = &pythonLibraryOwner{
				name:    libName,
				sources: sourceSet(importSrcs),
			}
		}
	}

	if hasConftest && !ownership.hasComputedSourceAttrs(conftestTargetName, false) {
		r := rule.NewRule(cfg.libraryKind, conftestTargetName)
		r.SetAttr("srcs", []string{conftestFilename})
		r.SetAttr("testonly", true)
		if len(cfg.visibility) > 0 {
			r.SetAttr("visibility", cfg.visibility)
		}
		annot := facts.annotationsFor([]string{conftestFilename})
		data := withExistingDeps(ImportData{
			Imports:     conftestImports,
			Ignore:      annot.ignore,
			IncludeDeps: annot.includeDep,
		}, ownership, conftestTargetName, false)
		if data.PreserveDeps {
			preserveExistingDeps(r, ownership, conftestTargetName, false)
		}
		plans = append(plans, rulePlan{
			rule:    r,
			imports: data,
		})
	}

	if len(testSrcs) > 0 && !skipTest && !leaveTestUnmanaged {
		r := rule.NewRule(cfg.testKind, testName)
		importSrcs := testSrcs
		if ownership.preservesSourceAttrs(testName, true) {
			if explicitSrcs, ok := ownership.existingExplicitRuleSources(testName, true); ok {
				importSrcs = explicitSrcs
				r.SetAttr("srcs", explicitSrcs)
			} else if srcs, ok := ownership.existingRuleSources(testName, true); ok {
				importSrcs = srcs
			}
		} else {
			r.SetAttr("srcs", testSrcs)
		}
		data := withExistingDeps(importDataForSources(facts, importSrcs, true), ownership, testName, true)
		if data.PreserveDeps {
			preserveExistingDeps(r, ownership, testName, true)
		}
		plans = append(plans, rulePlan{rule: r, imports: data})
	}

	if manageHandRolled {
		extraPlans := planHandRolledRules(cfg, c, rel, specs, facts, ownership, file, managed, packageLibrary)
		plans = append(plans, extraPlans...)
	} else {
		plans = append(plans, planHandRolledBinaryRules(cfg, c, rel, specs, facts, ownership, file, packageLibrary)...)
	}
	plans = append(plans, planDetectedBinaryRules(cfg, rel, specs, results, ownership, file, packageLibrary, sourceSet(libSrcs), binaryOwned, plans)...)

	return generateResultFromPlans(plans, cfg)
}

func planDetectedBinaryRules(
	cfg *pyConfig,
	rel string,
	specs []FileSpec,
	results map[string]FileImports,
	ownership *packageSourceOwnership,
	file *rule.File,
	packageLibrary *pythonLibraryOwner,
	eligible map[string]bool,
	owned map[string]bool,
	reserved []rulePlan,
) []rulePlan {
	occupied := map[string]bool{}
	if file != nil {
		for _, existing := range file.Rules {
			occupied[existing.Name()] = true
		}
	}
	for _, plan := range reserved {
		occupied[plan.rule.Name()] = true
	}

	sortedSpecs := append([]FileSpec(nil), specs...)
	sort.Slice(sortedSpecs, func(i, j int) bool {
		return sortedSpecs[i].RelPath < sortedSpecs[j].RelPath
	})
	hasPackageEntrypoint := false
	for _, spec := range sortedSpecs {
		if filepath.ToSlash(pkgRelativePath(spec.RelPath, rel)) == binaryEntrypointFile {
			hasPackageEntrypoint = true
			break
		}
	}

	var plans []rulePlan
	for _, spec := range sortedSpecs {
		source := filepath.ToSlash(pkgRelativePath(spec.RelPath, rel))
		if !eligible[source] || owned[source] || isTestFile(source, cfg) {
			continue
		}
		parsed, ok := results[spec.RelPath]
		if !ok {
			continue
		}

		name := perFileRuleName(source)
		if hasPackageEntrypoint {
			if source != binaryEntrypointFile {
				continue
			}
			name = resolveBinaryName(cfg, rel)
		} else if !parsed.HasMain {
			continue
		}
		if occupied[name] {
			continue
		}

		binary := rule.NewRule(defaultBinaryKind, name)
		binary.SetAttr("srcs", []string{source})
		binary.SetAttr("main", source)
		if len(cfg.visibility) > 0 {
			binary.SetAttr("visibility", cfg.visibility)
		}

		data := ImportData{
			Imports:     parsed.Modules,
			Ignore:      parsed.Annotations.ignore,
			IncludeDeps: parsed.Annotations.includeDep,
		}
		if library := binarySourceLibraryOwner(ownership, packageLibrary, name, []string{source}); library != nil {
			data = ImportData{IncludeDeps: []string{":" + library.name}}
		}
		plans = append(plans, rulePlan{rule: binary, imports: data})
		occupied[name] = true
	}
	return plans
}

func refreshManagedSources(ownership *packageSourceOwnership, name string, isTest bool, inferred []string, facts *sourceFacts, handOwned map[string]bool) []string {
	existing, ok := ownership.existingExplicitRuleSources(name, isTest)
	if !ok {
		return inferred
	}

	refreshed := sourceSet(inferred)
	for _, src := range existing {
		key := normalizeLocalSource(src)
		if key == "" {
			key = filepath.ToSlash(src)
		}
		if facts.contains(key) || handOwned[key] {
			continue
		}
		refreshed[key] = true
	}

	srcs := make([]string, 0, len(refreshed))
	for src := range refreshed {
		srcs = append(srcs, src)
	}
	sort.Strings(srcs)
	return srcs
}

func importDataForSources(facts *sourceFacts, srcs []string, isTest bool) ImportData {
	imps, ok := facts.importsFor(srcs)
	if !ok {
		return ImportData{PreserveDeps: true}
	}
	annot := facts.annotationsFor(srcs)
	data := ImportData{
		Ignore:      annot.ignore,
		IncludeDeps: annot.includeDep,
	}
	if isTest {
		data.TestImports = imps
	} else {
		data.Imports = imps
	}
	return data
}

func preserveExistingDeps(r *rule.Rule, ownership *packageSourceOwnership, name string, isTest bool) {
	existing := ownership.existingPythonRule(name, isTest)
	if existing == nil || existing.Attr("deps") == nil {
		return
	}
	r.SetAttr("deps", preservedExpr{expr: existing.Attr("deps")})
}

func withExistingDeps(data ImportData, ownership *packageSourceOwnership, name string, isTest bool) ImportData {
	if ownership.hasComputedDeps(name, isTest) {
		data.PreserveDeps = true
		return data
	}
	deps, ok := ownership.existingRuleDeps(name, isTest)
	if !ok {
		return data
	}
	data.ExistingDeps = deps
	return data
}

func existingSourceSet(ownership *packageSourceOwnership, name string, isTest bool) map[string]bool {
	srcs, ok := ownership.existingRuleSources(name, isTest)
	if !ok {
		r := ownership.existingPythonRule(name, isTest)
		if r == nil {
			return nil
		}
		srcs = ownership.knownLiteralPythonSources(r)
	}
	set := make(map[string]bool, len(srcs))
	for _, src := range srcs {
		set[filepath.ToSlash(src)] = true
	}
	return set
}

func sourceSet(srcs []string) map[string]bool {
	set := make(map[string]bool, len(srcs))
	for _, src := range srcs {
		set[filepath.ToSlash(src)] = true
	}
	return set
}

func libraryTargetNameAvailable(ownership *packageSourceOwnership, name string) bool {
	if ownership == nil || ownership.file == nil {
		return true
	}
	for _, r := range ownership.file.Rules {
		if r.Name() != name {
			continue
		}
		ok, isTest := ownership.isPythonRule(r)
		return ok && !isTest
	}
	return true
}

func handOwnedPythonSources(cfg *pyConfig, c *config.Config, rel string, specs []FileSpec, file *rule.File, managed map[string]bool) map[string]bool {
	return newSpecPackageSourceOwnership(cfg, c, rel, specs, file, managed).handOwnedSources()
}

func generateHandRolledRules(cfg *pyConfig, c *config.Config, rel string, specs []FileSpec, facts *sourceFacts, ownership *packageSourceOwnership, file *rule.File, managed map[string]bool) ([]*rule.Rule, []interface{}) {
	return splitRulePlans(planHandRolledRules(cfg, c, rel, specs, facts, ownership, file, managed, nil), cfg)
}

func planHandRolledRules(cfg *pyConfig, c *config.Config, rel string, specs []FileSpec, facts *sourceFacts, ownership *packageSourceOwnership, file *rule.File, managed map[string]bool, packageLibrary *pythonLibraryOwner) []rulePlan {
	return planExistingPythonRules(cfg, c, rel, specs, facts, ownership, file, managed, true, packageLibrary)
}

func planHandRolledBinaryRules(cfg *pyConfig, c *config.Config, rel string, specs []FileSpec, facts *sourceFacts, ownership *packageSourceOwnership, file *rule.File, packageLibrary *pythonLibraryOwner) []rulePlan {
	return planExistingPythonRules(cfg, c, rel, specs, facts, ownership, file, nil, false, packageLibrary)
}

func planExistingPythonRules(cfg *pyConfig, c *config.Config, rel string, specs []FileSpec, facts *sourceFacts, ownership *packageSourceOwnership, file *rule.File, managed map[string]bool, includeLibrariesAndTests bool, packageLibrary *pythonLibraryOwner) []rulePlan {
	if file == nil {
		return nil
	}
	if facts == nil {
		facts = newSourceFacts(rel, specs, nil)
	}
	if ownership == nil {
		ownership = newSpecPackageSourceOwnership(cfg, c, rel, specs, file, managed)
	}

	var plans []rulePlan
	for _, er := range file.Rules {
		if managed != nil && managed[er.Name()] {
			continue
		}
		isBinary := ownership.isPythonBinaryRule(er)
		okRule, isTest := ownership.isPythonRule(er)
		if !isBinary && (!includeLibrariesAndTests || !okRule) {
			continue
		}
		existingDeps, depsAreLiteral := literalStringListAttr(er, "deps")
		if !depsAreLiteral {
			continue
		}
		srcs, ok := ownership.sourcesForRule(er)
		if !ok {
			continue
		}
		imps, ok := facts.importsFor(srcs)
		if !ok {
			continue
		}
		annot := facts.annotationsFor(srcs)
		kind := cfg.libraryKind
		data := ImportData{Imports: imps, Ignore: annot.ignore, IncludeDeps: annot.includeDep}
		if isBinary {
			kind = defaultBinaryKind
			if library := binarySourceLibraryOwner(ownership, packageLibrary, er.Name(), srcs); library != nil {
				data = ImportData{IncludeDeps: []string{":" + library.name}}
			}
		} else if isTest {
			kind = cfg.testKind
			data = ImportData{TestImports: imps, Ignore: annot.ignore, IncludeDeps: annot.includeDep}
		}
		if er.Attr("deps") != nil {
			data.ExistingDeps = existingDeps
		}
		r := rule.NewRule(kind, er.Name())
		if !isBinary && er.Attr("srcs") != nil {
			r.SetAttr("srcs", er.Attr("srcs"))
		}
		plans = append(plans, rulePlan{rule: r, imports: data})
	}
	return plans
}

func binarySourceLibraryOwner(ownership *packageSourceOwnership, generated *pythonLibraryOwner, binaryName string, srcs []string) *pythonLibraryOwner {
	if len(srcs) == 0 {
		return nil
	}

	owners := map[string]*pythonLibraryOwner{}
	if libraryOwnsSources(generated, binaryName, srcs) {
		owners[generated.name] = generated
	}
	if ownership != nil && ownership.file != nil {
		for _, r := range ownership.file.Rules {
			ok, isTest := ownership.isPythonRule(r)
			if !ok || isTest || r.Name() == binaryName {
				continue
			}
			ownedSources, ok := ownership.sourcesForRule(r)
			if !ok {
				continue
			}
			owner := &pythonLibraryOwner{name: r.Name(), sources: sourceSet(ownedSources)}
			if libraryOwnsSources(owner, binaryName, srcs) {
				owners[owner.name] = owner
			}
		}
	}
	if len(owners) != 1 {
		return nil
	}
	for _, owner := range owners {
		return owner
	}
	return nil
}

func libraryOwnsSources(owner *pythonLibraryOwner, binaryName string, srcs []string) bool {
	if owner == nil || owner.name == binaryName {
		return false
	}
	for _, src := range srcs {
		if !owner.sources[filepath.ToSlash(src)] {
			return false
		}
	}
	return true
}

func mappedKinds(c *config.Config, kind string) map[string]bool {
	kinds := map[string]bool{kind: true}
	if c == nil {
		return kinds
	}
	seen := map[string]bool{kind: true}
	for {
		mk, ok := c.KindMap[kind]
		if !ok {
			break
		}
		kinds[mk.KindName] = true
		if seen[mk.KindName] || kind == mk.KindName {
			break
		}
		seen[mk.KindName] = true
		kind = mk.KindName
	}
	return kinds
}

func importsForSrcs(rel string, srcs []string, results map[string]FileImports) ([]ImportStatement, bool) {
	return newSourceFacts(rel, nil, results).importsFor(srcs)
}

func annotationsForSrcs(rel string, srcs []string, results map[string]FileImports) annotations {
	return newSourceFacts(rel, nil, results).annotationsFor(srcs)
}

// generatePerFileRules emits one rule per source file: a library rule named
// after the file (e.g. `helpers.py` → `helpers`) for non-test files, and the
// configured test kind for test files. Selected by `python_generation_mode file`.
func generatePerFileRules(cfg *pyConfig, c *config.Config, rel string, specs []FileSpec, results map[string]FileImports, file *rule.File) language.GenerateResult {
	facts := newSourceFacts(rel, specs, results)
	ownership := newSpecPackageSourceOwnership(cfg, c, rel, specs, file, nil)
	binaryOwned := existingBinarySources(ownership, file)
	eligible := map[string]bool{}
	for _, spec := range specs {
		eligible[filepath.ToSlash(pkgRelativePath(spec.RelPath, rel))] = true
	}
	detectedBinaries := planDetectedBinaryRules(cfg, rel, specs, results, ownership, file, nil, eligible, binaryOwned, nil)
	for _, binary := range detectedBinaries {
		binaryOwned[binary.rule.AttrString("main")] = true
	}
	// Sort by the in-package relative path so emitted rules are stable.
	sortedSpecs := append([]FileSpec(nil), specs...)
	sort.Slice(sortedSpecs, func(i, j int) bool {
		return sortedSpecs[i].RelPath < sortedSpecs[j].RelPath
	})

	var (
		plans    []rulePlan
		libSpecs []FileSpec
	)
	for _, s := range sortedSpecs {
		srcName := pkgRelativePath(s.RelPath, rel)
		if binaryOwned[filepath.ToSlash(srcName)] {
			continue
		}
		if isTestFile(srcName, cfg) {
			continue
		}
		libSpecs = append(libSpecs, s)
	}

	for _, s := range libSpecs {
		srcName := pkgRelativePath(s.RelPath, rel)
		if cfg.skipEmptyInit && isInitFile(srcName) {
			if r, ok := results[s.RelPath]; ok && r.IsEmpty {
				continue
			}
		}
		ruleName := perFileRuleName(srcName)
		if ownership.hasComputedSourceAttrs(ruleName, false) {
			continue
		}
		r := rule.NewRule(cfg.libraryKind, ruleName)
		r.SetAttr("srcs", []string{srcName})
		if isConftestAtPackageRoot(srcName) {
			r.SetAttr("testonly", true)
		}
		if len(cfg.visibility) > 0 {
			r.SetAttr("visibility", cfg.visibility)
		}
		var imports []ImportStatement
		if pr, ok := facts.resultFor(srcName); ok {
			imports = pr.Modules
		}
		annot := facts.annotationsFor([]string{srcName})
		data := withExistingDeps(ImportData{
			Imports:     imports,
			Ignore:      annot.ignore,
			IncludeDeps: annot.includeDep,
		}, ownership, ruleName, false)
		if data.PreserveDeps {
			preserveExistingDeps(r, ownership, ruleName, false)
		}
		plans = append(plans, rulePlan{
			rule:    r,
			imports: data,
		})
	}

	for _, s := range sortedSpecs {
		srcName := pkgRelativePath(s.RelPath, rel)
		if binaryOwned[filepath.ToSlash(srcName)] {
			continue
		}
		if !isTestFile(srcName, cfg) {
			continue
		}
		if cfg.skipEmptyInit && isInitFile(srcName) {
			if r, ok := results[s.RelPath]; ok && r.IsEmpty {
				continue
			}
		}
		ruleName := perFileRuleName(srcName)
		// rules_python's file mode uses the library naming convention for
		// tests too (since each file is its own unit). Suffix with _test
		// when the bare basename collides with a sibling library rule.
		if !strings.HasSuffix(ruleName, "_test") {
			ruleName += "_test"
		}
		if ownership.hasComputedSourceAttrs(ruleName, true) {
			continue
		}
		r := rule.NewRule(cfg.testKind, ruleName)
		r.SetAttr("srcs", []string{srcName})
		var testMods []ImportStatement
		if pr, ok := facts.resultFor(srcName); ok {
			testMods = pr.Modules
		}
		annot := facts.annotationsFor([]string{srcName})
		data := withExistingDeps(ImportData{
			TestImports: testMods,
			Ignore:      annot.ignore,
			IncludeDeps: annot.includeDep,
		}, ownership, ruleName, true)
		if data.PreserveDeps {
			preserveExistingDeps(r, ownership, ruleName, true)
		}
		plans = append(plans, rulePlan{
			rule:    r,
			imports: data,
		})
	}

	plans = append(plans, planHandRolledBinaryRules(cfg, c, rel, specs, facts, ownership, file, nil)...)
	plans = append(plans, detectedBinaries...)

	return generateResultFromPlans(plans, cfg)
}

func existingBinarySources(ownership *packageSourceOwnership, file *rule.File) map[string]bool {
	owned := map[string]bool{}
	if file == nil {
		return owned
	}
	for _, r := range file.Rules {
		if !ownership.isPythonBinaryRule(r) {
			continue
		}
		sources, ok := ownership.sourcesForRule(r)
		if !ok {
			for _, source := range ownership.knownLiteralPythonSources(r) {
				owned[source] = true
			}
			if main, literal := literalStringAttr(r, "main"); literal && r.Attr("main") != nil {
				main = normalizeLocalSource(main)
				if ownership.expander.contains(main) {
					owned[main] = true
				}
			}
			defaultMain := r.Name() + ".py"
			if ownership.expander.contains(defaultMain) {
				owned[defaultMain] = true
			}
			continue
		}
		for _, source := range sources {
			source = normalizeLocalSource(source)
			if source != "" && ownership.expander.contains(source) {
				owned[source] = true
			}
		}
	}
	return owned
}

// pkgRelativePath drops the package prefix from a workspace-relative path.
// "apps/server/utils/x.py" within package "apps/server" → "utils/x.py".
func pkgRelativePath(workspaceRel, pkg string) string {
	if pkg == "" {
		return workspaceRel
	}
	if workspaceRel == pkg {
		return filepath.Base(workspaceRel)
	}
	prefix := pkg + string(filepath.Separator)
	if strings.HasPrefix(workspaceRel, prefix) {
		return strings.TrimPrefix(workspaceRel, prefix)
	}
	return workspaceRel
}

func perFileRuleName(srcName string) string {
	base := filepath.Base(srcName)
	for _, ext := range []string{".py", ".pyi"} {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

func isInitFile(name string) bool {
	return filepath.Base(name) == "__init__.py"
}

// isConftestAtPackageRoot returns true when the package-relative source
// path is exactly `conftest.py` (i.e. lives at the package directory itself,
// not in a sub-tree). Conftest files at deeper levels belong to that
// sub-package's own BUILD file when gazelle later processes that directory.
func isConftestAtPackageRoot(srcName string) bool {
	return srcName == conftestFilename
}

// allEmptyInits reports whether every entry in `srcs` is an `__init__.py`
// whose parsed AST has no top-level code. Empty `srcs` returns
// false — there's nothing to be "all empty inits" about. Emptiness is
// supplied by the rust import_extractor (FileImports.IsEmpty); a file
// missing from `results` is treated as non-empty so we never accidentally
// suppress a rule on a parser cache miss.
//
// Used by `python_skip_empty_init` to suppress both library and test rules.
// Covers the simple package case (sole src is one empty __init__.py), the
// project-mode rollup case (multiple nested empty __init__.py files and
// nothing else), and project-mode rollups where every test src is an
// `__init__.py` under a `tests/` subtree (matching the default `tests/**`
// test pattern). We deliberately do NOT strip empty `__init__.py` files
// from rules that also contain real sources — relative imports
// (`from . import x`) require the `__init__.py` to be part of the same
// rule as the importing module.
func allEmptyInits(srcs []string, specs []FileSpec, rel string, results map[string]FileImports) bool {
	return newSourceFacts(rel, specs, results).allEmptyInits(srcs)
}

// resolveRuleNames returns the (library, test) rule names for a directory,
// applying the directive overrides if set or falling back to package-name-
// derived defaults.
//
// Defaults — given a package at //apps/server (rel = "apps/server"):
//
//	library: "server"      → //apps/server:server (Bazel shortens to //apps/server)
//	test:    "server_test" → //apps/server:server_test
//
// Both can be overridden per-tree via the python_library_naming_convention /
// python_test_naming_convention directives. The directive value can include
// the rules_python `$package_name$` placeholder, which expands to the package
// basename. At the repo root (rel = ""), where there's no basename to derive
// from, library falls back to "lib" and test to "test".
func resolveRuleNames(cfg *pyConfig, rel string) (libName, testName string) {
	base := filepath.Base(rel)
	if base == "." || base == "" || base == "/" {
		base = ""
	}

	libName = applyNameConvention(cfg.libraryName, base)
	if libName == "" {
		if base != "" {
			libName = base
		} else {
			libName = "lib"
		}
	}

	testName = applyNameConvention(cfg.testName, base)
	if testName == "" {
		if base != "" {
			testName = base + "_test"
		} else {
			testName = "test"
		}
	}
	return
}

func resolveBinaryName(cfg *pyConfig, rel string) string {
	base := filepath.Base(rel)
	if base == "." || base == "" || base == "/" {
		base = ""
	}
	if name := applyNameConvention(cfg.binaryName, base); name != "" {
		return name
	}
	if base == "" {
		return "bin"
	}
	return base + "_bin"
}

// applyNameConvention substitutes the rules_python `$package_name$` placeholder
// in a naming-convention template. Returns "" when the directive isn't set, or
// when the placeholder would expand to an empty package name (repo root): in
// that case resolveRuleNames falls back to its literal "lib"/"test" defaults.
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

// isPythonFile checks the configured extensions list.
func isPythonFile(name string, cfg *pyConfig) bool {
	for _, ext := range cfg.extensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// isTestFile matches the file path against any of the configured test
// patterns. Patterns may contain `**` (matches across directories) and `*`
// (matches within a path segment).
func isTestFile(name string, cfg *pyConfig) bool {
	for _, pat := range cfg.testPatterns {
		if matchTestPattern(pat, name) {
			return true
		}
	}
	return false
}

// matchTestPattern dispatches a single pattern to doublestar's glob matcher.
// The path separator is fixed at `/` (matching Gazelle's workspace-relative
// path conventions, regardless of host OS), so we use Match — not PathMatch,
// which would split on the system separator.
//
// Pattern syntax (full doublestar grammar):
//   - `*`          matches any chars within a single path segment (no `/`)
//   - `**`         matches across path segments (zero or more)
//   - `?`          matches a single char
//   - `[abc]`      character class
//
// Concretely: `*_test.py` matches `foo_test.py` but NOT `pkg/foo_test.py`;
// for the latter use `**/*_test.py`. `tests/**` matches `tests`, `tests/x`,
// and `tests/sub/x` but not `src/tests/x`.
func matchTestPattern(pattern, name string) bool {
	ok, err := doublestar.Match(pattern, name)
	return ok && err == nil
}

// collectSrcs partitions the directory's files into library and test srcs,
// each sorted for deterministic BUILD output. We keep __init__.py in the
// library bucket so it travels with the package.
func collectSrcs(regularFiles []string, cfg *pyConfig) (libFiles, testFiles []string) {
	for _, f := range regularFiles {
		if !isPythonFile(f, cfg) {
			continue
		}
		if isTestFile(f, cfg) {
			testFiles = append(testFiles, f)
		} else {
			libFiles = append(libFiles, f)
		}
	}
	sort.Strings(libFiles)
	sort.Strings(testFiles)
	return
}
