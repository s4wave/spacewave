// quickjs-reactor.browser.test.ts - Browser tests for QuickJS WASI reactor model
//
// These tests verify the reactor model implementation works correctly in the browser.
// The reactor model uses qjs_init_argv, qjs_loop_once, and qjs_poll_io instead of
// the blocking command model (wasi.start).

import { describe, it, expect } from 'vitest'

const SERVER_PORT = 8091
const SERVER_URL = `http://localhost:${SERVER_PORT}`

// Helper to run a worker with reactor model code
async function runReactorWorker(workerCode: string, timeoutMs = 30000) {
  const blob = new Blob([workerCode], { type: 'application/javascript' })
  const workerUrl = URL.createObjectURL(blob)

  const messages: Array<{ type: string; data?: unknown; [key: string]: unknown }> = []
  const worker = new Worker(workerUrl, { type: 'module' })

  const workerDone = new Promise<void>((resolve, reject) => {
    const timeout = window.setTimeout(() => {
      reject(new Error('Worker timed out'))
    }, timeoutMs)

    worker.onmessage = (e) => {
      messages.push(e.data)
      console.log('[Worker]', e.data)

      if (e.data.type === 'exit' || e.data.type === 'error' || e.data.type === 'result') {
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

  return messages
}

describe('QuickJS WASI Reactor Model', () => {
  it('should execute basic JavaScript using reactor model', async () => {
    const testScript = `
      console.log("Hello from reactor!");
      console.log("1 + 2 =", 1 + 2);
      std.exit(0);
    `

    const workerCode = `
      const wasiShim = await import('${SERVER_URL}/wasi-shim.esm.js')
      const {
        WASI,
        WASIProcExit,
        File,
        PreopenDirectory,
        ConsoleStdout,
        PollableStdin,
        DevOut,
        DevDirectory,
      } = wasiShim

      const LOOP_ERROR = -2
      const LOOP_IDLE = -1

      async function main() {
        try {
          const wasmModule = await WebAssembly.compileStreaming(fetch('${SERVER_URL}/qjs-wasi.wasm'))

          const stdin = new PollableStdin()
          const devOut = new DevOut(() => {})
          const devDir = new DevDirectory('/dev', new Map([['out', devOut]]))

          const stdoutLines = []
          const stdout = ConsoleStdout.lineBuffered((line) => {
            stdoutLines.push(line)
            self.postMessage({ type: 'stdout', data: line })
          })
          const stderr = ConsoleStdout.lineBuffered((line) => {
            self.postMessage({ type: 'stderr', data: line })
          })

          const script = ${JSON.stringify(testScript)}
          const rootDir = new PreopenDirectory('/', new Map([
            ['test.js', new File(new TextEncoder().encode(script))],
          ]))

          const args = ['qjs', '--std', '/test.js']
          const fds = [stdin, stdout, stderr, rootDir, devDir]
          const wasi = new WASI(args, [], fds, { debug: false })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasi.wasiImport,
          })

          const exports = instance.exports

          // Initialize WASI reactor (not start!)
          wasi.initialize(instance)

          // Set up argv in memory
          const memory = exports.memory
          const ARGV_BASE = 65536
          const view = new DataView(memory.buffer)
          const bytes = new Uint8Array(memory.buffer)
          const encoder = new TextEncoder()

          let stringOffset = ARGV_BASE + args.length * 4 + 4
          for (let i = 0; i < args.length; i++) {
            view.setUint32(ARGV_BASE + i * 4, stringOffset, true)
            const encoded = encoder.encode(args[i])
            bytes.set(encoded, stringOffset)
            bytes[stringOffset + encoded.length] = 0
            stringOffset += encoded.length + 1
          }
          view.setUint32(ARGV_BASE + args.length * 4, 0, true)

          // Initialize QuickJS
          let running = true
          let exitCode = 0
          let iterations = 0

          try {
            const initResult = exports.qjs_init_argv(args.length, ARGV_BASE)
            if (initResult !== 0) {
              throw new Error('qjs_init_argv failed: ' + initResult)
            }
          } catch (e) {
            if (e instanceof WASIProcExit) {
              exitCode = e.code
              running = false
            } else {
              throw e
            }
          }

          // Run reactor event loop
          while (running && iterations < 10000) {
            iterations++

            let result
            try {
              result = exports.qjs_loop_once()
            } catch (e) {
              if (e instanceof WASIProcExit) {
                exitCode = e.code
                running = false
                break
              }
              throw e
            }

            if (result === LOOP_ERROR) {
              throw new Error('JavaScript error in reactor')
            }

            if (result === 0) {
              // More microtasks - continue immediately
              continue
            }

            if (result > 0) {
              // Timer - wait
              await new Promise(r => setTimeout(r, result))
              continue
            }

            if (result === LOOP_IDLE) {
              // Idle - exit for this simple test
              break
            }
          }

          self.postMessage({
            type: 'exit',
            exitCode,
            iterations,
            stdoutLines,
          })
        } catch (error) {
          self.postMessage({ type: 'error', data: error.message + '\\n' + error.stack })
        }
      }

      main()
    `

    const messages = await runReactorWorker(workerCode)

    const exitMsg = messages.find((m) => m.type === 'exit') as {
      exitCode?: number
      iterations?: number
      stdoutLines?: string[]
    } | undefined
    const errorMsg = messages.find((m) => m.type === 'error')
    const stdoutMsgs = messages.filter((m) => m.type === 'stdout')

    expect(errorMsg).toBeUndefined()
    expect(exitMsg).toBeDefined()
    expect(exitMsg?.exitCode).toBe(0)
    expect(stdoutMsgs.some((m) => String(m.data).includes('Hello from reactor'))).toBe(true)
    expect(stdoutMsgs.some((m) => String(m.data).includes('1 + 2 = 3'))).toBe(true)

    console.log('Reactor iterations:', exitMsg?.iterations)
  }, 60000)

  it('should handle timers using reactor model', async () => {
    const testScript = `
      console.log("Timer test start");
      
      let count = 0;
      function tick() {
        count++;
        console.log("Tick", count);
        if (count >= 3) {
          console.log("Timer test complete");
          std.exit(0);
        } else {
          os.setTimeout(tick, 50);
        }
      }
      os.setTimeout(tick, 50);
    `

    const workerCode = `
      const wasiShim = await import('${SERVER_URL}/wasi-shim.esm.js')
      const {
        WASI,
        WASIProcExit,
        File,
        PreopenDirectory,
        ConsoleStdout,
        PollableStdin,
        DevOut,
        DevDirectory,
      } = wasiShim

      const LOOP_ERROR = -2
      const LOOP_IDLE = -1

      async function main() {
        try {
          const wasmModule = await WebAssembly.compileStreaming(fetch('${SERVER_URL}/qjs-wasi.wasm'))

          const stdin = new PollableStdin()
          const devOut = new DevOut(() => {})
          const devDir = new DevDirectory('/dev', new Map([['out', devOut]]))

          const stdoutLines = []
          const stdout = ConsoleStdout.lineBuffered((line) => {
            stdoutLines.push(line)
            self.postMessage({ type: 'stdout', data: line })
          })
          const stderr = ConsoleStdout.lineBuffered((line) => {
            self.postMessage({ type: 'stderr', data: line })
          })

          const script = ${JSON.stringify(testScript)}
          const rootDir = new PreopenDirectory('/', new Map([
            ['test.js', new File(new TextEncoder().encode(script))],
          ]))

          const args = ['qjs', '--std', '/test.js']
          const fds = [stdin, stdout, stderr, rootDir, devDir]
          const wasi = new WASI(args, [], fds, { debug: false })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasi.wasiImport,
          })

          const exports = instance.exports
          wasi.initialize(instance)

          const memory = exports.memory
          const ARGV_BASE = 65536
          const view = new DataView(memory.buffer)
          const bytes = new Uint8Array(memory.buffer)
          const encoder = new TextEncoder()

          let stringOffset = ARGV_BASE + args.length * 4 + 4
          for (let i = 0; i < args.length; i++) {
            view.setUint32(ARGV_BASE + i * 4, stringOffset, true)
            const encoded = encoder.encode(args[i])
            bytes.set(encoded, stringOffset)
            bytes[stringOffset + encoded.length] = 0
            stringOffset += encoded.length + 1
          }
          view.setUint32(ARGV_BASE + args.length * 4, 0, true)

          let running = true
          let exitCode = 0
          let iterations = 0
          const startTime = Date.now()

          try {
            const initResult = exports.qjs_init_argv(args.length, ARGV_BASE)
            if (initResult !== 0) throw new Error('qjs_init_argv failed')
          } catch (e) {
            if (e instanceof WASIProcExit) {
              exitCode = e.code
              running = false
            } else {
              throw e
            }
          }

          while (running && iterations < 10000 && Date.now() - startTime < 10000) {
            iterations++

            let result
            try {
              result = exports.qjs_loop_once()
            } catch (e) {
              if (e instanceof WASIProcExit) {
                exitCode = e.code
                running = false
                break
              }
              throw e
            }

            if (result === LOOP_ERROR) throw new Error('JavaScript error')
            if (result === 0) continue
            if (result > 0) {
              await new Promise(r => setTimeout(r, Math.min(result, 100)))
              continue
            }
            if (result === LOOP_IDLE) {
              await new Promise(r => setTimeout(r, 10))
              continue
            }
          }

          self.postMessage({
            type: 'exit',
            exitCode,
            iterations,
            stdoutLines,
          })
        } catch (error) {
          self.postMessage({ type: 'error', data: error.message + '\\n' + error.stack })
        }
      }

      main()
    `

    const messages = await runReactorWorker(workerCode)

    const exitMsg = messages.find((m) => m.type === 'exit') as {
      exitCode?: number
      stdoutLines?: string[]
    } | undefined
    const errorMsg = messages.find((m) => m.type === 'error')

    expect(errorMsg).toBeUndefined()
    expect(exitMsg).toBeDefined()
    expect(exitMsg?.exitCode).toBe(0)

    const stdoutLines = exitMsg?.stdoutLines || []
    expect(stdoutLines.some((l) => l.includes('Timer test start'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('Tick 1'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('Tick 2'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('Tick 3'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('Timer test complete'))).toBe(true)
  }, 60000)

  it('should handle async stdin with os.setReadHandler using qjs_poll_io', async () => {
    const testScript = `
      console.log("Async stdin test start");

      const stdinFd = 0;
      const readBuffer = new Uint8Array(1024);
      let received = "";

      os.setReadHandler(stdinFd, function() {
        const bytesRead = os.read(stdinFd, readBuffer.buffer, 0, readBuffer.length);
        console.log("Read handler called, bytes:", bytesRead);
        
        if (bytesRead > 0) {
          let str = "";
          for (let i = 0; i < bytesRead; i++) {
            str += String.fromCharCode(readBuffer[i]);
          }
          received += str;
          console.log("Received:", received);
          
          if (received.includes("\\n")) {
            console.log("Complete message received");
            os.setReadHandler(stdinFd, null);
            
            // Write response to /dev/out
            const response = "ECHO:" + received.trim();
            const outFd = os.open("/dev/out", os.O_WRONLY);
            if (outFd >= 0) {
              const respBytes = new Uint8Array(response.length);
              for (let i = 0; i < response.length; i++) {
                respBytes[i] = response.charCodeAt(i);
              }
              os.write(outFd, respBytes.buffer, 0, respBytes.length);
              os.close(outFd);
              console.log("Response written");
            }
            
            std.exit(0);
          }
        }
      });

      console.log("Read handler set, waiting...");
    `

    const workerCode = `
      const wasiShim = await import('${SERVER_URL}/wasi-shim.esm.js')
      const {
        WASI,
        WASIProcExit,
        File,
        PreopenDirectory,
        ConsoleStdout,
        PollableStdin,
        DevOut,
        DevDirectory,
      } = wasiShim

      const LOOP_ERROR = -2
      const LOOP_IDLE = -1

      async function main() {
        try {
          const wasmModule = await WebAssembly.compileStreaming(fetch('${SERVER_URL}/qjs-wasi.wasm'))

          const stdin = new PollableStdin()
          
          const devOutChunks = []
          const devOut = new DevOut((data) => {
            devOutChunks.push(new Uint8Array(data))
            self.postMessage({ type: 'devout', data: new TextDecoder().decode(data) })
          })
          const devDir = new DevDirectory('/dev', new Map([['out', devOut]]))

          const stdoutLines = []
          const stdout = ConsoleStdout.lineBuffered((line) => {
            stdoutLines.push(line)
            self.postMessage({ type: 'stdout', data: line })
          })
          const stderr = ConsoleStdout.lineBuffered((line) => {
            self.postMessage({ type: 'stderr', data: line })
          })

          const script = ${JSON.stringify(testScript)}
          const rootDir = new PreopenDirectory('/', new Map([
            ['test.js', new File(new TextEncoder().encode(script))],
          ]))

          const args = ['qjs', '--std', '/test.js']
          const fds = [stdin, stdout, stderr, rootDir, devDir]
          const wasi = new WASI(args, [], fds, { debug: false })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasi.wasiImport,
          })

          const exports = instance.exports
          wasi.initialize(instance)

          const memory = exports.memory
          const ARGV_BASE = 65536
          const view = new DataView(memory.buffer)
          const bytes = new Uint8Array(memory.buffer)
          const encoder = new TextEncoder()

          let stringOffset = ARGV_BASE + args.length * 4 + 4
          for (let i = 0; i < args.length; i++) {
            view.setUint32(ARGV_BASE + i * 4, stringOffset, true)
            const encoded = encoder.encode(args[i])
            bytes.set(encoded, stringOffset)
            bytes[stringOffset + encoded.length] = 0
            stringOffset += encoded.length + 1
          }
          view.setUint32(ARGV_BASE + args.length * 4, 0, true)

          const initResult = exports.qjs_init_argv(args.length, ARGV_BASE)
          if (initResult !== 0) throw new Error('qjs_init_argv failed')

          // Push stdin data after a short delay (simulating async I/O)
          setTimeout(() => {
            self.postMessage({ type: 'info', data: 'Pushing stdin data...' })
            stdin.push(new TextEncoder().encode("Hello from stdin test!\\n"))
          }, 100)

          let running = true
          let exitCode = 0
          let iterations = 0
          const startTime = Date.now()

          while (running && iterations < 10000 && Date.now() - startTime < 10000) {
            iterations++

            let result
            try {
              result = exports.qjs_loop_once()
            } catch (e) {
              if (e instanceof WASIProcExit) {
                exitCode = e.code
                running = false
                break
              }
              throw e
            }

            if (result === LOOP_ERROR) throw new Error('JavaScript error')
            if (result === 0) continue
            
            if (result > 0) {
              // Timer pending - but check stdin first
              if (stdin.hasData()) {
                try {
                  exports.qjs_poll_io(0)
                } catch (e) {
                  if (e instanceof WASIProcExit) {
                    exitCode = e.code
                    running = false
                    break
                  }
                  throw e
                }
                continue
              }
              await new Promise(r => setTimeout(r, Math.min(result, 50)))
              continue
            }
            
            if (result === LOOP_IDLE) {
              // Idle - check stdin
              if (stdin.hasData()) {
                try {
                  exports.qjs_poll_io(0)
                } catch (e) {
                  if (e instanceof WASIProcExit) {
                    exitCode = e.code
                    running = false
                    break
                  }
                  throw e
                }
                continue
              }
              await new Promise(r => setTimeout(r, 10))
              continue
            }
          }

          self.postMessage({
            type: 'exit',
            exitCode,
            iterations,
            stdoutLines,
            devOutChunks: devOutChunks.length,
          })
        } catch (error) {
          self.postMessage({ type: 'error', data: error.message + '\\n' + error.stack })
        }
      }

      main()
    `

    const messages = await runReactorWorker(workerCode)

    const exitMsg = messages.find((m) => m.type === 'exit') as {
      exitCode?: number
      stdoutLines?: string[]
      devOutChunks?: number
    } | undefined
    const errorMsg = messages.find((m) => m.type === 'error')
    const devoutMsgs = messages.filter((m) => m.type === 'devout')

    if (errorMsg) {
      console.error('Error:', errorMsg.data)
    }

    expect(errorMsg).toBeUndefined()
    expect(exitMsg).toBeDefined()
    expect(exitMsg?.exitCode).toBe(0)

    const stdoutLines = exitMsg?.stdoutLines || []
    expect(stdoutLines.some((l) => l.includes('Async stdin test start'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('Read handler called'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('Complete message received'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('Response written'))).toBe(true)

    // Verify /dev/out received the echo response
    expect(devoutMsgs.length).toBeGreaterThan(0)
    expect(devoutMsgs.some((m) => String(m.data).includes('ECHO:'))).toBe(true)
  }, 60000)

  it('should run boot harness with plugin using reactor model', async () => {
    const pluginScript = `
      export default async function main(backendAPI, abortSignal) {
        console.log("Plugin started!");
        console.log("backendAPI:", typeof backendAPI);
        console.log("abortSignal:", typeof abortSignal);
        console.log("startInfo.pluginId:", backendAPI.startInfo?.pluginId);
        console.log("Plugin complete");
        
        globalThis.setTimeout(() => {
          std.exit(0);
        }, 50);
      }
    `

    const workerCode = `
      const wasiShim = await import('${SERVER_URL}/wasi-shim.esm.js')
      const {
        WASI,
        WASIProcExit,
        File,
        Directory,
        PreopenDirectory,
        ConsoleStdout,
        PollableStdin,
        DevOut,
        DevDirectory,
      } = wasiShim

      const LOOP_ERROR = -2
      const LOOP_IDLE = -1

      async function main() {
        try {
          // Fetch boot harness
          const bootResp = await fetch('${SERVER_URL}/boot/plugin-quickjs.esm.js')
          if (!bootResp.ok) throw new Error('Failed to fetch boot harness')
          const bootHarness = await bootResp.text()
          self.postMessage({ type: 'info', data: 'Boot harness: ' + bootHarness.length + ' bytes' })

          const wasmModule = await WebAssembly.compileStreaming(fetch('${SERVER_URL}/qjs-wasi.wasm'))

          const stdin = new PollableStdin()
          
          const devOutChunks = []
          const devOut = new DevOut((data) => {
            devOutChunks.push(new Uint8Array(data))
          })
          const devDir = new DevDirectory('/dev', new Map([['out', devOut]]))

          const stdoutLines = []
          const stdout = ConsoleStdout.lineBuffered((line) => {
            stdoutLines.push(line)
            self.postMessage({ type: 'stdout', data: line })
          })
          const stderr = ConsoleStdout.lineBuffered((line) => {
            self.postMessage({ type: 'stderr', data: line })
          })

          const pluginScript = ${JSON.stringify(pluginScript)}
          
          // Create filesystem with boot harness and plugin
          const rootDir = new PreopenDirectory('/', new Map([
            ['boot', new Directory(new Map([
              ['plugin-quickjs.esm.js', new File(new TextEncoder().encode(bootHarness))],
            ]))],
            ['dist', new Directory(new Map([
              ['plugin.mjs', new File(new TextEncoder().encode(pluginScript))],
            ]))],
          ]))

          const startInfo = { pluginId: 'test-plugin', instanceId: 'test-123' }
          const startInfoB64 = btoa(JSON.stringify(startInfo))

          const args = ['qjs', '--std', '/boot/plugin-quickjs.esm.js']
          const env = [
            'BLDR_SCRIPT_PATH=/dist/plugin.mjs',
            'BLDR_PLUGIN_START_INFO=' + startInfoB64,
          ]
          const fds = [stdin, stdout, stderr, rootDir, devDir]
          const wasi = new WASI(args, env, fds, { debug: false })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasi.wasiImport,
          })

          const exports = instance.exports
          wasi.initialize(instance)

          const memory = exports.memory
          const ARGV_BASE = 65536
          const view = new DataView(memory.buffer)
          const bytes = new Uint8Array(memory.buffer)
          const encoder = new TextEncoder()

          let stringOffset = ARGV_BASE + args.length * 4 + 4
          for (let i = 0; i < args.length; i++) {
            view.setUint32(ARGV_BASE + i * 4, stringOffset, true)
            const encoded = encoder.encode(args[i])
            bytes.set(encoded, stringOffset)
            bytes[stringOffset + encoded.length] = 0
            stringOffset += encoded.length + 1
          }
          view.setUint32(ARGV_BASE + args.length * 4, 0, true)

          const initResult = exports.qjs_init_argv(args.length, ARGV_BASE)
          if (initResult !== 0) throw new Error('qjs_init_argv failed')

          let running = true
          let exitCode = 0
          let iterations = 0
          const startTime = Date.now()

          while (running && iterations < 10000 && Date.now() - startTime < 10000) {
            iterations++

            let result
            try {
              result = exports.qjs_loop_once()
            } catch (e) {
              if (e instanceof WASIProcExit) {
                exitCode = e.code
                running = false
                break
              }
              throw e
            }

            if (result === LOOP_ERROR) throw new Error('JavaScript error')
            if (result === 0) continue
            if (result > 0) {
              if (stdin.hasData()) {
                try { exports.qjs_poll_io(0) } catch (e) {
                  if (e instanceof WASIProcExit) { exitCode = e.code; running = false; break }
                  throw e
                }
                continue
              }
              await new Promise(r => setTimeout(r, Math.min(result, 50)))
              continue
            }
            if (result === LOOP_IDLE) {
              if (stdin.hasData()) {
                try { exports.qjs_poll_io(0) } catch (e) {
                  if (e instanceof WASIProcExit) { exitCode = e.code; running = false; break }
                  throw e
                }
                continue
              }
              await new Promise(r => setTimeout(r, 10))
              continue
            }
          }

          self.postMessage({
            type: 'exit',
            exitCode,
            iterations,
            stdoutLines,
            devOutChunks: devOutChunks.length,
          })
        } catch (error) {
          self.postMessage({ type: 'error', data: error.message + '\\n' + error.stack })
        }
      }

      main()
    `

    const messages = await runReactorWorker(workerCode, 120000)

    const exitMsg = messages.find((m) => m.type === 'exit') as {
      exitCode?: number
      stdoutLines?: string[]
    } | undefined
    const errorMsg = messages.find((m) => m.type === 'error')
    const stderrMsgs = messages.filter((m) => m.type === 'stderr')

    if (errorMsg) {
      console.error('Error:', errorMsg.data)
    }
    if (stderrMsgs.length > 0) {
      console.error('Stderr:', stderrMsgs.map((m) => m.data))
    }

    expect(errorMsg).toBeUndefined()
    expect(stderrMsgs.length).toBe(0)
    expect(exitMsg).toBeDefined()
    expect(exitMsg?.exitCode).toBe(0)

    const stdoutLines = exitMsg?.stdoutLines || []
    console.log('stdout:', stdoutLines)

    expect(stdoutLines.some((l) => l.includes('Plugin started!'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('backendAPI: object'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('abortSignal: object'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('startInfo.pluginId: test-plugin'))).toBe(true)
    expect(stdoutLines.some((l) => l.includes('Plugin complete'))).toBe(true)
  }, 120000)
})
