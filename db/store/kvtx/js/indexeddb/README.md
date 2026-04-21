# Testing

Use [wasmbrowsertest]

## GitHub Action Reference

```yaml
name: Tests

on:
  push:
    branches: [ "master" ]
  pull_request:
    # The branches below must be a subset of the branches above
    branches: [ "master" ]

# Builds images for target boards.
permissions:
  contents: read

jobs:
  tests:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.24']
    timeout-minutes: 10
    steps:
    - uses: actions/checkout@44c2b7a8a4ea60a981eaca3cf939b5f4305c123b # v4.1.5
    - name: Setup Go ${{ matrix.go }}
      uses: actions/setup-go@cdcb36043654635271a94b9a6d1392de5bb323a7 # v5.0.1
      with:
        go-version: ${{ matrix.go }}
    - name: Vendor go
      run: go mod vendor
    - name: Test Go (native)
      run: go test -v ./...
    - name: Install chrome
      uses: browser-actions/setup-chrome@latest
    - name: Install wasmbrowsertest
      run: go install github.com/agnivade/wasmbrowsertest@latest
    - name: Setup wasmexec
      run: mv $(go env GOPATH)/bin/wasmbrowsertest $(go env GOPATH)/bin/go_js_wasm_exec
    - name: Test with wasmbrowsertest
      env:
        GOOS: js
        GOARCH: wasm
      run: go test -v ./...
```
