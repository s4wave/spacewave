import { describe, it, expect } from 'vitest'

const SERVER_PORT = 8091
const SERVER_URL = `http://localhost:${SERVER_PORT}`

// A minimal plugin script that logs, then exits after a short delay
const MINIMAL_PLUGIN_SCRIPT = `
// Minimal plugin that exports a default function
export default async function main(backendAPI, abortSignal) {
  console.log("Plugin started!");
  console.log("backendAPI:", typeof backendAPI);
  console.log("abortSignal:", typeof abortSignal);
  console.log("Plugin initialization complete");
  
  // Exit cleanly after a short delay to allow test to verify output
  // In a real plugin, this would stay running and handle RPC calls
  globalThis.setTimeout(() => {
    console.log("Plugin exiting...");
    std.exit(0);
  }, 100);
}
`

describe('QuickJS Boot Harness Integration', () => {
  it('should load and verify boot harness is available', async () => {
    // First, verify the boot harness is being served
    const response = await fetch(`${SERVER_URL}/boot/plugin-quickjs.esm.js`)
    expect(response.ok).toBe(true)
    const content = await response.text()
    expect(content.length).toBeGreaterThan(1000)
    expect(content).toContain('eslint-disable')
    console.log('Boot harness size:', content.length, 'bytes')
  })

  it('should load boot harness in QuickJS and initialize plugin', async () => {
    // Encode the minimal plugin script as base64
    const pluginScriptB64 = btoa(MINIMAL_PLUGIN_SCRIPT)

    const workerCode = `
      // Import our bundled WASI shim
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

      async function main() {
        try {
          // Fetch the boot harness
          const bootHarnessResp = await fetch('${SERVER_URL}/boot/plugin-quickjs.esm.js')
          if (!bootHarnessResp.ok) {
            throw new Error('Failed to fetch boot harness: ' + bootHarnessResp.status)
          }
          const bootHarnessCode = await bootHarnessResp.text()
          self.postMessage({ type: 'info', data: 'Boot harness loaded: ' + bootHarnessCode.length + ' bytes' })

          // Decode the plugin script
          const pluginScript = atob('${pluginScriptB64}')

          // Load QuickJS WASM
          const wasmModule = await WebAssembly.compileStreaming(fetch('${SERVER_URL}/qjs-wasi.wasm'))

          // Create pollable stdin for yamux communication
          const stdin = new PollableStdin()
          
          // Create /dev/out for yamux output
          const devOutWrites = []
          const devOut = new DevOut((data) => {
            const text = new TextDecoder().decode(data)
            devOutWrites.push(data)
            self.postMessage({ type: 'devout', data: text, bytes: data.length })
          })
          const devDir = new DevDirectory('/dev', new Map([['out', devOut]]))

          const stdout = ConsoleStdout.lineBuffered((line) => {
            self.postMessage({ type: 'stdout', data: line })
          })
          const stderr = ConsoleStdout.lineBuffered((line) => {
            self.postMessage({ type: 'stderr', data: line })
          })

          // Create virtual filesystem with:
          // - /boot/plugin-quickjs.esm.js (the boot harness)
          // - /dist/plugin.mjs (the plugin script)
          const rootDir = new PreopenDirectory('/', new Map([
            ['boot', new wasiShim.Directory(new Map([
              ['plugin-quickjs.esm.js', new File(new TextEncoder().encode(bootHarnessCode))],
            ]))],
            ['dist', new wasiShim.Directory(new Map([
              ['plugin.mjs', new File(new TextEncoder().encode(pluginScript))],
            ]))],
          ]))

          const args = ['qjs-wasi.wasm', '--std', '/boot/plugin-quickjs.esm.js']
          const env = [
            'BLDR_SCRIPT_PATH=/dist/plugin.mjs',
            'BLDR_PLUGIN_START_INFO=', // Empty start info for now
          ]
          const fds = [
            stdin,   // fd 0 - pollable stdin
            stdout,  // fd 1 - stdout
            stderr,  // fd 2 - stderr
            rootDir, // fd 3 - preopened /
            devDir,  // fd 4 - preopened /dev
          ]

          const wasiInstance = new WASI(args, env, fds, { debug: false })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasiInstance.wasiImport,
          })

          self.postMessage({ type: 'info', data: 'Starting QuickJS with boot harness...' })

          try {
            wasiInstance.start(instance)
          } catch (e) {
            if (!e.message?.includes('exit')) {
              self.postMessage({ type: 'error', data: 'WASI error: ' + e.message + '\\n' + e.stack })
              return
            }
          }

          self.postMessage({ type: 'exit', data: 0, devOutWrites: devOutWrites.length })
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
      }, 60000)

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

    const stdoutMessages = messages.filter((m) => m.type === 'stdout')
    const stderrMessages = messages.filter((m) => m.type === 'stderr')
    const errorMessage = messages.find((m) => m.type === 'error')

    console.log('stdout:', stdoutMessages.map(m => m.data))
    console.log('stderr:', stderrMessages.map(m => m.data))

    if (errorMessage) {
      console.error('Error:', errorMessage.data)
      // Don't throw yet - let's see what errors we get
    }

    // Verify the plugin ran successfully
    expect(stdoutMessages.some(m => String(m.data).includes('Plugin started!'))).toBe(true)
    expect(stdoutMessages.some(m => String(m.data).includes('backendAPI: object'))).toBe(true)
    expect(stdoutMessages.some(m => String(m.data).includes('abortSignal: object'))).toBe(true)
    expect(stdoutMessages.some(m => String(m.data).includes('Plugin initialization complete'))).toBe(true)
    expect(stdoutMessages.some(m => String(m.data).includes('Plugin exiting...'))).toBe(true)
    
    // No errors should have occurred
    expect(stderrMessages.length).toBe(0)
    expect(errorMessage).toBeUndefined()
  }, 120000)
})
