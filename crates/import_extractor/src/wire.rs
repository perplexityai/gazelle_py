use crate::py;
use import_extractor_proto::gazelle_py::import_extractor as pb;
use prost::Message;
use rayon::prelude::*;

pub fn encode_response(resp: pb::Response) -> Vec<u8> {
    resp.encode_to_vec()
}

/// Decode a request frame and produce the encoded response bytes.
///
/// Returns an error response (encoded) when the input bytes don't decode as a `Request`.
pub fn dispatch(frame: &[u8]) -> Vec<u8> {
    let req = match pb::Request::decode(frame) {
        Ok(r) => r,
        Err(e) => {
            eprintln!("import_extractor: invalid request: {e}");
            return encode_response(pb::Response {
                id: 0,
                data: Some(pb::response::Data::Error(pb::ResponseError {
                    message: format!("invalid request: {e}"),
                })),
            });
        }
    };

    let id = req.id;
    let resp = match req.data {
        Some(pb::request::Data::PyQuery(py_req)) => handle_py(id, py_req),
        None => pb::Response {
            id,
            data: Some(pb::response::Data::Error(pb::ResponseError {
                message: "missing request data".to_string(),
            })),
        },
    };

    encode_response(resp)
}

pub fn handle_py(id: u32, req: pb::PyQueryRequest) -> pb::Response {
    let results: Vec<pb::PyFileOutput> = req
        .files
        .par_iter()
        .filter_map(
            |f| match py::extract_imports_from_file(&f.path, &f.rel_path) {
                Ok(output) => Some(pb::PyFileOutput {
                    file_name: output.file_name,
                    modules: output
                        .modules
                        .into_iter()
                        .map(|m| pb::PyModule {
                            name: m.name,
                            lineno: m.lineno,
                            filepath: m.filepath,
                            from: m.from,
                            type_checking_only: m.type_checking_only,
                        })
                        .collect(),
                    comments: output.comments,
                    has_main: output.has_main,
                    is_empty: output.is_empty,
                }),
                Err(e) => {
                    eprintln!("import_extractor: skipping {}: {e}", f.path);
                    None
                }
            },
        )
        .collect();

    pb::Response {
        id,
        data: Some(pb::response::Data::PyResult(pb::PyResponseResult { results })),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn build_py_request(id: u32, specs: Vec<(&str, &str)>) -> Vec<u8> {
        pb::Request {
            id,
            data: Some(pb::request::Data::PyQuery(pb::PyQueryRequest {
                files: specs
                    .into_iter()
                    .map(|(p, r)| pb::PyFileSpec {
                        path: p.into(),
                        rel_path: r.into(),
                    })
                    .collect(),
            })),
        }
        .encode_to_vec()
    }

    fn decode(bytes: &[u8]) -> pb::Response {
        pb::Response::decode(bytes).expect("response decodes")
    }

    #[test]
    fn dispatch_returns_error_for_garbage_frame() {
        let resp = decode(&dispatch(&[0xff, 0xff, 0xff, 0xff]));
        assert_eq!(resp.id, 0);
        match resp.data {
            Some(pb::response::Data::Error(e)) => assert!(e.message.starts_with("invalid request")),
            _ => panic!("expected error variant"),
        }
    }

    #[test]
    fn dispatch_returns_error_when_data_oneof_is_missing() {
        let req = pb::Request { id: 7, data: None }.encode_to_vec();
        let resp = decode(&dispatch(&req));
        assert_eq!(resp.id, 7);
        match resp.data {
            Some(pb::response::Data::Error(e)) => assert_eq!(e.message, "missing request data"),
            _ => panic!("expected error variant"),
        }
    }

    #[test]
    fn dispatch_preserves_request_id_on_py_query() {
        let req = build_py_request(42, vec![]);
        let resp = decode(&dispatch(&req));
        assert_eq!(resp.id, 42);
        assert!(matches!(resp.data, Some(pb::response::Data::PyResult(_))));
    }

    #[test]
    fn handle_py_skips_files_that_fail_to_read() {
        let resp = handle_py(
            1,
            pb::PyQueryRequest {
                files: vec![pb::PyFileSpec {
                    path: "/nonexistent/file/that/cannot/be/read.py".into(),
                    rel_path: "missing.py".into(),
                }],
            },
        );
        match resp.data {
            Some(pb::response::Data::PyResult(r)) => assert!(r.results.is_empty()),
            _ => panic!("expected py_result"),
        }
    }

    #[test]
    fn handle_py_returns_modules_for_real_file() {
        let dir = std::env::temp_dir().join("import_extractor_wire_test_py");
        std::fs::create_dir_all(&dir).unwrap();
        let path = dir.join("a.py");
        std::fs::write(&path, "import os\nfrom sys import argv\n").unwrap();

        let resp = handle_py(
            5,
            pb::PyQueryRequest {
                files: vec![pb::PyFileSpec {
                    path: path.to_string_lossy().into_owned(),
                    rel_path: "a.py".into(),
                }],
            },
        );
        match resp.data {
            Some(pb::response::Data::PyResult(r)) => {
                assert_eq!(r.results.len(), 1);
                let out = &r.results[0];
                assert_eq!(out.file_name, "a.py");
                assert_eq!(out.modules.len(), 2);
                assert_eq!(out.modules[0].name, "os");
                assert_eq!(out.modules[1].name, "sys.argv");
            }
            _ => panic!("expected py_result"),
        }
    }
}
