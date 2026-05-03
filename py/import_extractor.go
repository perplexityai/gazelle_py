// Package-internal cgo bridge to the Rust import_extractor staticlib.
//
// The crate at //crates/import_extractor exposes a 2-function C ABI
// (gazelle_py_ie_dispatch / gazelle_py_ie_free) wrapping the protobuf wire
// dispatcher. We marshal a Request, hand the bytes to gazelle_py_ie_dispatch,
// unmarshal the Response, and free the buffer the Rust side allocated.
package py

/*
#include <stddef.h>
#include <stdint.h>

void gazelle_py_ie_dispatch(
    const uint8_t *req_ptr,
    size_t req_len,
    uint8_t **out_resp_ptr,
    size_t *out_resp_len);

void gazelle_py_ie_free(uint8_t *ptr, size_t len);
*/
import "C"

import (
	"fmt"
	"unsafe"

	pb "github.com/hermeticbuild/gazelle_py/py/proto"

	"google.golang.org/protobuf/proto"
)

// extractImports sends a batch of file specs through the FFI. Files that fail
// to parse are silently dropped by the Rust side (the parser does error
// recovery and emits whatever it could read), so the caller's `len(results)`
// can be smaller than `len(specs)`.
func extractImports(specs []FileSpec) ([]FileImports, error) {
	files := make([]*pb.PyFileSpec, len(specs))
	for i, s := range specs {
		files[i] = &pb.PyFileSpec{Path: s.Path, RelPath: s.RelPath}
	}
	req := &pb.Request{
		Data: &pb.Request_PyQuery{
			PyQuery: &pb.PyQueryRequest{Files: files},
		},
	}
	resp, err := dispatch(req)
	if err != nil {
		return nil, err
	}
	switch d := resp.Data.(type) {
	case *pb.Response_Error:
		return nil, fmt.Errorf("import-extractor: %s", d.Error.Message)
	case *pb.Response_PyResult:
		out := make([]FileImports, 0, len(d.PyResult.Results))
		for _, r := range d.PyResult.Results {
			modules := make([]ImportStatement, 0, len(r.Modules))
			for _, m := range r.Modules {
				modules = append(modules, ImportStatement{
					ImportPath:       m.Name,
					From:             m.From,
					SourceFile:       m.Filepath,
					LineNumber:       m.Lineno,
					TypeCheckingOnly: m.TypeCheckingOnly,
				})
			}
			out = append(out, FileImports{
				FileName: r.FileName,
				Modules:  modules,
				Comments: r.Comments,
				HasMain:  r.HasMain,
				IsEmpty:  r.IsEmpty,
			})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("import-extractor: empty response oneof")
	}
}

func dispatch(req *pb.Request) (*pb.Response, error) {
	reqBytes, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var reqPtr *C.uint8_t
	if len(reqBytes) > 0 {
		reqPtr = (*C.uint8_t)(unsafe.Pointer(&reqBytes[0]))
	}

	var respPtr *C.uint8_t
	var respLen C.size_t
	C.gazelle_py_ie_dispatch(reqPtr, C.size_t(len(reqBytes)), &respPtr, &respLen)

	if respPtr == nil || respLen == 0 {
		return nil, fmt.Errorf("import-extractor: empty response from FFI")
	}
	defer C.gazelle_py_ie_free(respPtr, respLen)

	respBytes := C.GoBytes(unsafe.Pointer(respPtr), C.int(respLen))
	var resp pb.Response
	if err := proto.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}
