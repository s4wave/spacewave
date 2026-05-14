# TinyGo

TinyGo is an explicit experimental opt-in for compatible WebAssembly builds.
The default remains the standard Go compiler until browser plugin product proof
passes.

Enable it from Bldr Go plugin or dist compiler config with
`enableTinygo: "ENABLE"` on a TinyGo-compatible WebAssembly platform such as
`web/js/wasm`.

## Installation

We need the dev version of TinyGo.

```
git clone https://github.com/tinygo-org/tinygo
cd tinygo

# release branch as of 2024-05-04
git checkout 6384ecace093df2d0b93915886954abfc4ecfe01
git submodule update --init --recursive

# download llvm source
make llvm-source

# see: https://tinygo.org/docs/guides/build/manual-llvm/
make llvm-build

# build wasi-libc
make wasi-libc

# build wasm-ld
# https://github.com/tinygo-org/tinygo/pull/4254
cd ./llvm-build && ninja lld && cd -

# build tinygo
make

# add to your PATH
export PATH=$PATH:$(pwd)/build
```
