import { describe, it, expect } from 'vitest'

const SERVER_PORT = 8091
const SERVER_URL = `http://localhost:${SERVER_PORT}`

// A test plugin that attempts an RPC call like the real plugin does
const TEST_PLUGIN_RPC_CALL = `
// Test plugin that attempts to call backendAPI.pluginHost.GetPluginInfo
export default async function main(backendAPI, abortSignal) {
  console.log("Plugin started!");
  console.log("backendAPI:", typeof backendAPI);
  console.log("backendAPI.pluginHost:", typeof backendAPI.pluginHost);
  console.log("backendAPI.pluginHost.GetPluginInfo:", typeof backendAPI.pluginHost?.GetPluginInfo);
  
  console.log("About to call GetPluginInfo...");
  try {
    // This is the call that hangs in the real scenario
    const pluginInfo = await backendAPI.pluginHost.GetPluginInfo({});
    console.log("GetPluginInfo returned:", JSON.stringify(pluginInfo));
  } catch (err) {
    console.log("GetPluginInfo error:", err.message);
  }
  
  console.log("Plugin done");
  std.exit(0);
}
`

describe('RPC Debug', () => {
  it('should trace RPC call through yamux', async () => {
    const pluginScriptB64 = btoa(TEST_PLUGIN_RPC_CALL)

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

          // Create pollable stdin for yamux communication (host → plugin)
          const stdin = new PollableStdin()
          
          // Track all /dev/out writes with detailed logging
          const devOutChunks = []
          let totalDevOutBytes = 0
          const devOut = new DevOut((data) => {
            const copy = new Uint8Array(data)
            devOutChunks.push(copy)
            totalDevOutBytes += copy.length
            
            // Parse yamux header if present
            let headerInfo = null
            if (copy.length >= 12) {
              headerInfo = {
                version: copy[0],
                type: ['Data', 'WindowUpdate', 'Ping', 'GoAway'][copy[1]] || 'Unknown(' + copy[1] + ')',
                flags: (copy[2] << 8) | copy[3],
                streamId: (copy[4] << 24) | (copy[5] << 16) | (copy[6] << 8) | copy[7],
                length: (copy[8] << 24) | (copy[9] << 16) | (copy[10] << 8) | copy[11],
              }
            }
            
            self.postMessage({ 
              type: 'devout', 
              chunkSize: copy.length,
              totalBytes: totalDevOutBytes,
              headerInfo,
              preview: Array.from(copy.slice(0, 32)).map(b => b.toString(16).padStart(2, '0')).join(' ')
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

          // Create start info
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

          // Set a timeout to detect hangs
          let timedOut = false
          const hangTimeout = setTimeout(() => {
            timedOut = true
            self.postMessage({ 
              type: 'timeout', 
              message: 'QuickJS execution timed out after 10 seconds',
              stdoutMessages,
              devOutChunks: devOutChunks.length,
              totalDevOutBytes,
            })
          }, 10000)

          try {
            wasiInstance.start(instance)
          } catch (e) {
            if (!e.message?.includes('exit')) {
              self.postMessage({ type: 'error', data: 'WASI error: ' + e.message + '\\n' + e.stack })
              return
            }
          }

          clearTimeout(hangTimeout)

          if (timedOut) {
            return
          }

          self.postMessage({ 
            type: 'exit', 
            exitCode: 0,
            stdoutMessages,
            devOutChunks: devOutChunks.length,
            totalDevOutBytes,
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
        reject(new Error('Test timed out after 30 seconds'))
      }, 30000)

      worker.onmessage = (e) => {
        messages.push(e.data)
        console.log('[Worker Message]', e.data)

        if (e.data.type === 'exit' || e.data.type === 'error' || e.data.type === 'timeout') {
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
    const timeoutMessage = messages.find((m) => m.type === 'timeout')
    const exitMessage = messages.find((m) => m.type === 'exit')

    console.log('=== Test Results ===')
    console.log('stdout:', stdoutMessages.map((m) => m.data))
    console.log('stderr:', stderrMessages.map((m) => m.data))
    console.log('devout chunks:', devoutMessages.length)
    console.log('devout details:', devoutMessages)
    
    if (errorMessage) {
      console.error('Error:', errorMessage.data)
    }
    if (timeoutMessage) {
      console.log('Timeout:', timeoutMessage)
    }
    if (exitMessage) {
      console.log('Exit:', exitMessage)
    }

    // The test should either complete or timeout
    // If it times out after "About to call GetPluginInfo...", we know the RPC is hanging
    expect(errorMessage).toBeUndefined()
    
    // Check what happened
    const sawAboutToCall = stdoutMessages.some((m) => 
      String(m.data).includes('About to call GetPluginInfo')
    )
    const sawGetPluginInfoReturned = stdoutMessages.some((m) => 
      String(m.data).includes('GetPluginInfo returned:')
    )
    const sawGetPluginInfoError = stdoutMessages.some((m) => 
      String(m.data).includes('GetPluginInfo error:')
    )
    
    console.log('sawAboutToCall:', sawAboutToCall)
    console.log('sawGetPluginInfoReturned:', sawGetPluginInfoReturned)
    console.log('sawGetPluginInfoError:', sawGetPluginInfoError)
    
    // If we timed out after "About to call GetPluginInfo", the RPC is hanging
    if (timeoutMessage && sawAboutToCall && !sawGetPluginInfoReturned && !sawGetPluginInfoError) {
      console.log('=== RPC is hanging! ===')
      console.log('The plugin called GetPluginInfo but never got a response.')
      console.log('devout messages show what yamux data was sent:')
      devoutMessages.forEach((m, i) => {
        console.log(`  chunk ${i}:`, m)
      })
    }
    
    // For now, just verify we get to the point of attempting the call
    expect(sawAboutToCall).toBe(true)
    
  }, 60000)
})
