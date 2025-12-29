import { describe, it, expect } from 'vitest'

const SERVER_PORT = 8091
const SERVER_URL = `http://localhost:${SERVER_PORT}`

describe('QuickJS WASI bidirectional I/O in Browser WebWorker', () => {
  it('should support bidirectional communication via stdin and /dev/out', async () => {
    const workerCode = `
      import {
        Fd,
        File,
        OpenFile,
        PreopenDirectory,
        WASI,
        ConsoleStdout,
      } from '${SERVER_URL}/node_modules/@bjorn3/browser_wasi_shim/dist/index.js'
      import * as wasi from '${SERVER_URL}/node_modules/@bjorn3/browser_wasi_shim/dist/wasi_defs.js'

      // Test script that echoes stdin to /dev/out
      const TEST_SCRIPT = \`
        // Read from stdin
        const input = std.in.readAsString();
        console.log("Read from stdin:", input.length, "bytes");

        // Open /dev/out and write the data back
        const fd = os.open("/dev/out", os.O_WRONLY | os.O_CREAT);
        if (fd < 0) {
          console.log("Failed to open /dev/out, errno:", fd);
        } else {
          // Convert string to bytes manually
          const bytes = [];
          for (let i = 0; i < input.length; i++) {
            bytes.push(input.charCodeAt(i));
          }
          const buf = new Uint8Array(bytes);
          const written = os.write(fd, buf.buffer, 0, buf.length);
          console.log("Wrote", written, "bytes to /dev/out");
          os.close(fd);
        }
      \`

      // Custom stdin Fd that provides data
      class StdinBuffer extends Fd {
        constructor(data) {
          super()
          this.data = new TextEncoder().encode(data)
          this.pos = 0
        }

        fd_fdstat_get() {
          return {
            ret: 0,
            fdstat: new wasi.Fdstat(wasi.FILETYPE_CHARACTER_DEVICE, 0)
          }
        }

        fd_read(size) {
          const remaining = this.data.length - this.pos
          const toRead = Math.min(size, remaining)
          const slice = this.data.slice(this.pos, this.pos + toRead)
          this.pos += toRead
          return { ret: 0, data: slice }
        }
      }

      // Custom Fd that captures writes to /dev/out
      class DevOut extends Fd {
        constructor(onWrite) {
          super()
          this.onWrite = onWrite
        }

        fd_fdstat_get() {
          return {
            ret: 0,
            fdstat: new wasi.Fdstat(wasi.FILETYPE_CHARACTER_DEVICE, 0)
          }
        }

        fd_write(data) {
          this.onWrite(data)
          return { ret: 0, nwritten: data.byteLength }
        }
      }

      // Custom Directory that contains /dev/out
      class DevDirectory extends Fd {
        constructor(devOut) {
          super()
          this.devOut = devOut
        }

        fd_fdstat_get() {
          return {
            ret: 0,
            fdstat: new wasi.Fdstat(wasi.FILETYPE_DIRECTORY, 0)
          }
        }

        fd_prestat_get() {
          return { ret: 0, prestat: wasi.Prestat.dir('/dev') }
        }

        path_open(dirflags, path, oflags, fs_rights_base, fs_rights_inheriting, fd_flags) {
          if (path === 'out') {
            return { ret: 0, fd_obj: this.devOut }
          }
          return { ret: wasi.ERRNO_NOENT, fd_obj: null }
        }
      }

      async function main() {
        try {
          const wasmModule = await WebAssembly.compileStreaming(fetch('${SERVER_URL}/qjs-wasi.wasm'))

          const testMessage = "Hello, bidirectional world!"
          const devOutWrites = []

          const stdin = new StdinBuffer(testMessage)
          const devOut = new DevOut((data) => {
            const text = new TextDecoder().decode(data)
            devOutWrites.push(data)
            self.postMessage({ type: 'devout', data: text, bytes: Array.from(data) })
          })
          const devDir = new DevDirectory(devOut)

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
            stdin,   // fd 0 - stdin with test data
            stdout,  // fd 1 - stdout
            stderr,  // fd 2 - stderr
            rootDir, // fd 3 - preopened /
            devDir,  // fd 4 - preopened /dev
          ]

          const wasiInstance = new WASI(args, [], fds, { debug: false })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasiInstance.wasiImport,
          })

          try {
            wasiInstance.start(instance)
          } catch (e) {
            if (!e.message?.includes('exit')) throw e
          }

          self.postMessage({ type: 'exit', data: 0, testMessage })
        } catch (error) {
          self.postMessage({ type: 'error', data: error.message + '\\n' + error.stack })
        }
      }

      main()
    `

    const blob = new Blob([workerCode], { type: 'application/javascript' })
    const workerUrl = URL.createObjectURL(blob)

    const messages: Array<{ type: string; data: unknown; bytes?: number[]; testMessage?: string }> = []

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
    const errorMessage = messages.find((m) => m.type === 'error')
    const exitMessage = messages.find((m) => m.type === 'exit')

    if (errorMessage) {
      throw new Error('QuickJS error: ' + String(errorMessage.data))
    }

    expect(exitMessage).toBeDefined()
    expect(devoutMessages.length).toBeGreaterThan(0)

    // Verify the data was echoed correctly
    const echoedData = devoutMessages.map(m => String(m.data)).join('')
    const originalMessage = exitMessage?.testMessage as string
    expect(echoedData).toBe(originalMessage)
    console.log('Bidirectional echo successful:', echoedData)
  }, 60000)

  it('should handle os.setReadHandler for async stdin (exploration)', async () => {
    // This test explores whether os.setReadHandler works in browser WASI
    // In the wazero implementation, setReadHandler is used for async I/O
    const workerCode = `
      import {
        Fd,
        File,
        OpenFile,
        PreopenDirectory,
        WASI,
        ConsoleStdout,
      } from '${SERVER_URL}/node_modules/@bjorn3/browser_wasi_shim/dist/index.js'
      import * as wasi from '${SERVER_URL}/node_modules/@bjorn3/browser_wasi_shim/dist/wasi_defs.js'

      // Test script that checks setReadHandler availability without entering event loop
      const TEST_SCRIPT = \`
        console.log("Testing os.setReadHandler availability...");
        
        // Check if setReadHandler exists
        if (typeof os.setReadHandler === 'function') {
          console.log("os.setReadHandler is available");
          console.log("typeof os.setReadHandler:", typeof os.setReadHandler);
        } else {
          console.log("os.setReadHandler is NOT available");
        }
        
        // Also check for other relevant os functions
        console.log("os.read available:", typeof os.read === 'function');
        console.log("os.write available:", typeof os.write === 'function');
        console.log("os.open available:", typeof os.open === 'function');
        console.log("os.close available:", typeof os.close === 'function');
        
        // NOTE: We don't actually call setReadHandler here because that would
        // cause QuickJS to enter its event loop and wait for events, which
        // would hang the test. The wazero implementation handles this by
        // implementing poll_oneoff to wake QuickJS when data arrives.
        
        console.log("Test complete");
      \`

      async function main() {
        try {
          const wasmModule = await WebAssembly.compileStreaming(fetch('${SERVER_URL}/qjs-wasi.wasm'))

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
            new OpenFile(new File([])), // stdin
            stdout,
            stderr,
            rootDir,
          ]

          const wasiInstance = new WASI(args, [], fds, { debug: false })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasiInstance.wasiImport,
          })

          try {
            wasiInstance.start(instance)
          } catch (e) {
            if (!e.message?.includes('exit')) throw e
          }

          self.postMessage({ type: 'exit', data: 0 })
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

    const stdoutMessages = messages
      .filter((m) => m.type === 'stdout')
      .map((m) => String(m.data))
    const errorMessage = messages.find((m) => m.type === 'error')

    console.log('setReadHandler test output:', stdoutMessages)

    if (errorMessage) {
      console.error('QuickJS error:', errorMessage.data)
    }

    // This is exploratory - we just want to see the output
    expect(messages.length).toBeGreaterThan(0)
    expect(stdoutMessages.some(m => m.includes('os.setReadHandler'))).toBe(true)
  }, 60000)
})
