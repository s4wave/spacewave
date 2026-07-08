# Spacewave BLAKE3 WASM sidecar

This crate builds the small browser `wasm32-unknown-unknown` BLAKE3 sidecar used only by the GoScript BLAKE3 owner package. It is a manual source artifact, not a normal Spacewave build, release, devtool, Bldr, or CI input.

One-time target install if the local nightly toolchain does not already have it:

```sh
~/.cargo/bin/rustup target add wasm32-unknown-unknown --toolchain nightly-x86_64-unknown-linux-gnu
```

Rebuild the committed artifact from this directory with:

```sh
RUSTFLAGS="-C opt-level=z -C lto=fat -C codegen-units=1 -C panic=abort -C strip=symbols" ~/.cargo/bin/cargo +nightly build --release --target wasm32-unknown-unknown
cp target/wasm32-unknown-unknown/release/spacewave_blake3.wasm blake3.wasm
```

The checked-in `blake3.wasm` is copied into GoScript browser output by Bldr. Do not add Cargo or Rust to ordinary Spacewave build requirements.
