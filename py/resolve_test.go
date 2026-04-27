package py

import (
	"reflect"
	"testing"
)

func TestNormalizeDist(t *testing.T) {
	cases := map[string]string{
		"requests": "requests",
		"NumPy":    "numpy",
		"cv2":      "opencv_python",
		"PIL":      "pillow",
		"sklearn":  "scikit_learn",
	}
	for in, want := range cases {
		if got := normalizeDist(in); got != want {
			t.Errorf("normalizeDist(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPipLabel(t *testing.T) {
	cases := []struct {
		pattern string
		dist    string
		want    string
	}{
		{"@pip//{pkg}", "requests", "@pip//requests"},
		{"@my_pip//{pkg}", "numpy", "@my_pip//numpy"},
		{"//third_party/pip:{pkg}", "scikit_learn", "//third_party/pip:scikit_learn"},
	}
	for _, c := range cases {
		cfg := &pyConfig{pipLinkPattern: c.pattern}
		got := pipLabel(cfg, c.dist)
		if got != c.want {
			t.Errorf("pipLabel(%q, %q) = %q, want %q", c.pattern, c.dist, got, c.want)
		}
	}
}

func TestParseRequirementLine(t *testing.T) {
	cases := map[string]string{
		"requests":                 "requests",
		"requests==2.31.0":         "requests",
		"requests>=2.31.0":         "requests",
		"requests[security]":       "requests",
		"requests ; python<'3.10'": "requests",
		"# comment":                "",
		"":                         "",
		"-e .":                     "",
		"scikit-learn==1.0":        "scikit_learn",
		"NumPy":                    "numpy",
	}
	for in, want := range cases {
		if got := parseRequirementLine(in); got != want {
			t.Errorf("parseRequirementLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScanPyProjectDeps(t *testing.T) {
	content := `
[build-system]
requires = ["hatchling"]

[project]
name = "myproj"
dependencies = [
  "requests>=2.31",
  "numpy",
  # comment in array
  "scikit-learn",
]
`
	got := scanPyProjectDeps(content)
	want := []string{"requests", "numpy", "scikit_learn"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scanPyProjectDeps = %v, want %v", got, want)
	}
}

func TestDeduplicateAndSort(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{nil, nil},
		{[]string{}, nil},
		{[]string{"b", "a", "b", "c"}, []string{"a", "b", "c"}},
		{[]string{"x"}, []string{"x"}},
	}
	for _, c := range cases {
		got := deduplicateAndSort(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("deduplicateAndSort(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestPythonStdlibCovered(t *testing.T) {
	for _, mod := range []string{"os", "sys", "json", "re", "asyncio", "typing"} {
		if !pythonStdlib[mod] {
			t.Errorf("expected %q in pythonStdlib", mod)
		}
	}
}
