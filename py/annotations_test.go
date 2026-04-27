package py

import "testing"

func TestParseAnnotations(t *testing.T) {
	got := parseAnnotations([]string{
		"# gazelle:ignore sqlalchemy",
		"# regular comment",
		"# gazelle:ignore numpy pandas",
		"# gazelle:ignore torch,torchaudio,transformers",
		"# gazelle:include_dep //extra:dep",
	})
	for _, mod := range []string{"sqlalchemy", "numpy", "pandas", "torch", "torchaudio", "transformers"} {
		if !got.ignore[mod] {
			t.Errorf("expected %q in ignore set", mod)
		}
	}
	if got.ignore["regular"] {
		t.Error("unexpected 'regular' in ignore set")
	}
	if len(got.includeDep) != 1 || got.includeDep[0] != "//extra:dep" {
		t.Errorf("includeDep = %v", got.includeDep)
	}
}

func TestIsIgnored(t *testing.T) {
	ignore := map[string]bool{
		"pkg.core.connectors.oauth.definition": true,
		"sqlalchemy":                           true,
	}
	cases := []struct {
		mod  string
		from string
		want bool
	}{
		{"sqlalchemy", "", true},
		{"sqlalchemy.orm", "", true},
		{"pkg.core.connectors.oauth.definition.OAuth", "pkg.core.connectors.oauth.definition", true},
		{"pkg.core.connectors.oauth.definition", "", true},
		{"pkg.core.connectors.oauth.definition.foo.bar", "", true},
		{"numpy", "", false},
		{"pkg.core.connectors.oauth.other", "", false},
	}
	for _, c := range cases {
		if got := isIgnored(c.mod, c.from, ignore); got != c.want {
			t.Errorf("isIgnored(%q, %q) = %v, want %v", c.mod, c.from, got, c.want)
		}
	}
}
