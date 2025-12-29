import { describe, it, expect } from 'vitest'

const SERVER_PORT = 8091
const SERVER_URL = `http://localhost:${SERVER_PORT}`

describe('QuickJS WASI in Browser WebWorker', () => {
  it('should load QuickJS WASM and execute JavaScript', async () => {
    // Create a worker that loads QuickJS
    const workerCode = `
      import {
        File,
        OpenFile,
        PreopenDirectory,
        WASI,
        ConsoleStdout,
      } from '${SERVER_URL}/node_modules/@bjorn3/browser_wasi_shim/dist/index.js'

      const TEST_SCRIPT = \`
        console.log("Hello from QuickJS!");
        console.log("1 + 2 =", 1 + 2);
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
          const env = []
          const fds = [
            new OpenFile(new File([])),
            stdout,
            stderr,
            rootDir,
          ]

          const wasi = new WASI(args, env, fds, { debug: false })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasi.wasiImport,
          })

          let exitCode = 0
          try {
            exitCode = wasi.start(instance)
          } catch (e) {
            if (e.message?.includes('exit')) {
              exitCode = e.exit_code ?? 0
            } else {
              throw e
            }
          }

          self.postMessage({ type: 'exit', data: exitCode })
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
        reject(new Error(`Worker error: ${e.message}`))
      }
    })

    await workerDone

    URL.revokeObjectURL(workerUrl)
    worker.terminate()

    // Verify we got expected output
    const stdoutMessages = messages
      .filter((m) => m.type === 'stdout')
      .map((m) => m.data)
    const exitMessage = messages.find((m) => m.type === 'exit')
    const errorMessage = messages.find((m) => m.type === 'error')

    if (errorMessage) {
      throw new Error(`QuickJS error: ${errorMessage.data}`)
    }

    expect(exitMessage).toBeDefined()
    expect(exitMessage?.data).toBe(0)

    // Check that QuickJS produced output
    expect(stdoutMessages.length).toBeGreaterThan(0)
    expect(stdoutMessages.some((m) => String(m).includes('Hello from QuickJS'))).toBe(true)
    expect(stdoutMessages.some((m) => String(m).includes('1 + 2 = 3'))).toBe(true)
  }, 60000)

  it('should support ES6+ features like generators', async () => {
    const workerCode = `
      import {
        File,
        OpenFile,
        PreopenDirectory,
        WASI,
        ConsoleStdout,
      } from '${SERVER_URL}/node_modules/@bjorn3/browser_wasi_shim/dist/index.js'

      const TEST_SCRIPT = \`
        function* idGen() {
          let id = 1;
          while (true) yield id++;
        }
        const gen = idGen();
        console.log("gen1:", gen.next().value);
        console.log("gen2:", gen.next().value);
        console.log("gen3:", gen.next().value);
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
            new OpenFile(new File([])),
            stdout,
            stderr,
            rootDir,
          ]

          const wasi = new WASI(args, [], fds, { debug: false })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasi.wasiImport,
          })

          try {
            wasi.start(instance)
          } catch (e) {
            if (!e.message?.includes('exit')) throw e
          }

          self.postMessage({ type: 'exit', data: 0 })
        } catch (error) {
          self.postMessage({ type: 'error', data: error.message })
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

        if (e.data.type === 'exit' || e.data.type === 'error') {
          window.clearTimeout(timeout)
          resolve()
        }
      }

      worker.onerror = (e) => {
        window.clearTimeout(timeout)
        reject(new Error(`Worker error: ${e.message}`))
      }
    })

    await workerDone

    URL.revokeObjectURL(workerUrl)
    worker.terminate()

    const stdoutMessages = messages
      .filter((m) => m.type === 'stdout')
      .map((m) => String(m.data))
    const errorMessage = messages.find((m) => m.type === 'error')

    if (errorMessage) {
      throw new Error(`QuickJS error: ${errorMessage.data}`)
    }

    expect(stdoutMessages.some((m) => m.includes('gen1: 1'))).toBe(true)
    expect(stdoutMessages.some((m) => m.includes('gen2: 2'))).toBe(true)
    expect(stdoutMessages.some((m) => m.includes('gen3: 3'))).toBe(true)
  }, 60000)
})
