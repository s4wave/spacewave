import { describe, it, expect } from 'vitest'

const SERVER_PORT = 8091
const SERVER_URL = `http://localhost:${SERVER_PORT}`

// A test plugin that verifies backendAPI is functional and exercises yamux
const TEST_PLUGIN_WITH_RPC = `
// Test plugin that verifies backendAPI is functional
export default async function main(backendAPI, abortSignal) {
  console.log("Plugin started!");
  console.log("backendAPI type:", typeof backendAPI);
  console.log("abortSignal type:", typeof abortSignal);
  
  // Verify backendAPI has expected properties
  console.log("has openStream:", typeof backendAPI.openStream === 'function');
  console.log("has startInfo:", typeof backendAPI.startInfo === 'object');
  console.log("startInfo.pluginId:", backendAPI.startInfo?.pluginId);
  console.log("startInfo.instanceId:", backendAPI.startInfo?.instanceId);
  
  // The yamux connection is established by the boot harness before we get here
  // We can verify it worked by checking that we got a valid backendAPI
  console.log("YAMUX_READY");
  
  // Keep plugin alive briefly
  await new Promise(resolve => globalThis.setTimeout(resolve, 200));
  
  console.log("Plugin exiting cleanly");
  std.exit(0);
}
`

describe('shw-quickjs E2E Integration', () => {
  it('should run plugin with boot harness and verify yamux output', async () => {
    // This test verifies the same pattern used in shw-quickjs.ts:
    // 1. Load QuickJS WASM and boot harness
    // 2. Set up stdin/devout (yamux I/O channels)
    // 3. Run the plugin and verify yamux header is written to /dev/out
    // 4. Verify backendAPI is provided to the plugin

    const pluginScriptB64 = btoa(TEST_PLUGIN_WITH_RPC)

    const workerCode = `
      const wasiShim = await import('${SERVER_URL}/wasi-shim.esm.js')
      const {
        WASI,
        File,
        Directory,
        PreopenDirectory,
        ConsoleStdout,
        PollableStdin,
        DevOut,
        DevDirectory,
      } = wasiShim

      async function main() {
        try {
          // Fetch the boot harness (same as shw-quickjs.ts)
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

          // Create pollable stdin for yamux communication (host → plugin)
          const stdin = new PollableStdin()
          
          // Collect all bytes written to /dev/out (yamux output from boot harness)
          const devOutChunks = []
          let totalDevOutBytes = 0
          const devOut = new DevOut((data) => {
            const copy = new Uint8Array(data)
            devOutChunks.push(copy)
            totalDevOutBytes += copy.length
            self.postMessage({ 
              type: 'devout', 
              chunkSize: copy.length,
              totalBytes: totalDevOutBytes,
              // Show first few bytes as hex for debugging
              preview: Array.from(copy.slice(0, 16)).map(b => b.toString(16).padStart(2, '0')).join(' ')
            })
          })
          const devDir = new DevDirectory('/dev', new Map([['out', devOut]]))

          const stdoutMessages = []
          const stdout = ConsoleStdout.lineBuffered((line) => {
            stdoutMessages.push(line)
            self.postMessage({ type: 'stdout', data: line })
          })
          const stderr = ConsoleStdout.lineBuffered((line) => {
            self.postMessage({ type: 'stderr', data: line })
          })

          // Create virtual filesystem
          const rootDir = new PreopenDirectory('/', new Map([
            ['boot', new Directory(new Map([
              ['plugin-quickjs.esm.js', new File(new TextEncoder().encode(bootHarnessCode))],
            ]))],
            ['dist', new Directory(new Map([
              ['plugin.mjs', new File(new TextEncoder().encode(pluginScript))],
            ]))],
          ]))

          // Create start info (like WebQuickJSHost does)
          const startInfo = { pluginId: 'test-plugin', instanceId: 'test-instance-123' }
          const startInfoB64 = btoa(JSON.stringify(startInfo))

          const args = ['qjs-wasi.wasm', '--std', '/boot/plugin-quickjs.esm.js']
          const env = [
            'BLDR_SCRIPT_PATH=/dist/plugin.mjs',
            'BLDR_PLUGIN_START_INFO=' + startInfoB64,
          ]
          const fds = [
            stdin,   // fd 0 - pollable stdin
            stdout,  // fd 1 - stdout
            stderr,  // fd 2 - stderr
            rootDir, // fd 3 - preopened /
            devDir,  // fd 4 - preopened /dev
          ]

          const wasiInstance = new WASI(args, env, fds, { debug: false })

          self.postMessage({ type: 'info', data: 'Starting QuickJS...' })

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasiInstance.wasiImport,
          })

          try {
            wasiInstance.start(instance)
          } catch (e) {
            if (!e.message?.includes('exit')) {
              self.postMessage({ type: 'error', data: 'WASI error: ' + e.message + '\\n' + e.stack })
              return
            }
          }

          // Analyze yamux header from first chunk
          // Yamux header format (12 bytes):
          // - Version (1 byte): should be 0
          // - Type (1 byte): 0=Data, 1=WindowUpdate, 2=Ping, 3=GoAway  
          // - Flags (2 bytes, big-endian)
          // - Stream ID (4 bytes, big-endian)
          // - Length (4 bytes, big-endian)
          let yamuxInfo = null
          if (devOutChunks.length > 0) {
            const firstBytes = devOutChunks[0]
            if (firstBytes.length >= 12) {
              yamuxInfo = {
                version: firstBytes[0],
                type: firstBytes[1],
                flags: (firstBytes[2] << 8) | firstBytes[3],
                streamId: (firstBytes[4] << 24) | (firstBytes[5] << 16) | (firstBytes[6] << 8) | firstBytes[7],
                length: (firstBytes[8] << 24) | (firstBytes[9] << 16) | (firstBytes[10] << 8) | firstBytes[11],
                headerHex: Array.from(firstBytes.slice(0, 12)).map(b => b.toString(16).padStart(2, '0')).join(' ')
              }
            }
          }

          self.postMessage({ 
            type: 'exit', 
            exitCode: 0,
            stdoutMessages,
            devOutChunks: devOutChunks.length,
            totalDevOutBytes,
            yamuxInfo,
            // Check for key indicators
            hasYamuxOutput: totalDevOutBytes >= 12,
            pluginStarted: stdoutMessages.some(m => m.includes('Plugin started!')),
            yamuxReady: stdoutMessages.some(m => m.includes('YAMUX_READY')),
          })
        } catch (error) {
          self.postMessage({ type: 'error', data: error.message + '\\n' + error.stack })
        }
      }

      main()
    `

    const blob = new Blob([workerCode], { type: 'application/javascript' })
    const workerUrl = URL.createObjectURL(blob)

    const messages: Array<{
      type: string
      data?: unknown
      [key: string]: unknown
    }> = []

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
    const devoutMessages = messages.filter((m) => m.type === 'devout')
    const errorMessage = messages.find((m) => m.type === 'error')
    const exitMessage = messages.find((m) => m.type === 'exit') as
      | {
          type: string
          stdoutMessages?: string[]
          yamuxInfo?: { version: number; type: number; headerHex: string }
          hasYamuxOutput?: boolean
          pluginStarted?: boolean
          yamuxReady?: boolean
        }
      | undefined

    console.log(
      'stdout:',
      stdoutMessages.map((m) => m.data),
    )
    console.log('devout chunks:', devoutMessages.length)
    console.log('exit:', exitMessage)

    if (errorMessage) {
      console.error('Error:', errorMessage.data)
    }

    // Verify no errors
    expect(stderrMessages.length).toBe(0)
    expect(errorMessage).toBeUndefined()

    // Verify the plugin ran successfully
    expect(exitMessage).toBeDefined()
    expect(exitMessage?.pluginStarted).toBe(true)
    expect(exitMessage?.yamuxReady).toBe(true)

    // Verify yamux output was written (boot harness establishes yamux connection)
    expect(exitMessage?.hasYamuxOutput).toBe(true)
    expect(devoutMessages.length).toBeGreaterThan(0)

    // Verify yamux header format
    expect(exitMessage?.yamuxInfo).toBeDefined()
    expect(exitMessage?.yamuxInfo?.version).toBe(0) // Yamux version 0
    console.log('Yamux header:', exitMessage?.yamuxInfo?.headerHex)

    // Verify startInfo was passed correctly (check stdout for the values we logged)
    expect(
      stdoutMessages.some((m) =>
        String(m.data).includes('startInfo.pluginId: test-plugin'),
      ),
    ).toBe(true)
    expect(
      stdoutMessages.some((m) =>
        String(m.data).includes('startInfo.instanceId: test-instance-123'),
      ),
    ).toBe(true)
  }, 120000)

  it('should pass startInfo to plugin correctly', async () => {
    // This test verifies that the BLDR_PLUGIN_START_INFO environment variable
    // is correctly parsed and passed to the plugin via backendAPI.startInfo

    const pluginScriptB64 = btoa(`
export default async function main(backendAPI, abortSignal) {
  // Log all startInfo fields
  const info = backendAPI.startInfo || {}
  console.log("START_INFO_CHECK");
  console.log("pluginId=" + (info.pluginId || "MISSING"));
  console.log("instanceId=" + (info.instanceId || "MISSING"));
  console.log("START_INFO_END");
  
  globalThis.setTimeout(() => std.exit(0), 50);
}
`)

    const workerCode = `
      const wasiShim = await import('${SERVER_URL}/wasi-shim.esm.js')
      const {
        WASI,
        File,
        Directory,
        PreopenDirectory,
        ConsoleStdout,
        PollableStdin,
        DevOut,
        DevDirectory,
      } = wasiShim

      async function main() {
        try {
          const bootHarnessResp = await fetch('${SERVER_URL}/boot/plugin-quickjs.esm.js')
          const bootHarnessCode = await bootHarnessResp.text()
          const pluginScript = atob('${pluginScriptB64}')
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

          const rootDir = new PreopenDirectory('/', new Map([
            ['boot', new Directory(new Map([
              ['plugin-quickjs.esm.js', new File(new TextEncoder().encode(bootHarnessCode))],
            ]))],
            ['dist', new Directory(new Map([
              ['plugin.mjs', new File(new TextEncoder().encode(pluginScript))],
            ]))],
          ]))

          // Use specific test values for startInfo
          const startInfo = { 
            pluginId: 'my-test-plugin-id', 
            instanceId: 'instance-abc-123' 
          }
          const startInfoB64 = btoa(JSON.stringify(startInfo))

          const fds = [stdin, stdout, stderr, rootDir, devDir]
          const wasiInstance = new WASI(
            ['qjs-wasi.wasm', '--std', '/boot/plugin-quickjs.esm.js'],
            ['BLDR_SCRIPT_PATH=/dist/plugin.mjs', 'BLDR_PLUGIN_START_INFO=' + startInfoB64],
            fds,
            { debug: false }
          )

          const instance = await WebAssembly.instantiate(wasmModule, {
            wasi_snapshot_preview1: wasiInstance.wasiImport,
          })

          try {
            wasiInstance.start(instance)
          } catch (e) {
            if (!e.message?.includes('exit')) throw e
          }

          self.postMessage({ 
            type: 'exit', 
            stdoutLines,
          })
        } catch (error) {
          self.postMessage({ type: 'error', data: error.message + '\\n' + error.stack })
        }
      }

      main()
    `

    const blob = new Blob([workerCode], { type: 'application/javascript' })
    const workerUrl = URL.createObjectURL(blob)
    const messages: Array<{ type: string; [key: string]: unknown }> = []
    const worker = new Worker(workerUrl, { type: 'module' })

    const workerDone = new Promise<void>((resolve, reject) => {
      const timeout = window.setTimeout(
        () => reject(new Error('Timeout')),
        60000,
      )
      worker.onmessage = (e) => {
        messages.push(e.data)
        console.log('[Worker]', e.data)
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

    const exitMsg = messages.find((m) => m.type === 'exit') as
      | { stdoutLines?: string[] }
      | undefined
    const errorMsg = messages.find((m) => m.type === 'error')
    const stderrMsgs = messages.filter((m) => m.type === 'stderr')

    expect(errorMsg).toBeUndefined()
    expect(stderrMsgs.length).toBe(0)
    expect(exitMsg).toBeDefined()

    const lines = exitMsg?.stdoutLines || []
    console.log('Plugin stdout:', lines)

    // Verify startInfo was passed correctly
    expect(lines.some((l) => l.includes('START_INFO_CHECK'))).toBe(true)
    expect(lines.some((l) => l.includes('pluginId=my-test-plugin-id'))).toBe(
      true,
    )
    expect(lines.some((l) => l.includes('instanceId=instance-abc-123'))).toBe(
      true,
    )
    expect(lines.some((l) => l.includes('START_INFO_END'))).toBe(true)
  }, 120000)
})
