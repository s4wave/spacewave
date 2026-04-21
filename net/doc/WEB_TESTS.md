# Web Tests

Some tests in this project require internet access to perform HTTP requests to external services. These tests are controlled by the `webtests` build tag.

## Running Web Tests

To run tests that include web requests, use the following command:

```
go test -tags webtests ./...
```

This will enable the `webtests` build tag and run all tests, including those that make HTTP requests to external services.

### WebAssembly

This package can be tested in a browser environment using [`wasmbrowsertest`](https://github.com/agnivade/wasmbrowsertest).

1. Install `wasmbrowsertest`:

   ```bash
   go install github.com/agnivade/wasmbrowsertest@latest
   ```

2. Rename the `wasmbrowsertest` binary to `go_js_wasm_exec`:

   ```bash
   mv $(go env GOPATH)/bin/wasmbrowsertest $(go env GOPATH)/bin/go_js_wasm_exec
   ```

3. Run the tests with the `js` GOOS and `wasm` GOARCH:
   ```bash
   GOOS=js GOARCH=wasm go test -tags "webtests" -v ./...
   ```

This will compile the tests to WebAssembly and run them in a headless browser environment.

## Skipping Web Tests

By default, tests that require internet access are not run. To run all other tests without the web tests, simply run:

```
go test ./...
```

This will skip any tests that have the `webtests` build tag.

## Writing Web Tests

When writing tests that require internet access or make HTTP requests to external services, make sure to add the `webtests` build tag to the test file. For example:

```go
//go:build js && webtests

package mypackage

// Test code here
```

This ensures that these tests are only run when explicitly requested with the `webtests` build tag.
