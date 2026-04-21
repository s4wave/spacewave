# TinyGo

TinyGo is used on default for release WebAssembly builds.

It produces significantly smaller binaries than the Go compiler.

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
