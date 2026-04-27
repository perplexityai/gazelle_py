package py

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// manifestData mirrors the rules_python_gazelle_plugin's gazelle_python.yaml.
// It maps Python import names (top-level modules) to PyPI distribution names
// so the resolver can emit the right `@<pip>//<dist>` label without guessing.
//
//	manifest:
//	  pip_repository:
//	    name: pip
//	  modules_mapping:
//	    google.api_core: google-api-core
//	    PIL:             pillow
type manifestData struct {
	PipRepoName    string            // empty string means "pip"
	ModulesMapping map[string]string // import-name → PyPI distribution
}

// loadManifest reads gazelle_python.yaml at the configured path. Missing file
// is not an error — callers fall back to deriving deps from pyproject.toml /
// requirements.txt (see resolve.go).
func loadManifest(path string) (*manifestData, error) {
	if path == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		// resolve.go passes the absolute path; this guard is just belt-and-braces.
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseManifest(string(data)), nil
}

// parseManifest is a tiny dependency-free YAML reader for our specific shape.
// We avoid pulling in a full YAML parser because the file format is small,
// fixed, and the only thing we read is two flat sections under `manifest:`.
//
// Anything we can't parse cleanly is silently skipped — the resolver treats
// "no mapping" the same as "module not in manifest", which is the right
// failure mode (gazelle prints a missing-dep diagnostic the user can fix).
func parseManifest(content string) *manifestData {
	out := &manifestData{ModulesMapping: map[string]string{}}

	var (
		section      string // "" | "modules_mapping" | "pip_repository"
		baseIndent   = -1   // indent of `manifest:` block contents
		sectionStart = -1   // indent of the current section header
	)

	for _, raw := range strings.Split(content, "\n") {
		if i := strings.Index(raw, "#"); i >= 0 {
			raw = raw[:i]
		}
		line := strings.TrimRight(raw, " \t")
		if line == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		trimmed := strings.TrimSpace(line)

		// Top-level `manifest:` opens the block we care about.
		if indent == 0 {
			section = ""
			sectionStart = -1
			if trimmed == "manifest:" {
				baseIndent = 0
			} else if baseIndent == 0 {
				// We've left the manifest block at top-level.
				baseIndent = -1
			}
			continue
		}

		if baseIndent < 0 {
			continue
		}

		// Section header line (e.g. `  modules_mapping:`).
		if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, " ") {
			section = strings.TrimSuffix(trimmed, ":")
			sectionStart = indent
			continue
		}

		// Skip lines that aren't deeper than the section header.
		if section == "" || sectionStart < 0 || indent <= sectionStart {
			continue
		}

		// key: value
		key, value, ok := splitYAMLKV(trimmed)
		if !ok {
			continue
		}
		switch section {
		case "modules_mapping":
			if value != "" {
				out.ModulesMapping[key] = value
			}
		case "pip_repository":
			if key == "name" && value != "" {
				out.PipRepoName = value
			}
		}
	}

	return out
}

// splitYAMLKV splits "key: value" with optional surrounding quotes on the
// value. Returns false for lines that don't have a colon.
func splitYAMLKV(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	value = strings.Trim(value, `"'`)
	return key, value, true
}

var (
	manifestOnce   sync.Once
	manifestCached *manifestData
)

// loadManifestOnce caches the manifest across Configure calls so we don't
// re-read the file for every directory in the gazelle walk.
func loadManifestOnce(path string) *manifestData {
	manifestOnce.Do(func() {
		m, err := loadManifest(path)
		if err != nil {
			// Treat read errors the same as missing — the user gets diagnostics
			// from gazelle when a dep can't be resolved.
			manifestCached = &manifestData{ModulesMapping: map[string]string{}}
			return
		}
		if m == nil {
			manifestCached = &manifestData{ModulesMapping: map[string]string{}}
			return
		}
		manifestCached = m
	})
	return manifestCached
}
