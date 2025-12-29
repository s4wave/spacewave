import { describe, it, expect } from 'vitest'

const SERVER_PORT = 8091
const SERVER_URL = `http://localhost:${SERVER_PORT}`

// QuickJS test script that uses os.setReadHandler for async stdin
// Kept separate to avoid escaping issues in nested template literals
const QUICKJS_TEST_SCRIPT = `
console.log("Starting async stdin test...");

let received = "";
let readCount = 0;
const stdinFd = 0;
const readBuffer = new Uint8Array(1024);

// Set up a read handler for stdin
os.setReadHandler(stdinFd, function() {
  readCount++;
  const bytesRead = os.read(stdinFd, readBuffer.buffer, 0, readBuffer.length);
  console.log("Read handler called, bytes:", bytesRead);
  
  if (bytesRead > 0) {
    // Decode and accumulate the data
    const chunk = readBuffer.slice(0, bytesRead);
    let str = "";
    for (let i = 0; i < chunk.length; i++) {
      str += String.fromCharCode(chunk[i]);
    }
    received += str;
    console.log("Received so far:", received.length, "bytes");
    
    // Check if we received the terminator (newline)
    if (received.indexOf("\\n") >= 0) {
      console.log("Received complete message:", received.trim());
      
      // Write response to /dev/out
      const response = "ECHO:" + received.trim();
      const respBytes = [];
      for (let i = 0; i < response.length; i++) {
        respBytes.push(response.charCodeAt(i));
      }
      const respBuf = new Uint8Array(respBytes);
      
      const outFd = os.open("/dev/out", os.O_WRONLY);
      if (outFd >= 0) {
        os.write(outFd, respBuf.buffer, 0, respBuf.length);
        os.close(outFd);
        console.log("Response written to /dev/out");
      }
      
      // Clear the read handler and exit
      os.setReadHandler(stdinFd, null);
      console.log("Test complete, readCount:", readCount);
    }
  } else if (bytesRead === 0) {
    // No data available yet, will be called again when data arrives
  } else {
    console.log("Read error:", bytesRead);
  }
});

console.log("Read handler set, waiting for data...");
`

describe('QuickJS WASI async stdin with custom WASI shim', () => {
  it('should support os.setReadHandler for async stdin reading', async () => {
    // Encode the test script as base64 to avoid escaping issues
    const testScriptB64 = btoa(QUICKJS_TEST_SCRIPT)
    
    // This test uses our custom WASI shim with poll_oneoff fd support
    const workerCode = `
      // Import our bundled WASI shim (served by the Go server)
      const wasiShim = await import('${SERVER_URL}/wasi-shim.esm.js')
      const {
        WASI,
        File,
        PreopenDirectory,
        ConsoleStdout,
        PollableStdin,
        DevOut,
        DevDirectory,
      } = wasiShim

      // Decode the test script from base64
      const TEST_SCRIPT = atob('${testScriptB64}')

      async function main() {
        try {
          const wasmModule = await WebAssembly.compileStreaming(fetch('${SERVER_URL}/qjs-wasi.wasm'))

          // Create pollable stdin
          const stdin = new PollableStdin()
          
          // Create /dev/out for output
          const devOutWrites = []
          const devOut = new DevOut((data) => {
            const text = new TextDecoder().decode(data)
            devOutWrites.push(text)
            self.postMessage({ type: 'devout', data: text })
          })
          const devDir = new DevDirectory('/dev', new Map([['out', devOut]]))

          const stdout = ConsoleStdout.lineBuffered((line) => {
            self.postMessage({ type: 'stdout', data: line })
          })
          const stderr = ConsoleStdout.lineBuffered((line) => {
            self.postMessage({ type: 'stderr', data: line })
          })

          const rootDir = new PreopenDirectory('/', new Map([
            ['test.js', new File(new TextEncoder().encode(TEST_SCRIPT))],
          ]))

          const args = ['qjs-wasi.wasm', '--std', '/test.js']
          const fds = [
            stdin,   // fd 0 - pollable stdin
            stdout,  // fd 1 - stdout
            stderr,  // fd 2 - stderr
            rootDir, // fd 3 - preopened /
            devDir,  // fd 4 - preopened /dev
          ]

          const wasiInstance = new WASI(args, [], fds, { debug: false })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasiInstance.wasiImport,
          })

          // Push data to stdin BEFORE starting QuickJS
          // This simulates data already being available when the read handler is set
          self.postMessage({ type: 'info', data: 'Pushing data to stdin...' })
          const message = "Hello from async stdin test!\\n"
          stdin.push(new TextEncoder().encode(message))

          try {
            wasiInstance.start(instance)
          } catch (e) {
            if (!e.message?.includes('exit')) throw e
          }

          self.postMessage({ type: 'exit', data: 0, devOutWrites })
        } catch (error) {
          self.postMessage({ type: 'error', data: error.message + '\\n' + error.stack })
        }
      }

      main()
    `

    const blob = new Blob([workerCode], { type: 'application/javascript' })
    const workerUrl = URL.createObjectURL(blob)

    const messages: Array<{ type: string; data: unknown }> = []

    const worker = new Worker(workerUrl, { type: 'module' })

    const workerDone = new Promise<void>((resolve, reject) => {
      const timeout = window.setTimeout(() => {
        reject(new Error('Worker timed out'))
      }, 30000)

      worker.onmessage = (e) => {
        messages.push(e.data)
        console.log('[Worker Message]', e.data)

        if (e.data.type === 'exit' || e.data.type === 'error') {
          window.clearTimeout(timeout)
          resolve()
        }
      }

      worker.onerror = (e) => {
        window.clearTimeout(timeout)
        reject(new Error('Worker error: ' + e.message))
      }
    })

    await workerDone

    URL.revokeObjectURL(workerUrl)
    worker.terminate()

    const devoutMessages = messages.filter((m) => m.type === 'devout')
    const stdoutMessages = messages.filter((m) => m.type === 'stdout')
    const errorMessage = messages.find((m) => m.type === 'error')

    console.log('stdout:', stdoutMessages.map(m => m.data))
    console.log('devout:', devoutMessages.map(m => m.data))

    if (errorMessage) {
      throw new Error('QuickJS error: ' + String(errorMessage.data))
    }

    // Verify the async read handler was called and produced output
    expect(stdoutMessages.some(m => String(m.data).includes('Read handler called'))).toBe(true)
    expect(devoutMessages.length).toBeGreaterThan(0)
    expect(devoutMessages.some(m => String(m.data).includes('ECHO:'))).toBe(true)
  }, 60000)
})
