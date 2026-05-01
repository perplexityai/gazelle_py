// py.rs -- ruff-based Python import extractor.
//
// Parses Python files with ruff_python_parser and walks the AST to collect:
// - Import statements (import X, from X import Y)
// - Comments (for gazelle annotations like # gazelle:ignore)
// - __main__ detection (if __name__ == "__main__":)
//
// Output matches the Go-side PyFileOutput/PyModule structs exactly so that
// downstream gazelle code (annotation parsing, dependency resolution) requires
// zero translation beyond the wire format.

use ruff_python_ast::{self as ast, Expr, Stmt};
use ruff_python_parser::{Mode, parse_unchecked};
use ruff_text_size::Ranged;

/// A parsed Python module import.
#[derive(Debug, Clone, PartialEq)]
pub struct PyModule {
    pub name: String,
    pub lineno: u32,
    pub filepath: String,
    pub from: String,
    pub type_checking_only: bool,
}

/// Output from parsing a single Python file.
#[derive(Debug, Clone)]
pub struct PyFileOutput {
    pub file_name: String,
    pub modules: Vec<PyModule>,
    pub comments: Vec<String>,
    pub has_main: bool,
    /// True iff the parsed AST has no top-level statements — i.e. the file
    /// is whitespace and/or comments only. A docstring, `pass`, or any
    /// import/assignment counts as a statement and yields false. Drives
    /// `python_skip_empty_init` on the Go side.
    pub is_empty: bool,
}

/// Extract imports from a Python file on disk.
pub fn extract_imports_from_file(abs_path: &str, rel_path: &str) -> Result<PyFileOutput, String> {
    let source =
        std::fs::read_to_string(abs_path).map_err(|e| format!("Failed to read {abs_path}: {e}"))?;
    Ok(extract_imports(&source, rel_path))
}

/// Extract imports, comments, and main detection from Python source code.
///
/// For malformed files, ruff performs error recovery and produces a partial AST.
/// We extract imports from whatever the parser could recover, which is the right
/// behavior for gazelle: partially-edited files during development still get
/// their valid imports resolved.
pub fn extract_imports(source: &str, rel_filepath: &str) -> PyFileOutput {
    let parsed = parse_unchecked(source, Mode::Module.into());
    let module = parsed.into_syntax();

    let stmts = match module {
        ast::Mod::Module(m) => m.body,
        ast::Mod::Expression(_) => return empty_output(rel_filepath),
    };

    let is_empty = stmts.is_empty();

    let mut modules = Vec::new();
    let mut has_main = false;

    extract_from_stmts(
        &stmts,
        source,
        rel_filepath,
        false,
        &mut modules,
        &mut has_main,
    );

    let comments = extract_comments(source);

    PyFileOutput {
        file_name: rel_filepath.to_string(),
        modules,
        comments,
        has_main,
        is_empty,
    }
}

fn extract_from_stmts(
    stmts: &[Stmt],
    source: &str,
    rel_filepath: &str,
    in_type_checking: bool,
    modules: &mut Vec<PyModule>,
    has_main: &mut bool,
) {
    for stmt in stmts {
        match stmt {
            Stmt::Import(import) => {
                for alias in &import.names {
                    let name = alias.name.as_str();
                    if name.starts_with('.') {
                        continue;
                    }
                    let lineno = line_number(source, alias.range().start());
                    modules.push(PyModule {
                        name: name.to_string(),
                        lineno,
                        filepath: rel_filepath.to_string(),
                        from: String::new(),
                        type_checking_only: in_type_checking,
                    });
                }
            }
            Stmt::ImportFrom(import_from) => {
                let level = import_from.level;
                let module_name = import_from
                    .module
                    .as_ref()
                    .map(|m| m.as_str())
                    .unwrap_or("");

                let from_prefix = if level > 0 {
                    let dots = ".".repeat(level as usize);
                    if module_name.is_empty() {
                        dots
                    } else {
                        format!("{dots}{module_name}")
                    }
                } else {
                    module_name.to_string()
                };

                // Skip bare relative imports: "from . import X", "from .. import X", etc.
                if level > 0 && module_name.is_empty() {
                    continue;
                }

                for alias in &import_from.names {
                    let alias_name = alias.name.as_str();
                    if alias_name == "*" {
                        let lineno = line_number(source, alias.range().start());
                        modules.push(PyModule {
                            name: from_prefix.clone(),
                            lineno,
                            filepath: rel_filepath.to_string(),
                            from: from_prefix.clone(),
                            type_checking_only: in_type_checking,
                        });
                        continue;
                    }

                    let full_name = if from_prefix.is_empty() {
                        alias_name.to_string()
                    } else {
                        format!("{from_prefix}.{alias_name}")
                    };

                    let lineno = line_number(source, alias.range().start());
                    modules.push(PyModule {
                        name: full_name,
                        lineno,
                        filepath: rel_filepath.to_string(),
                        from: from_prefix.clone(),
                        type_checking_only: in_type_checking,
                    });
                }
            }
            Stmt::If(if_stmt) => {
                let is_type_checking = is_type_checking_test(&if_stmt.test);
                if is_main_test(&if_stmt.test) {
                    *has_main = true;
                }
                extract_from_stmts(
                    &if_stmt.body,
                    source,
                    rel_filepath,
                    in_type_checking || is_type_checking,
                    modules,
                    has_main,
                );
                for clause in &if_stmt.elif_else_clauses {
                    extract_from_stmts(
                        &clause.body,
                        source,
                        rel_filepath,
                        in_type_checking,
                        modules,
                        has_main,
                    );
                }
            }
            Stmt::Try(try_stmt) => {
                extract_from_stmts(
                    &try_stmt.body,
                    source,
                    rel_filepath,
                    in_type_checking,
                    modules,
                    has_main,
                );
                for handler in &try_stmt.handlers {
                    let ast::ExceptHandler::ExceptHandler(h) = handler;
                    extract_from_stmts(
                        &h.body,
                        source,
                        rel_filepath,
                        in_type_checking,
                        modules,
                        has_main,
                    );
                }
                extract_from_stmts(
                    &try_stmt.orelse,
                    source,
                    rel_filepath,
                    in_type_checking,
                    modules,
                    has_main,
                );
                extract_from_stmts(
                    &try_stmt.finalbody,
                    source,
                    rel_filepath,
                    in_type_checking,
                    modules,
                    has_main,
                );
            }
            // Recurse into all compound statement bodies to find deferred/inline
            // imports. Python allows imports inside functions, classes, with blocks,
            // for loops, while loops, and async variants.
            Stmt::FunctionDef(func_def) => {
                extract_from_stmts(
                    &func_def.body,
                    source,
                    rel_filepath,
                    in_type_checking,
                    modules,
                    has_main,
                );
            }
            Stmt::ClassDef(class_def) => {
                extract_from_stmts(
                    &class_def.body,
                    source,
                    rel_filepath,
                    in_type_checking,
                    modules,
                    has_main,
                );
            }
            Stmt::With(with_stmt) => {
                extract_from_stmts(
                    &with_stmt.body,
                    source,
                    rel_filepath,
                    in_type_checking,
                    modules,
                    has_main,
                );
            }
            Stmt::For(for_stmt) => {
                extract_from_stmts(
                    &for_stmt.body,
                    source,
                    rel_filepath,
                    in_type_checking,
                    modules,
                    has_main,
                );
                extract_from_stmts(
                    &for_stmt.orelse,
                    source,
                    rel_filepath,
                    in_type_checking,
                    modules,
                    has_main,
                );
            }
            Stmt::While(while_stmt) => {
                extract_from_stmts(
                    &while_stmt.body,
                    source,
                    rel_filepath,
                    in_type_checking,
                    modules,
                    has_main,
                );
                extract_from_stmts(
                    &while_stmt.orelse,
                    source,
                    rel_filepath,
                    in_type_checking,
                    modules,
                    has_main,
                );
            }
            _ => {}
        }
    }
}

/// `TYPE_CHECKING` or `typing.TYPE_CHECKING`.
fn is_type_checking_test(expr: &Expr) -> bool {
    match expr {
        Expr::Name(name) => name.id.as_str() == "TYPE_CHECKING",
        Expr::Attribute(attr) => {
            attr.attr.as_str() == "TYPE_CHECKING"
                && matches!(&*attr.value, Expr::Name(name) if name.id.as_str() == "typing")
        }
        _ => false,
    }
}

/// `__name__ == "__main__"`.
fn is_main_test(expr: &Expr) -> bool {
    if let Expr::Compare(cmp) = expr {
        if cmp.ops.len() != 1 || cmp.comparators.len() != 1 {
            return false;
        }
        if !matches!(cmp.ops[0], ast::CmpOp::Eq) {
            return false;
        }
        let is_name = matches!(&*cmp.left, Expr::Name(name) if name.id.as_str() == "__name__");
        let is_main =
            matches!(&cmp.comparators[0], Expr::StringLiteral(s) if s.value.to_str() == "__main__");
        return is_name && is_main;
    }
    false
}

fn extract_comments(source: &str) -> Vec<String> {
    source
        .lines()
        .filter_map(|line| {
            let trimmed = line.trim();
            if trimmed.starts_with('#') {
                Some(trimmed.to_string())
            } else {
                None
            }
        })
        .collect()
}

fn line_number(source: &str, offset: ruff_text_size::TextSize) -> u32 {
    let byte_offset = offset.to_u32() as usize;
    let line = source[..byte_offset.min(source.len())]
        .bytes()
        .filter(|&b| b == b'\n')
        .count();
    (line + 1) as u32
}

fn empty_output(rel_filepath: &str) -> PyFileOutput {
    PyFileOutput {
        file_name: rel_filepath.to_string(),
        modules: Vec::new(),
        comments: Vec::new(),
        has_main: false,
        is_empty: true,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn simple_import() {
        let out = extract_imports("import os\nimport sys", "test.py");
        assert_eq!(out.modules.len(), 2);
        assert_eq!(out.modules[0].name, "os");
        assert_eq!(out.modules[1].name, "sys");
        assert_eq!(out.modules[0].from, "");
    }

    #[test]
    fn from_import() {
        let out = extract_imports("from os.path import join, exists", "test.py");
        assert_eq!(out.modules.len(), 2);
        assert_eq!(out.modules[0].name, "os.path.join");
        assert_eq!(out.modules[0].from, "os.path");
        assert_eq!(out.modules[1].name, "os.path.exists");
    }

    #[test]
    fn relative_import() {
        let out = extract_imports("from .sibling import foo", "test.py");
        assert_eq!(out.modules.len(), 1);
        assert_eq!(out.modules[0].name, ".sibling.foo");
        assert_eq!(out.modules[0].from, ".sibling");
    }

    #[test]
    fn bare_relative_import_skipped() {
        let out = extract_imports("from . import foo", "test.py");
        assert_eq!(out.modules.len(), 0);
    }

    #[test]
    fn wildcard_import() {
        let out = extract_imports("from os.path import *", "test.py");
        assert_eq!(out.modules.len(), 1);
        assert_eq!(out.modules[0].name, "os.path");
        assert_eq!(out.modules[0].from, "os.path");
    }

    #[test]
    fn type_checking_block() {
        let out = extract_imports(
            "import os\nif TYPE_CHECKING:\n    import typing\nimport sys",
            "test.py",
        );
        assert_eq!(out.modules.len(), 3);
        assert!(!out.modules[0].type_checking_only);
        assert!(out.modules[1].type_checking_only);
        assert!(!out.modules[2].type_checking_only);
    }

    #[test]
    fn typing_type_checking_block() {
        let out = extract_imports(
            "if typing.TYPE_CHECKING:\n    from foo import Bar",
            "test.py",
        );
        assert_eq!(out.modules.len(), 1);
        assert!(out.modules[0].type_checking_only);
    }

    #[test]
    fn has_main() {
        let out = extract_imports("if __name__ == \"__main__\":\n    main()", "test.py");
        assert!(out.has_main);
    }

    #[test]
    fn no_main() {
        let out = extract_imports("import os", "test.py");
        assert!(!out.has_main);
    }

    #[test]
    fn comments() {
        let out = extract_imports(
            "# gazelle:ignore sqlalchemy\nimport os\n# regular comment",
            "test.py",
        );
        assert_eq!(out.comments.len(), 2);
        assert_eq!(out.comments[0], "# gazelle:ignore sqlalchemy");
        assert_eq!(out.comments[1], "# regular comment");
    }

    #[test]
    fn line_numbers() {
        let out = extract_imports("import os\nimport sys\nimport json", "test.py");
        assert_eq!(out.modules[0].lineno, 1);
        assert_eq!(out.modules[1].lineno, 2);
        assert_eq!(out.modules[2].lineno, 3);
    }

    #[test]
    fn try_except_imports() {
        let out = extract_imports(
            "try:\n    import ujson\nexcept ImportError:\n    import json",
            "test.py",
        );
        assert_eq!(out.modules.len(), 2);
        assert_eq!(out.modules[0].name, "ujson");
        assert_eq!(out.modules[1].name, "json");
    }

    #[test]
    fn malformed_file_recovers_valid_imports() {
        let out = extract_imports("import os\ndef {{{broken\nimport sys", "test.py");
        assert!(!out.modules.is_empty());
        assert!(out.modules.iter().any(|m| m.name == "os"));
    }

    #[test]
    fn empty_file() {
        let out = extract_imports("", "test.py");
        assert!(out.modules.is_empty());
        assert!(out.comments.is_empty());
        assert!(!out.has_main);
        assert!(out.is_empty, "blank file should report is_empty=true");
    }

    #[test]
    fn whitespace_only_is_empty() {
        let out = extract_imports("\n\n   \n\t\n", "test.py");
        assert!(out.is_empty);
    }

    #[test]
    fn comments_only_is_empty() {
        // Whole-line `#` comments don't appear in the AST, so a file made up of
        // only blank lines and comments has zero top-level statements.
        let out = extract_imports("# header\n\n# another comment\n", "test.py");
        assert!(out.is_empty);
        assert_eq!(out.comments.len(), 2);
    }

    #[test]
    fn docstring_only_is_not_empty() {
        // Module docstrings are real statements (Expr Stmt holding a string
        // literal) — they assign to `__doc__` at runtime. Treat as code.
        let out = extract_imports("\"\"\"module docstring\"\"\"\n", "test.py");
        assert!(!out.is_empty);
    }

    #[test]
    fn pass_only_is_not_empty() {
        let out = extract_imports("pass\n", "test.py");
        assert!(!out.is_empty);
    }

    #[test]
    fn import_only_is_not_empty() {
        let out = extract_imports("import os\n", "test.py");
        assert!(!out.is_empty);
    }

    #[test]
    fn trailing_comment_on_code_is_not_empty() {
        // Inline trailing comments are stripped at the AST level but the
        // assignment is still a real statement.
        let out = extract_imports("x = 1  # note\n", "test.py");
        assert!(!out.is_empty);
    }

    #[test]
    fn assignment_only_is_not_empty() {
        // `__all__ = []` is the canonical case where someone explicitly
        // marks a package as code-bearing despite re-exports living elsewhere.
        let out = extract_imports("__all__ = []\n", "test.py");
        assert!(!out.is_empty);
    }

    #[test]
    fn function_body_import() {
        let out = extract_imports(
            "def foo():\n    from bar import baz\n    import qux",
            "test.py",
        );
        assert_eq!(out.modules.len(), 2);
        assert_eq!(out.modules[0].name, "bar.baz");
        assert_eq!(out.modules[1].name, "qux");
    }

    #[test]
    fn extract_from_file_real() {
        let dir = std::env::temp_dir();
        let path = dir.join("test_py_extract_oss.py");
        std::fs::write(&path, "import os\nfrom sys import argv").unwrap();
        let result = extract_imports_from_file(path.to_str().unwrap(), "test.py");
        assert!(result.is_ok());
        let out = result.unwrap();
        assert_eq!(out.modules.len(), 2);
        assert_eq!(out.modules[0].name, "os");
        assert_eq!(out.modules[1].name, "sys.argv");
        std::fs::remove_file(path).ok();
    }
}
