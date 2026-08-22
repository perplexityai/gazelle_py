package py

import (
	"reflect"
	"testing"
)

func TestLiteralStringListAttrRejectsComputedElements(t *testing.T) {
	file := mustLoadBuildFile(t, "pkg", `
filegroup(
    name = "literal",
    srcs = ["first.py", "second.py"],
)

filegroup(
    name = "mixed",
    srcs = ["literal.py", GENERATED_SRCS],
)

filegroup(
    name = "selected",
    srcs = select({"//conditions:default": ["selected.py"]}),
)
`)

	want := []string{"first.py", "second.py"}
	if got, ok := literalStringListAttr(file.Rules[0], "srcs"); !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("literal srcs = %v, %v; want %v, true", got, ok, want)
	}
	if got, ok := literalStringListAttr(file.Rules[1], "srcs"); ok || !reflect.DeepEqual(got, []string{"literal.py"}) {
		t.Errorf("mixed srcs = %v, %v; want [literal.py], false", got, ok)
	}
	if got, ok := literalStringListAttr(file.Rules[2], "srcs"); ok || len(got) != 0 {
		t.Errorf("selected srcs = %v, %v; want no direct literals, false", got, ok)
	}
}

func TestHandOwnedSourcesIgnoresResourceRules(t *testing.T) {
	file := mustLoadBuildFile(t, "pkg", `
filegroup(
    name = "resources",
    srcs = ["payload.py"],
)
`)
	ownership := newPackageSourceOwnership(newPyConfig(), nil, file, nil, []string{"payload.py"})

	if got := ownership.handOwnedSources(); len(got) != 0 {
		t.Fatalf("hand-owned sources = %v, want resource rule ignored", got)
	}
}

func TestHandOwnedSourcesIgnoresUnrecognizedRules(t *testing.T) {
	file := mustLoadBuildFile(t, "pkg", `
pkg_tar(
    name = "archive",
    srcs = ["app.py"],
)
`)
	ownership := newPackageSourceOwnership(newPyConfig(), nil, file, nil, []string{"app.py"})

	if got := ownership.handOwnedSources(); len(got) != 0 {
		t.Fatalf("hand-owned sources = %v, want unrecognized rule ignored", got)
	}
}

func TestHandOwnedSourcesIgnoresUnmappedTestRules(t *testing.T) {
	file := mustLoadBuildFile(t, "pkg", `
custom_functional_test(
    name = "integration_test",
    srcs = ["test_app.py"],
)
`)
	ownership := newPackageSourceOwnership(newPyConfig(), nil, file, nil, []string{"test_app.py"})

	if got := ownership.handOwnedSources(); len(got) != 0 {
		t.Fatalf("hand-owned sources = %v, want unmapped test rule ignored", got)
	}
}

func TestHandOwnedSourcesIgnoresUnmappedLaunchers(t *testing.T) {
	file := mustLoadBuildFile(t, "pkg", `
custom_launcher(
    name = "launch",
    main = "entry.py",
    srcs = ["helper.py"],
)
`)
	ownership := newPackageSourceOwnership(newPyConfig(), nil, file, nil, []string{"entry.py", "helper.py"})

	if got := ownership.handOwnedSources(); len(got) != 0 {
		t.Fatalf("hand-owned sources = %v, want unmapped launcher ignored", got)
	}
}

func TestLiteralStringAttrRejectsComputedValues(t *testing.T) {
	file := mustLoadBuildFile(t, "pkg", `
custom_binary(
    name = "literal",
    main = "main.py",
)

custom_binary(
    name = "computed",
    main = MAIN,
)
`)

	if got, ok := literalStringAttr(file.Rules[0], "main"); !ok || got != "main.py" {
		t.Fatalf("literal main = %q, %v; want main.py, true", got, ok)
	}
	if got, ok := literalStringAttr(file.Rules[1], "main"); ok {
		t.Fatalf("computed main = %q, true; want attribute left unmanaged", got)
	}
}
