// stub.go is hand-written Go that mirrors the public surface of the
// Bazel-generated `go_proto_library` output. It exists only so that
// `go build`, `go mod tidy`, and `gopls` can resolve the import path
// `github.com/hermeticbuild/gazelle_py/py/proto` outside
// of Bazel.
//
// At Bazel build time, the real types come from
// //py/proto:import_extractor_go_proto and these stubs are
// shadowed (rules_go's go_library replaces the package when compiling).
//
// Keep the public API in sync with proto/message.proto.
//
//go:build never

package proto

// Marker so this file never compiles outside Bazel. The build tag above
// ensures `go build` skips it; if you remove the tag, Bazel will still win
// because it doesn't honor build tags this way for go_proto_library outputs.
