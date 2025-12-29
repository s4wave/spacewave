/**
 * WebWorker that loads and runs QuickJS WASI (qjs-wasi.wasm)
 *
 * This uses @bjorn3/browser_wasi_shim to provide WASI syscalls in the browser.
 */

import {
  File,
  OpenFile,
  PreopenDirectory,
  WASI,
  ConsoleStdout,
} from '/node_modules/@bjorn3/browser_wasi_shim/dist/index.js'

// Path to the QuickJS WASI binary (served by main.go)
const QUICKJS_WASM_URL = '/qjs-wasi.wasm'

// Test script to run
const TEST_SCRIPT = `
// Test QuickJS execution in browser
console.log("Hello from QuickJS in browser WebWorker!");
console.log("navigator.userAgent:", navigator.userAgent);

// Test generators (from existing prototype)
function* idGenerator() {
  let id = 1;
  while (true) {
    yield id++;
  }
}

const gen = idGenerator();
console.log("Generator test:");
console.log("  gen.next():", JSON.stringify(gen.next()));
console.log("  gen.next():", JSON.stringify(gen.next()));
console.log("  gen.next().value:", gen.next().value);

// Test async/await
async function asyncTest() {
  console.log("Async test starting...");
  // Note: setTimeout may not work in QuickJS WASI without polyfills
  // await new Promise(resolve => setTimeout(resolve, 100));
  console.log("Async test complete!");
  return 42;
}

asyncTest().then(result => {
  console.log("Async result:", result);
});

console.log("Script execution complete.");
`



async function main() {
  try {
    self.postMessage({ type: 'info', data: 'Loading QuickJS WASM...' })

    // Fetch the QuickJS WASM binary
    const wasmModule = await WebAssembly.compileStreaming(fetch(QUICKJS_WASM_URL))

    self.postMessage({ type: 'info', data: 'Setting up WASI environment...' })

    // Create stdout/stderr handlers that send messages to main thread
    const stdout = ConsoleStdout.lineBuffered((line) => {
      self.postMessage({ type: 'stdout', data: line })
    })
    const stderr = ConsoleStdout.lineBuffered((line) => {
      self.postMessage({ type: 'stderr', data: line })
    })

    // Create a virtual filesystem with our test script
    const rootDir = new PreopenDirectory('/', new Map([
      ['test.js', new File(new TextEncoder().encode(TEST_SCRIPT))],
    ]))

    // Setup WASI with:
    // - args: qjs-wasi.wasm --std /test.js
    // - env: empty
    // - fds: stdin (empty file), stdout, stderr, preopened root directory
    const args = ['qjs-wasi.wasm', '--std', '/test.js']
    const env = []
    const fds = [
      new OpenFile(new File([])), // stdin (empty)
      stdout, // stdout
      stderr, // stderr
      rootDir, // preopened /
    ]

    const wasi = new WASI(args, env, fds, { debug: false })

    self.postMessage({ type: 'info', data: 'Instantiating QuickJS...' })

    // Instantiate the WASM module with WASI imports
    const instance = await WebAssembly.instantiate(wasmModule, {
      wasi_snapshot_preview1: wasi.wasiImport,
    })

    self.postMessage({ type: 'ready', data: null })

    // Run the WASM module (calls _start)
    self.postMessage({ type: 'info', data: 'Starting QuickJS execution...' })

    let exitCode = 0
    try {
      exitCode = wasi.start(instance)
    } catch (e) {
      if (e.message?.includes('exit')) {
        // Normal exit
        exitCode = e.exit_code ?? 0
      } else {
        throw e
      }
    }

    self.postMessage({ type: 'exit', data: exitCode })
  } catch (error) {
    self.postMessage({ type: 'error', data: error.message + '\n' + error.stack })
    console.error('Worker error:', error)
  }
}

main()
