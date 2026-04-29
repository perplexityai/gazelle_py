package py

import (
	"reflect"
	"testing"
)

func TestNormalizeDist_SnakeCase(t *testing.T) {
	cases := map[string]string{
		"requests": "requests",
		"NumPy":    "numpy",
		"cv2":      "opencv_python",
		"PIL":      "pillow",
		"sklearn":  "scikit_learn",
		"dateutil": "python_dateutil",
	}
	for in, want := range cases {
		if got := normalizeDist(in, snakeCaseNormalization); got != want {
			t.Errorf("normalizeDist(%q, snake_case) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeDist_Pep503(t *testing.T) {
	cases := map[string]string{
		"requests":         "requests",
		"NumPy":            "numpy",
		"cv2":              "opencv-python",
		"sklearn":          "scikit-learn",
		"dateutil":         "python-dateutil",
		"Some.Weird_Name":  "some-weird-name",
		"Multi___Underscore": "multi-underscore",
	}
	for in, want := range cases {
		if got := normalizeDist(in, pep503Normalization); got != want {
			t.Errorf("normalizeDist(%q, pep503) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeDist_None(t *testing.T) {
	// Identity, except the import→dist map still applies (caller's option).
	cases := map[string]string{
		"requests": "requests",
		"NumPy":    "NumPy",
		"cv2":      "opencv-python", // map lookup hits, then identity preserves form
	}
	for in, want := range cases {
		if got := normalizeDist(in, noneNormalization); got != want {
			t.Errorf("normalizeDist(%q, none) = %q, want %q", in, got, want)
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

func TestScanPyProjectDeps_ExtrasInStrings(t *testing.T) {
	// `]` inside a string literal (extras like `celery[redis]`) must not
	// terminate the array — every dep should still be captured.
	content := `
[project]
name = "myproj"
dependencies = [
  "aiohttp==3.11.16",
  "celery[redis]>=5.3.0,<6",
  "datadog==0.51.0",
  "ddtrace==2.18.1",
  "requests[security]==2.33.1",
]
`
	got := scanPyProjectDeps(content)
	want := []string{"aiohttp", "celery", "datadog", "ddtrace", "requests"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scanPyProjectDeps with extras = %v, want %v", got, want)
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
