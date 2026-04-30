package pureftests

import (
	"testing"
)

func TestNormalizeDist(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		mode     labelNormalizationType
		expected string
	}{
		// snakeCaseNormalization tests
		{
			name:     "snakeCase: opencv-python to opencv_python",
			input:    "opencv-python",
			mode:     snakeCaseNormalization,
			expected: "opencv_python",
		},
		{
			name:     "snakeCase: my.pkg to my_pkg",
			input:    "my.pkg",
			mode:     snakeCaseNormalization,
			expected: "my_pkg",
		},
		{
			name:     "snakeCase: already snake unchanged",
			input:    "already_snake_case",
			mode:     snakeCaseNormalization,
			expected: "already_snake_case",
		},
		{
			name:     "snakeCase: mixed separators",
			input:    "My-Package.Name",
			mode:     snakeCaseNormalization,
			expected: "my_package_name",
		},

		// pep503Normalization tests
		{
			name:     "pep503: My.Pkg_Name to my-pkg-name",
			input:    "My.Pkg_Name",
			mode:     pep503Normalization,
			expected: "my-pkg-name",
		},
		{
			name:     "pep503: Multi___Underscore to multi-underscore",
			input:    "Multi___Underscore",
			mode:     pep503Normalization,
			expected: "multi-underscore",
		},
		{
			name:     "pep503: Mixed separators to single dash",
			input:    "Test-Package.Name_With_Many---Separators",
			mode:     pep503Normalization,
			expected: "test-package-name-with-many-separators",
		},
		{
			name:     "pep503: already pep503 unchanged",
			input:    "already-pep503-name",
			mode:     pep503Normalization,
			expected: "already-pep503-name",
		},

		// noneNormalization tests
		{
			name:     "none: identity function",
			input:    "My-Package.Name",
			mode:     noneNormalization,
			expected: "My-Package.Name",
		},
		{
			name:     "none: empty string",
			input:    "",
			mode:     noneNormalization,
			expected: "",
		},

		// Alias map tests - all six entries in pythonImportToDist
		{
			name:     "alias: cv2 to opencv-python (snake case)",
			input:    "cv2",
			mode:     snakeCaseNormalization,
			expected: "opencv_python",
		},
		{
			name:     "alias: PIL to pillow (snake case)",
			input:    "PIL",
			mode:     snakeCaseNormalization,
			expected: "pillow",
		},
		{
			name:     "alias: sklearn to scikit-learn (snake case)",
			input:    "sklearn",
			mode:     snakeCaseNormalization,
			expected: "scikit_learn",
		},
		{
			name:     "alias: yaml to pyyaml (snake case)",
			input:    "yaml",
			mode:     snakeCaseNormalization,
			expected: "pyyaml",
		},
		{
			name:     "alias: bs4 to beautifulsoup4 (snake case)",
			input:    "bs4",
			mode:     snakeCaseNormalization,
			expected: "beautifulsoup4",
		},
		{
			name:     "alias: OpenSSL to pyopenssl (snake case)",
			input:    "OpenSSL",
			mode:     snakeCaseNormalization,
			expected: "pyopenssl",
		},
		{
			name:     "alias: dateutil to python-dateutil (snake case)",
			input:    "dateutil",
			mode:     snakeCaseNormalization,
			expected: "python_dateutil",
		},

		// Alias map tests with pep503 normalization
		{
			name:     "alias: cv2 to opencv-python (pep503)",
			input:    "cv2",
			mode:     pep503Normalization,
			expected: "opencv-python",
		},
		{
			name:     "alias: sklearn to scikit-learn (pep503)",
			input:    "sklearn",
			mode:     pep503Normalization,
			expected: "scikit-learn",
		},

		// Empty string tests in all modes
		{
			name:     "empty: snake case",
			input:    "",
			mode:     snakeCaseNormalization,
			expected: "",
		},
		{
			name:     "empty: pep503",
			input:    "",
			mode:     pep503Normalization,
			expected: "",
		},
		{
			name:     "empty: none",
			input:    "",
			mode:     noneNormalization,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeDist(tt.input, tt.mode)
			if result != tt.expected {
				t.Errorf("normalizeDist(%q, %v) = %q; want %q", tt.input, tt.mode, result, tt.expected)
			}
		})
	}
}
