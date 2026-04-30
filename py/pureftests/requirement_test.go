package pureftests

import (
	"testing"
)

func TestParseRequirementLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "comment line",
			input:    "# foo",
			expected: "",
		},
		{
			name:     "requirements file include",
			input:    "-r other.txt",
			expected: "",
		},
		{
			name:     "simple package name",
			input:    "requests",
			expected: "requests",
		},
		{
			name:     "package with version specifier",
			input:    "requests>=2.0",
			expected: "requests",
		},
		{
			name:     "package with exact version",
			input:    "flask==2.1.0",
			expected: "flask",
		},
		{
			name:     "package with compatible version",
			input:    "django~=4.0",
			expected: "django",
		},
		{
			name:     "package with extras",
			input:    "celery[redis]",
			expected: "celery",
		},
		{
			name:     "package with platform marker",
			input:    "pywin32 ; sys_platform=='win32'",
			expected: "pywin32",
		},
		{
			name:     "package with comment",
			input:    "numpy  # fast arrays",
			expected: "numpy",
		},
		{
			name:     "package with hyphens becomes underscores",
			input:    "My-Package",
			expected: "my_package",
		},
		{
			name:     "complex requirement with extras, version, marker, and comment",
			input:    "SomePackage[extra]>=1.0 ; python_version<'3.10'  # comment",
			expected: "somepackage",
		},
		{
			name:     "package with not equal version",
			input:    "package!=1.0",
			expected: "package",
		},
		{
			name:     "package with less than version",
			input:    "package<2.0",
			expected: "package",
		},
		{
			name:     "package with greater than version",
			input:    "package>1.0",
			expected: "package",
		},
		{
			name:     "package with less than or equal version",
			input:    "package<=2.0",
			expected: "package",
		},
		{
			name:     "whitespace handling",
			input:    "  package  ",
			expected: "package",
		},
		{
			name:     "package with dots and hyphens",
			input:    "My-Package.Name",
			expected: "my_package.name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRequirementLine(tt.input)
			if result != tt.expected {
				t.Errorf("parseRequirementLine(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDeduplicateAndSort(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: nil,
		},
		{
			name:     "already sorted no duplicates",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "unsorted with duplicates",
			input:    []string{"c", "a", "b", "a", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "single element",
			input:    []string{"single"},
			expected: []string{"single"},
		},
		{
			name:     "all duplicates",
			input:    []string{"a", "a", "a"},
			expected: []string{"a"},
		},
		{
			name:     "already sorted with duplicates",
			input:    []string{"a", "a", "b", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateAndSort(tt.input)
			if !equalStringSlices(result, tt.expected) {
				t.Errorf("deduplicateAndSort(%v) = %v; want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPipLabel(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		distName string
		expected string
	}{
		{
			name:     "standard pip pattern",
			pattern:  "@pip//{pkg}",
			distName: "requests",
			expected: "@pip//requests",
		},
		{
			name:     "custom pip pattern",
			pattern:  "@my_pip//{pkg}",
			distName: "numpy",
			expected: "@my_pip//numpy",
		},
		{
			name:     "path-based pattern",
			pattern:  "//third_party/pip:{pkg}",
			distName: "scikit_learn",
			expected: "//third_party/pip:scikit_learn",
		},
		{
			name:     "pattern without placeholder",
			pattern:  "//third_party/pip:fixed",
			distName: "requests",
			expected: "//third_party/pip:fixed",
		},
		{
			name:     "multiple placeholders (should replace all)",
			pattern:  "@pip//{pkg}/{pkg}",
			distName: "test",
			expected: "@pip//test/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pipLabel(tt.pattern, tt.distName)
			if result != tt.expected {
				t.Errorf("pipLabel(%q, %q) = %q; want %q", tt.pattern, tt.distName, result, tt.expected)
			}
		})
	}
}

func TestPipLabelForRepo(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		repo     string
		distName string
		expected string
	}{
		{
			name:     "explicit repo overrides pattern repo",
			pattern:  "@pip//{pkg}",
			repo:     "my_pip",
			distName: "requests",
			expected: "@my_pip//requests",
		},
		{
			name:     "empty repo falls back to plain substitution",
			pattern:  "@pip//{pkg}",
			repo:     "",
			distName: "numpy",
			expected: "@pip//numpy",
		},
		{
			name:     "pattern without @ prefix: repo ignored, plain substitution",
			pattern:  "//third_party/pip:{pkg}",
			repo:     "my_pip",
			distName: "scikit_learn",
			expected: "//third_party/pip:scikit_learn",
		},
		{
			name:     "pattern without @ prefix and no //: repo ignored",
			pattern:  "third_party:{pkg}",
			repo:     "my_pip",
			distName: "test",
			expected: "third_party:test",
		},
		{
			name:     "pattern with @ but no //: repo ignored",
			pattern:  "@pip{pkg}",
			repo:     "my_pip",
			distName: "test",
			expected: "@piptest",
		},
		{
			name:     "complex pattern with repo override",
			pattern:  "@pip//path/to/{pkg}",
			repo:     "custom_pip",
			distName: "package",
			expected: "@custom_pip//path/to/package",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pipLabelForRepo(tt.pattern, tt.repo, tt.distName)
			if result != tt.expected {
				t.Errorf("pipLabelForRepo(%q, %q, %q) = %q; want %q", tt.pattern, tt.repo, tt.distName, result, tt.expected)
			}
		})
	}
}

func TestParsePipRepo(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected string
	}{
		{
			name:     "standard pip pattern",
			pattern:  "@pip//{pkg}",
			expected: "pip",
		},
		{
			name:     "custom pip pattern",
			pattern:  "@my_pip//{pkg}",
			expected: "my_pip",
		},
		{
			name:     "path-based pattern without @",
			pattern:  "//pip/{pkg}",
			expected: "",
		},
		{
			name:     "pattern with @ but no //",
			pattern:  "@pip",
			expected: "",
		},
		{
			name:     "pattern with @ but no slash after",
			pattern:  "@pip{pkg}",
			expected: "",
		},
		{
			name:     "empty pattern",
			pattern:  "",
			expected: "",
		},
		{
			name:     "pattern starting with @ but empty repo",
			pattern:  "@//{pkg}",
			expected: "",
		},
		{
			name:     "complex repo name",
			pattern:  "@my_complex_pip_repo//{pkg}",
			expected: "my_complex_pip_repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePipRepo(tt.pattern)
			if result != tt.expected {
				t.Errorf("parsePipRepo(%q) = %q; want %q", tt.pattern, result, tt.expected)
			}
		})
	}
}

// Helper function for comparing string slices
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
