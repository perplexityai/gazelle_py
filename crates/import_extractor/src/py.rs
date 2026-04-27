// py.rs -- rustpython-parser-based Python import extractor.
//
// Parses Python files via rustpython-parser and walks the AST to collect all
// imported module paths. This is the core parsing logic used by the gazelle Python
// plugin to determine dependencies between Python packages.
//
// Handles all import forms:
//   - import statements:    import foo, import foo.bar, import foo as f
//   - from-import:          from foo import x, from foo.bar import x
//   - relative imports:     from . import x, from .foo import x, from ..bar import x
//                           (encoded with leading "." prefix so the Go side can
//                           skip them as relative — same convention as gazelle_ts'
//                           relative-path handling.)
//
// The extracted paths are dotted module specifiers (e.g., "os.path",
// "myorg.api.client"). Resolution to Bazel labels happens on the Go side in
// resolve.go.

use rustpython_ast::Visitor;
use rustpython_ast::{Stmt, StmtImport, StmtImportFrom};
use rustpython_parser::{Parse, ast};
use std::collections::HashSet;

/// Extract all import paths from a Python file on disk.
pub fn extract_imports_from_file(path: &str) -> Result<Vec<String>, String> {
    let source_text =
        std::fs::read_to_string(path).map_err(|e| format!("Failed to read {path}: {e}"))?;
    Ok(extract_imports(path, &source_text))
}

/// Extract all import paths from Python source code.
///
/// On parse failure we return an empty list rather than erroring — gazelle
/// often runs against working trees mid-edit, and a hard fail there would
/// poison the whole BUILD-generation pass. The unparsed file is just a
/// missed-imports gap; the next run picks it up.
pub fn extract_imports(path: &str, source_text: &str) -> Vec<String> {
    let module = match ast::Suite::parse(source_text, path) {
        Ok(m) => m,
        Err(_) => return Vec::new(),
    };

    let mut visitor = ImportVisitor::new();
    for stmt in module {
        visitor.visit_stmt(stmt);
    }
    visitor.into_imports()
}

/// AST visitor that collects dotted import paths from Python source code.
struct ImportVisitor {
    imports: Vec<String>,
    seen: HashSet<String>,
}

impl ImportVisitor {
    fn new() -> Self {
        Self {
            imports: Vec::new(),
            seen: HashSet::new(),
        }
    }

    fn add(&mut self, path: String) {
        if !path.is_empty() && self.seen.insert(path.clone()) {
            self.imports.push(path);
        }
    }

    fn into_imports(self) -> Vec<String> {
        self.imports
    }
}

impl Visitor for ImportVisitor {
    // import foo, import foo.bar, import foo as f
    fn visit_stmt_import(&mut self, node: StmtImport) {
        for alias in &node.names {
            self.add(alias.name.to_string());
        }
        self.generic_visit_stmt_import(node);
    }

    // from foo import x, from foo.bar import x, from . import x, from .foo import x
    fn visit_stmt_import_from(&mut self, node: StmtImportFrom) {
        let level = node.level.map(|l| l.to_u32()).unwrap_or(0);
        let module = node.module.as_ref().map(|m| m.to_string()).unwrap_or_default();

        if level > 0 {
            // Relative import: encode with leading "." prefix per the level.
            // The Go resolver treats anything starting with "." as relative
            // and skips emitting a dep, so this works both for `from . import`
            // (level=1, module="") and `from .foo import` (level=1, module="foo").
            let prefix: String = ".".repeat(level as usize);
            self.add(format!("{prefix}{module}"));
        } else if !module.is_empty() {
            self.add(module);
        }
        self.generic_visit_stmt_import_from(node);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_file() {
        assert_eq!(extract_imports("test.py", ""), Vec::<String>::new());
    }

    #[test]
    fn plain_import() {
        let imports = extract_imports("test.py", "import os\nimport sys\n");
        assert_eq!(imports, vec!["os", "sys"]);
    }

    #[test]
    fn dotted_import() {
        let imports = extract_imports("test.py", "import os.path\n");
        assert_eq!(imports, vec!["os.path"]);
    }

    #[test]
    fn aliased_import() {
        let imports = extract_imports("test.py", "import numpy as np\n");
        assert_eq!(imports, vec!["numpy"]);
    }

    #[test]
    fn from_import() {
        let imports = extract_imports("test.py", "from os import path\n");
        assert_eq!(imports, vec!["os"]);
    }

    #[test]
    fn from_dotted_import() {
        let imports = extract_imports("test.py", "from myorg.api.client import Client\n");
        assert_eq!(imports, vec!["myorg.api.client"]);
    }

    #[test]
    fn relative_import_dot() {
        let imports = extract_imports("test.py", "from . import sibling\n");
        assert_eq!(imports, vec!["."]);
    }

    #[test]
    fn relative_import_dotted() {
        let imports = extract_imports("test.py", "from .foo import bar\n");
        assert_eq!(imports, vec![".foo"]);
    }

    #[test]
    fn relative_import_double_dot() {
        let imports = extract_imports("test.py", "from ..foo import bar\n");
        assert_eq!(imports, vec!["..foo"]);
    }

    #[test]
    fn deduplicates() {
        let imports = extract_imports(
            "test.py",
            "import os\nimport os\nfrom os import path\n",
        );
        assert_eq!(imports, vec!["os"]);
    }

    #[test]
    fn malformed_returns_empty() {
        // Should not panic — we swallow parse errors.
        let imports = extract_imports("test.py", "this is not valid python ::: !!!");
        assert!(imports.is_empty());
    }

    #[test]
    fn ignores_comments_and_strings() {
        let imports = extract_imports(
            "test.py",
            "# import os\nx = 'import sys'\nimport json\n",
        );
        assert_eq!(imports, vec!["json"]);
    }

    #[test]
    fn multiple_names_in_one_import() {
        let imports = extract_imports("test.py", "import os, sys, json\n");
        assert_eq!(imports, vec!["os", "sys", "json"]);
    }
}
