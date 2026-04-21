# 🪶 Supporting Libraries

Common Go/TypeScript libraries:

- [**Common**][common] - Common project configuration files and Protobuf toolchain.
- [**go-kvfile**][go-kvfile] - File format for storing a compressed key/value store.
- [**goprotowrap**][goprotowrap] - Package-at-a-time wrapper for protoc.
- [**Util**][util] - Go utilities for easy concurrent programming.

These are lightweight reflection-free code-generation based implementations of
Protobuf designed to optimize binary size and performance, especially for
WebAssembly (wasm) applications:

- [**StaRPC**][starpc] - Protobuf 3 RPC services over any stream multiplexer
- [**protobuf-es-lite**][protobuf-es-lite] - Lightweight TypeScript protobuf implementation
- [**protobuf-go-lite**][protobuf-go-lite] - Lightweight Go protobuf implementation 
- [**protobuf-project**][protobuf-project] - Template repository for projects using protobufs

[starpc]: https://github.com/aperturerobotics/starpc
[protobuf-es-lite]: https://github.com/aperturerobotics/protobuf-es-lite
[protobuf-go-lite]: https://github.com/aperturerobotics/protobuf-go-lite
[protobuf-project]: https://github.com/aperturerobotics/protobuf-project

Protobuf libraries like [protobuf-es] and [protobuf-go] focus on spec compliance
and feature-complete implementations. These **lite** libraries focus on just the
base proto2 and proto3 spec including RPCs to simplify the implementation.

[protobuf-es]: https://github.com/bufbuild/protobuf-es
[protobuf-go]: https://github.com/protocolbuffers/protobuf-go

QuickJS WASI Reactor libraries:

- [**quickjs**][quickjs] - QuickJS-NG fork with a WASI reactor build target.
- [**go-quickjs-wasi**][go-quickjs-wasi] - Go module with an embedded QuickJS-NG WASI command binary.
- [**go-quickjs-wasi-reactor**][go-quickjs-wasi-reactor] - Go module with an embedded QuickJS-NG WASI reactor binary.
- [**js-quickjs-wasi-reactor**][js-quickjs-wasi-reactor] - Run QuickJS-NG within JavaScript using the WASI reactor model.

Lightweight / modified forks of other libraries:

- [**Cayley**][cayley] - Go Graph database forked from [cayleygraph]
- [**FlexLayout**][flex-layout] - Interactive drag/drop layout manager for React
- [**fastjson**][fastjson] - Reflection-free json parser and validator
- [**go-brotli-decoder**][go-brotli-decoder] - Pure Go Brotli decompressor
- [**go-indexeddb**][go-indexeddb] - Low-level Go driver for IndexedDB in Wasm
- [**json-iterator-lite**][json-iterator-lite] - Minimal and fast reflection-free json marshal and unmarshal for Go

[cayley]: https://github.com/aperturerobotics/cayley
[fastjson]: https://github.com/aperturerobotics/fastjson
[json-iterator-lite]: https://github.com/aperturerobotics/json-iterator-lite
[flex-layout]: https://github.com/aperturerobotics/flex-layout
[go-kvfile]: https://github.com/aperturerobotics/go-kvfile
[go-brotli-decoder]: https://github.com/aperturerobotics/go-brotli-decoder
[go-indexeddb]: https://github.com/aperturerobotics/go-indexeddb
[go-quickjs-wasi]: https://github.com/paralin/go-quickjs-wasi
[go-quickjs-wasi-reactor]: https://github.com/aperturerobotics/go-quickjs-wasi-reactor
[goprotowrap]: https://github.com/aperturerobotics/goprotowrap
[js-quickjs-wasi-reactor]: https://github.com/aperturerobotics/js-quickjs-wasi-reactor
[cayleygraph]: https://cayley.io/
[quickjs]: https://github.com/paralin/quickjs
[util]: https://github.com/aperturerobotics/util
[common]: https://github.com/aperturerobotics/common
