# Shielder Go Wrapper

This package is the Go boundary for the Rust Shielder privacy engine.

From repo root, build the native library before running packages that import
this wrapper:

```sh
cargo build -p thornado-ffi --release
cd go-thornado
CGO_ENABLED=1 go test ./go-wrappers/shielder
```

The cgo directives search `../target/release` first, then `../target/debug`.
Release and CI builds should use the release library.
