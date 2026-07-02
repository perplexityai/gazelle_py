package py

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest_Basic(t *testing.T) {
	const content = `
manifest:
  pip_repository:
    name: my_pip
  modules_mapping:
    google.api_core: google-api-core
    PIL: pillow
    yaml: pyyaml
`
	m := parseManifest(content)
	if m.PipRepoName != "my_pip" {
		t.Errorf("PipRepoName = %q, want my_pip", m.PipRepoName)
	}
	cases := map[string]string{
		"google.api_core": "google-api-core",
		"PIL":             "pillow",
		"yaml":            "pyyaml",
	}
	for k, want := range cases {
		if got := m.ModulesMapping[k]; got != want {
			t.Errorf("ModulesMapping[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestParseManifest_EmptyAndComments(t *testing.T) {
	const content = `
# leading comment
manifest:
  # nested comment
  modules_mapping:
    foo: foo  # inline comment
`
	m := parseManifest(content)
	if m.ModulesMapping["foo"] != "foo" {
		t.Errorf("foo not mapped: %v", m.ModulesMapping)
	}
}

func TestParseManifest_NoManifestSection(t *testing.T) {
	m := parseManifest("# no manifest:\n\nfoo: bar\n")
	if len(m.ModulesMapping) != 0 {
		t.Errorf("expected empty mapping, got %v", m.ModulesMapping)
	}
}

func TestLoadManifestOnce_CachesByPath(t *testing.T) {
	dir := t.TempDir()
	rootPath := filepath.Join(dir, "root.yaml")
	dataPath := filepath.Join(dir, "data.yaml")

	if err := os.WriteFile(rootPath, []byte(`
manifest:
  pip_repository:
    name: pip
  modules_mapping:
    yaml: pyyaml
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte(`
manifest:
  pip_repository:
    name: pip_ai_training
  modules_mapping:
    pyspark: pyspark
`), 0o644); err != nil {
		t.Fatal(err)
	}

	root := loadManifestOnce(rootPath)
	data := loadManifestOnce(dataPath)

	if root.PipRepoName != "pip" {
		t.Fatalf("root PipRepoName = %q, want pip", root.PipRepoName)
	}
	if _, ok := root.ModulesMapping["pyspark"]; ok {
		t.Fatalf("root manifest unexpectedly has data mapping: %v", root.ModulesMapping)
	}
	if data.PipRepoName != "pip_ai_training" {
		t.Fatalf("data PipRepoName = %q, want pip_ai_training", data.PipRepoName)
	}
	if data.ModulesMapping["pyspark"] != "pyspark" {
		t.Fatalf("data pyspark mapping = %q, want pyspark", data.ModulesMapping["pyspark"])
	}
}
