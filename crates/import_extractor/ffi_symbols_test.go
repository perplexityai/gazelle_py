package ffi_symbols_test

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var symbolsPath = flag.String("symbols", "", "path to the import_extractor static archive")

func TestStaticlibExportsNamespacedFFISymbols(t *testing.T) {
	path := resolveRunfile(t, *symbolsPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	symbols := readSymbols(t, data)
	for _, symbol := range []string{"gazelle_py_ie_dispatch", "gazelle_py_ie_free"} {
		if !symbols[symbol] {
			t.Fatalf("expected global symbol %q in %s", symbol, path)
		}
	}
	for _, symbol := range []string{"ie_dispatch", "ie_free"} {
		if symbols[symbol] {
			t.Fatalf("generic global symbol %q must not be present in %s", symbol, path)
		}
	}
	for _, symbol := range []string{"__rust_alloc", "rust_eh_personality"} {
		if symbols[symbol] {
			t.Fatalf("Rust runtime symbol %q must be local in %s", symbol, path)
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

func readSymbols(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	if bytes.HasPrefix(data, []byte("!<arch>\n")) {
		return archiveGlobalSymbols(t, data)
	}
	if f, err := macho.NewFile(bytes.NewReader(data)); err == nil {
		return machoGlobalSymbols(f)
	}
	if f, err := elf.NewFile(bytes.NewReader(data)); err == nil {
		return elfGlobalSymbols(t, f)
	}
	t.Fatal("object file is neither Mach-O nor ELF")
	return nil
}

func archiveGlobalSymbols(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	offset := 8
	for offset < len(data) {
		if offset+60 > len(data) {
			t.Fatalf("invalid archive member header at offset %d", offset)
		}
		header := data[offset : offset+60]
		size := atoi(t, bytes.TrimSpace(header[48:58]))
		if string(header[58:60]) != "`\n" {
			t.Fatalf("invalid archive member terminator at offset %d", offset)
		}
		offset += 60
		if offset+size > len(data) {
			t.Fatalf("archive member at offset %d exceeds archive size", offset)
		}
		member := data[offset : offset+size]
		name := bytes.TrimSpace(header[:16])
		if bytes.HasPrefix(name, []byte("#1/")) {
			nameLen := atoi(t, name[3:])
			if nameLen > len(member) {
				t.Fatalf("archive member name length %d exceeds member size %d", nameLen, len(member))
			}
			member = member[nameLen:]
		}
		for symbol := range readObjectSymbols(member) {
			out[symbol] = true
		}
		offset += size
		if offset%2 != 0 {
			offset++
		}
	}
	return out
}

func atoi(t *testing.T, b []byte) int {
	t.Helper()
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			t.Fatalf("invalid archive size %q", string(b))
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func readObjectSymbols(data []byte) map[string]bool {
	if f, err := macho.NewFile(bytes.NewReader(data)); err == nil {
		return machoGlobalSymbols(f)
	}
	if f, err := elf.NewFile(bytes.NewReader(data)); err == nil {
		out := map[string]bool{}
		syms, err := f.Symbols()
		if err != nil {
			return out
		}
		for _, sym := range syms {
			if sym.Section == elf.SHN_UNDEF || elf.ST_BIND(sym.Info) != elf.STB_GLOBAL {
				continue
			}
			out[sym.Name] = true
		}
		return out
	}
	return nil
}

func machoGlobalSymbols(f *macho.File) map[string]bool {
	out := map[string]bool{}
	if f.Symtab == nil {
		return out
	}
	for _, sym := range f.Symtab.Syms {
		if sym.Sect == 0 || sym.Type&0x01 == 0 {
			continue
		}
		name := sym.Name
		if len(name) > 0 && name[0] == '_' {
			name = name[1:]
		}
		out[name] = true
	}
	return out
}

func elfGlobalSymbols(t *testing.T, f *elf.File) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	syms, err := f.Symbols()
	if err != nil {
		t.Fatalf("read ELF symbols: %v", err)
	}
	for _, sym := range syms {
		if sym.Section == elf.SHN_UNDEF || elf.ST_BIND(sym.Info) != elf.STB_GLOBAL {
			continue
		}
		out[sym.Name] = true
	}
	return out
}
