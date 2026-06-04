# Release builds

Release builds have some extra optimizations applied.

## wasm-opt

For the web platform, the .wasm binary is optimized with wasm-opt.

```
# brew
brew install binaryen

# or for apt
apt install binaryen
```

## gzip

For the web platform, the .wasm binary is compressed with gzip.

```
# brew
brew install gzip

# or for apt
apt install gzip
```

## precompressed browser assets

Release browser builds may point shell asset URLs at explicit `.gz` objects.
Those objects are still stored as gzip bytes, while the public response describes
the original asset type plus gzip encoding.

| URL suffix | Content-Type | Content-Encoding | Vary |
| --- | --- | --- | --- |
| `.wasm.gz` | `application/wasm` | `gzip` | `Accept-Encoding` |
| `.mjs.gz`, `.js.gz` | `application/javascript` | `gzip` | `Accept-Encoding` |
| `.css.gz` | `text/css; charset=utf-8` | `gzip` | `Accept-Encoding` |
| other immutable `.gz` assets | `application/octet-stream` | `gzip` | `Accept-Encoding` |

Upload paths should write this metadata when possible. Serving paths must infer
the same headers for known immutable `.gz` release assets when object metadata
is missing or too generic.

Local Bldr entrypoint/static serving, Bldr web package serving, and UnixFS HTTP
serving use the same encoded-asset file server for release-shaped asset trees.
That keeps local development and Space-hosted file serving aligned with Cloud
responses for `.wasm.gz`, JavaScript, CSS, and other immutable gzip assets.

The browser wasm loader still accepts both historical headerless gzip bytes and
responses that already carry `Content-Encoding: gzip`. The loader sniffs the
actual response body before applying `DecompressionStream('gzip')` and removes
`Content-Encoding` before handing the response to WebAssembly compilation, so a
browser-decoded response is not decompressed twice.

# Unused

These dependencies may be used in future.

## brotli

For the web platform, the .wasm binary can be compressed with brotli.

```
# brew
brew install brotli

# or for apt
apt install brotli
```

NOTE: This is not currently used as DecompressionStream only supports gz.
