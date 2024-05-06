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
