package pureftests

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyNameConvention(t *testing.T) {
	tests := []struct {
		name     string
		template string
		pkgBase  string
		expected string
	}{
		{
			name:     "empty template",
			template: "",
			pkgBase:  "server",
			expected: "",
		},
		{
			name:     "template without placeholder",
			template: "fixed_name",
			pkgBase:  "server",
			expected: "fixed_name",
		},
		{
			name:     "template with placeholder and base",
			template: "$package_name$_lib",
			pkgBase:  "server",
			expected: "server_lib",
		},
		{
			name:     "template with placeholder, empty pkgBase",
			template: "$package_name$_lib",
			pkgBase:  "",
			expected: "",
		},
		{
			name:     "template with multiple placeholders",
			template: "$package_name$_$package_name$",
			pkgBase:  "test",
			expected: "test_test",
		},
		{
			name:     "template with placeholder at start",
			template: "$package_name$_suffix",
			pkgBase:  "prefix",
			expected: "prefix_suffix",
		},
		{
			name:     "template with placeholder at end",
			template: "prefix_$package_name$",
			pkgBase:  "suffix",
			expected: "prefix_suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyNameConvention(tt.template, tt.pkgBase)
			if result != tt.expected {
				t.Errorf("applyNameConvention(%q, %q) = %q; want %q", tt.template, tt.pkgBase, result, tt.expected)
			}
		})
	}
}

func TestMatchTestPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		filename string
		expected bool
	}{
		{
			name:     "simple pattern match",
			pattern:  "*_test.py",
			filename: "foo_test.py",
			expected: true,
		},
		{
			name:     "simple pattern no match in subdirectory",
			pattern:  "*_test.py",
			filename: "pkg/foo_test.py",
			expected: false,
		},
		{
			name:     "recursive pattern match in root",
			pattern:  "**/*_test.py",
			filename: "foo_test.py",
			expected: true,
		},
		{
			name:     "recursive pattern match in subdirectory",
			pattern:  "**/*_test.py",
			filename: "pkg/foo_test.py",
			expected: true,
		},
		{
			name:     "directory pattern match",
			pattern:  "tests/**",
			filename: "tests/x.py",
			expected: true,
		},
		{
			name:     "directory pattern match nested",
			pattern:  "tests/**",
			filename: "tests/sub/x.py",
			expected: true,
		},
		{
			name:     "directory pattern no match outside",
			pattern:  "tests/**",
			filename: "src/tests/x.py",
			expected: false,
		},
		{
			name:     "prefix pattern match",
			pattern:  "test_*.py",
			filename: "test_foo.py",
			expected: true,
		},
		{
			name:     "prefix pattern no match in subdirectory",
			pattern:  "test_*.py",
			filename: "pkg/test_foo.py",
			expected: false,
		},
		{
			name:     "invalid pattern returns false without panic",
			pattern:  "[invalid",
			filename: "foo.py",
			expected: false,
		},
		{
			name:     "exact match",
			pattern:  "conftest.py",
			filename: "conftest.py",
			expected: true,
		},
		{
			name:     "exact match no match",
			pattern:  "conftest.py",
			filename: "other.py",
			expected: false,
		},
		{
			name:     "complex recursive pattern",
			pattern:  "**/test_*.py",
			filename: "deep/nested/test_something.py",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchTestPattern(tt.pattern, tt.filename)
			if result != tt.expected {
				t.Errorf("matchTestPattern(%q, %q) = %v; want %v", tt.pattern, tt.filename, result, tt.expected)
			}
		})
	}
}

// TestPkgRelativePath uses forward-slash paths because pkgRelativePath operates
// on Gazelle workspace-relative paths, which always use "/" as the separator.
func TestPkgRelativePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pkg      string
		expected string
	}{
		{
			name:     "empty pkg returns workspace path",
			path:     "apps/server/utils/x.py",
			pkg:      "",
			expected: "apps/server/utils/x.py",
		},
		{
			name:     "path equals pkg returns basename",
			path:     "apps/server",
			pkg:      "apps/server",
			expected: "server",
		},
		{
			name:     "path with pkg prefix returns relative",
			path:     "apps/server/utils/x.py",
			pkg:      "apps/server",
			expected: "utils/x.py",
		},
		{
			name:     "path without pkg prefix returns as-is",
			path:     "other/path/x.py",
			pkg:      "apps/server",
			expected: "other/path/x.py",
		},
		{
			name:     "deep nesting",
			path:     "apps/server/utils/helpers/x.py",
			pkg:      "apps/server",
			expected: "utils/helpers/x.py",
		},
		{
			name:     "single character pkg",
			path:     "a/b/c.py",
			pkg:      "a",
			expected: "b/c.py",
		},
		{
			name:     "empty path",
			path:     "",
			pkg:      "apps/server",
			expected: "",
		},
		{
			name:     "both empty",
			path:     "",
			pkg:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pkgRelativePath(tt.path, tt.pkg)
			if result != tt.expected {
				t.Errorf("pkgRelativePath(%q, %q) = %q; want %q", tt.path, tt.pkg, result, tt.expected)
			}
		})
	}
}

func TestPerFileRuleName(t *testing.T) {
	tests := []struct {
		name     string
		srcName  string
		expected string
	}{
		{
			name:     "simple .py file",
			srcName:  "foo.py",
			expected: "foo",
		},
		{
			name:     "test file",
			srcName:  "foo_test.py",
			expected: "foo_test",
		},
		{
			name:     "file in subdirectory",
			srcName:  "sub/bar.py",
			expected: "bar",
		},
		{
			name:     "stub file",
			srcName:  "foo.pyi",
			expected: "foo",
		},
		{
			name:     "file with multiple extensions",
			srcName:  "test.py.bak",
			expected: "test.py.bak",
		},
		{
			name:     "file without extension",
			srcName:  "foo",
			expected: "foo",
		},
		{
			name:     "just .py extension",
			srcName:  ".py",
			expected: "",
		},
		{
			name:     "complex filename",
			srcName:  "my_complex_file_name.py",
			expected: "my_complex_file_name",
		},
		{
			name:     "stub file with test suffix",
			srcName:  "foo_test.pyi",
			expected: "foo_test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := perFileRuleName(tt.srcName)
			if result != tt.expected {
				t.Errorf("perFileRuleName(%q) = %q; want %q", tt.srcName, result, tt.expected)
			}
		})
	}
}

func TestIsInitFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected bool
	}{
		{
			name:     "root __init__.py",
			filename: "__init__.py",
			expected: true,
		},
		{
			name:     "subdirectory __init__.py",
			filename: "sub/__init__.py",
			expected: true,
		},
		{
			name:     "deep __init__.py",
			filename: "deep/nested/__init__.py",
			expected: true,
		},
		{
			name:     "init.py (not __init__.py)",
			filename: "init.py",
			expected: false,
		},
		{
			name:     "regular python file",
			filename: "foo.py",
			expected: false,
		},
		{
			name:     "file with init in name",
			filename: "my_init_file.py",
			expected: false,
		},
		{
			name:     "__init__ with different extension",
			filename: "__init__.pyi",
			expected: false,
		},
		{
			name:     "empty string",
			filename: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInitFile(tt.filename)
			if result != tt.expected {
				t.Errorf("isInitFile(%q) = %v; want %v", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestIsEmptyPython(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "file with only blank lines",
			content:  "   \n  \t\n\n  ",
			expected: true,
		},
		{
			name:     "file with only comments",
			content:  "# This is a comment\n# Another comment\n   # Indented comment",
			expected: true,
		},
		{
			name:     "file with blank line then comment",
			content:  "\n\n# Comment after blank lines",
			expected: true,
		},
		{
			name:     "file with import os",
			content:  "import os\n",
			expected: false,
		},
		{
			name:     "file with pass",
			content:  "pass\n",
			expected: false,
		},
		{
			name:     "file with mix of comments and real code",
			content:  "# Comment\nimport sys\n# Another comment\nx = 1\n",
			expected: false,
		},
		{
			name:     "empty file",
			content:  "",
			expected: true,
		},
		{
			name:     "file with docstring",
			content:  "\"\"\"Module docstring\"\"\"\n",
			expected: false,
		},
		{
			name:     "file with only whitespace and comments",
			content:  "\t\n  \n# Comment\n   \n\n",
			expected: true,
		},
		{
			name:     "file with single character code",
			content:  "x\n",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "test.py")

			err := os.WriteFile(filePath, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			result := isEmptyPython(filePath)
			if result != tt.expected {
				t.Errorf("isEmptyPython(%q) = %v; want %v", tt.content, result, tt.expected)
			}
		})
	}

	// Test nonexistent path
	t.Run("nonexistent path", func(t *testing.T) {
		result := isEmptyPython("/nonexistent/path/file.py")
		if result != false {
			t.Errorf("isEmptyPython(nonexistent) = %v; want false", result)
		}
	})
}
