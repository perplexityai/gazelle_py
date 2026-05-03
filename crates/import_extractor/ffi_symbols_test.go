package ffi_symbols_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var symbolsPath = flag.String("symbols", "", "path to nm output for the import_extractor staticlib")

func TestStaticlibExportsNamespacedFFISymbols(t *testing.T) {
	path := resolveRunfile(t, *symbolsPath)
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	symbols := string(out)
	for _, symbol := range []string{"gazelle_py_ie_dispatch", "gazelle_py_ie_free"} {
		if !definesSymbol(symbols, symbol) {
			t.Fatalf("expected exported symbol %q in %s", symbol, path)
		}
	}
	for _, symbol := range []string{"ie_dispatch", "ie_free"} {
		if definesSymbol(symbols, symbol) {
			t.Fatalf("generic exported symbol %q must not be present in %s", symbol, path)
		}
	}
}

func resolveRunfile(t *testing.T, path string) string {
	t.Helper()
	if path == "" {
		t.Fatal("-symbols is required")
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	for _, env := range []string{"RUNFILES_DIR", "TEST_SRCDIR"} {
		if root := os.Getenv(env); root != "" {
			for _, candidate := range []string{
				filepath.Join(root, path),
				filepath.Join(root, os.Getenv("TEST_WORKSPACE"), path),
				filepath.Join(root, "_main", path),
			} {
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			}
		}
	}
	t.Fatalf("symbol file not found: %s", path)
	return ""
}

func definesSymbol(nmOutput, symbol string) bool {
	for _, line := range strings.Split(nmOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[len(fields)-1]
		kind := fields[len(fields)-2]
		if kind == "U" {
			continue
		}
		if name == symbol || name == "_"+symbol {
			return true
		}
	}
	return false
}
