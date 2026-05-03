// ffi.rs -- C ABI for cgo callers.
//
// Two functions:
//
//   gazelle_py_ie_dispatch(req_ptr, req_len, *out_resp_ptr, *out_resp_len)
//     Decodes a protobuf Request, dispatches, encodes the Response, and hands
//     the encoded bytes back via the out parameters. The caller owns them
//     until gazelle_py_ie_free.
//
//   gazelle_py_ie_free(ptr, len)
//     Releases bytes returned by gazelle_py_ie_dispatch.

use crate::wire;
use std::slice;

/// # Safety
/// `req_ptr` must point to `req_len` valid bytes. `out_resp_ptr` and
/// `out_resp_len` must be valid out-pointers. The bytes written to
/// `*out_resp_ptr` are owned by the caller until they call
/// `gazelle_py_ie_free`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gazelle_py_ie_dispatch(
    req_ptr: *const u8,
    req_len: usize,
    out_resp_ptr: *mut *mut u8,
    out_resp_len: *mut usize,
) {
    let req = if req_len == 0 {
        &[][..]
    } else {
        unsafe { slice::from_raw_parts(req_ptr, req_len) }
    };

    let resp = wire::dispatch(req);
    let len = resp.len();
    let mut boxed = resp.into_boxed_slice();
    let ptr = boxed.as_mut_ptr();
    std::mem::forget(boxed);

    unsafe {
        *out_resp_ptr = ptr;
        *out_resp_len = len;
    }
}

/// # Safety
/// `ptr` must have been returned by `gazelle_py_ie_dispatch` paired with the
/// same `len`. Calling with a null pointer is a no-op.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gazelle_py_ie_free(ptr: *mut u8, len: usize) {
    if ptr.is_null() || len == 0 {
        return;
    }
    unsafe {
        let _ = Box::from_raw(slice::from_raw_parts_mut(ptr, len));
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use import_extractor_proto::gazelle_py::import_extractor as pb;
    use prost::Message;
    use std::ptr;

    #[test]
    fn dispatch_roundtrip_via_ffi() {
        let req = pb::Request {
            id: 42,
            data: Some(pb::request::Data::PyQuery(pb::PyQueryRequest {
                files: vec![],
            })),
        }
        .encode_to_vec();

        let mut out_ptr: *mut u8 = ptr::null_mut();
        let mut out_len: usize = 0;
        unsafe {
            gazelle_py_ie_dispatch(req.as_ptr(), req.len(), &mut out_ptr, &mut out_len);
        }
        assert!(!out_ptr.is_null());
        assert!(out_len > 0);

        let resp_bytes = unsafe { slice::from_raw_parts(out_ptr, out_len) };
        let resp = pb::Response::decode(resp_bytes).expect("decode");
        assert_eq!(resp.id, 42);
        assert!(matches!(resp.data, Some(pb::response::Data::PyResult(_))));

        unsafe { gazelle_py_ie_free(out_ptr, out_len) };
    }

    #[test]
    fn dispatch_empty_input_returns_error_response() {
        let mut out_ptr: *mut u8 = ptr::null_mut();
        let mut out_len: usize = 0;
        unsafe {
            gazelle_py_ie_dispatch(ptr::null(), 0, &mut out_ptr, &mut out_len);
        }
        assert!(!out_ptr.is_null());
        let resp_bytes = unsafe { slice::from_raw_parts(out_ptr, out_len) };
        let resp = pb::Response::decode(resp_bytes).expect("decode");
        assert!(matches!(resp.data, Some(pb::response::Data::Error(_))));
        unsafe { gazelle_py_ie_free(out_ptr, out_len) };
    }

    #[test]
    fn gazelle_py_ie_free_handles_null_and_zero() {
        unsafe { gazelle_py_ie_free(ptr::null_mut(), 0) };
        unsafe { gazelle_py_ie_free(ptr::null_mut(), 100) };
    }
}
