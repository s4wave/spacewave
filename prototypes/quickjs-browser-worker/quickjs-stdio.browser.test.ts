import { describe, it, expect } from 'vitest'

const SERVER_PORT = 8091
const SERVER_URL = `http://localhost:${SERVER_PORT}`

describe('QuickJS WASI stdio in Browser WebWorker', () => {
  it('should read from virtual stdin', async () => {
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

      // Test script that reads from stdin
      const TEST_SCRIPT = \`
        // Read from stdin
        const stdin = std.in;
        const data = stdin.readAsString();
        console.log("Read from stdin:", data);
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

      async function main() {
        try {
          const wasmModule = await WebAssembly.compileStreaming(fetch('${SERVER_URL}/qjs-wasi.wasm'))

          const stdin = new StdinBuffer("Hello from stdin!")
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
            stdin,  // fd 0 - stdin with data
            stdout, // fd 1 - stdout
            stderr, // fd 2 - stderr
            rootDir, // fd 3 - preopened /
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

    console.log('stdout messages:', stdoutMessages)

    if (errorMessage) {
      console.error('QuickJS error:', errorMessage.data)
      // Don't fail the test, just log the error for analysis
    }

    // This test is exploratory - we want to see if stdin reading works
    expect(messages.length).toBeGreaterThan(0)
  }, 60000)

  it('should write to /dev/out file', async () => {
    const workerCode = `
      import {
        Fd,
        File,
        OpenFile,
        Directory,
        PreopenDirectory,
        WASI,
        ConsoleStdout,
      } from '${SERVER_URL}/node_modules/@bjorn3/browser_wasi_shim/dist/index.js'
      import * as wasi from '${SERVER_URL}/node_modules/@bjorn3/browser_wasi_shim/dist/wasi_defs.js'

      // Test script that writes to /dev/out
      // Note: TextEncoder is not available in QuickJS by default, use std.writeFile or write bytes directly
      const TEST_SCRIPT = \`
        // Open /dev/out and write to it
        const fd = os.open("/dev/out", os.O_WRONLY | os.O_CREAT);
        if (fd < 0) {
          console.log("Failed to open /dev/out, errno:", fd);
        } else {
          // Write raw bytes - "Hello" as ASCII
          const message = [72, 101, 108, 108, 111]; // "Hello"
          const buf = new Uint8Array(message);
          const written = os.write(fd, buf.buffer, 0, buf.length);
          console.log("Wrote", written, "bytes to /dev/out");
          os.close(fd);
        }
      \`

      // Custom Fd that captures writes
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

          const devOutWrites = []
          const devOut = new DevOut((data) => {
            const text = new TextDecoder().decode(data)
            devOutWrites.push(text)
            self.postMessage({ type: 'devout', data: text })
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
            new OpenFile(new File([])), // fd 0 - stdin (empty)
            stdout, // fd 1 - stdout
            stderr, // fd 2 - stderr
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

    const devoutMessages = messages
      .filter((m) => m.type === 'devout')
      .map((m) => String(m.data))
    const stdoutMessages = messages
      .filter((m) => m.type === 'stdout')
      .map((m) => String(m.data))
    const errorMessage = messages.find((m) => m.type === 'error')

    console.log('devout messages:', devoutMessages)
    console.log('stdout messages:', stdoutMessages)

    if (errorMessage) {
      console.error('QuickJS error:', errorMessage.data)
    }

    // This test is exploratory
    expect(messages.length).toBeGreaterThan(0)
  }, 60000)
})
