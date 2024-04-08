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

## brotli

For the web platform, the .wasm binary is compressed with brotli.

```
# brew
brew install brotli

# or for apt
apt install brotli
```
