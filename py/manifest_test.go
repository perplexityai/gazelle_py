package py

import "testing"

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
