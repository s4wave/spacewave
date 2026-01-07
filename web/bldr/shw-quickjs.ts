// shw-quickjs.ts - SharedWorker for QuickJS WASI reactor model
//
// This worker runs JS plugins in QuickJS WASI reactor.
// Unlike the blocking command model, the reactor model yields to the
// browser event loop between iterations, allowing async RPC processing.
//
// Architecture:
// 1. Receive init message with plugin path from WebDocument
// 2. Fetch QuickJS WASM from /b/qjs/qjs-wasi.wasm
// 3. Fetch boot harness from /b/qjs/plugin-quickjs.esm.js
// 4. Create WASI environment with stdin/dev-out for yamux
// 5. Initialize QuickJS with initStdModule() and eval the boot harness
// 6. Run event loop with loopOnce()
//    - Returns >0: setTimeout(loop, ms)
//    - Returns 0: queueMicrotask(loop)
//    - Returns -1: idle, wait for I/O
//    - Returns -2: error
// 7. Yields to browser event loop between iterations

import { HandleStreamCtr, HandleStreamFunc, StreamConn } from 'starpc'
import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'
import {
  QuickJS,
  buildFileSystem,
  PollableStdin,
  LOOP_IDLE,
  LOOP_ERROR,
} from 'quickjs-wasi-reactor'

import { PluginWorker } from '../runtime/plugin-worker.js'
import { PluginStartInfo } from '../../plugin/plugin.pb.js'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope

console.log('shw-quickjs: SharedWorker loaded')

// handleIncomingStreamCtr is the container for the plugin handle stream func.
const handleIncomingStreamCtr = new HandleStreamCtr()
// handleIncomingStream waits for a handler to be registered in handleIncomingStreamCtr.
const handleIncomingStream: HandleStreamFunc =
  handleIncomingStreamCtr.handleStreamFunc

// Cached compiled QuickJS WASM module (shared across plugin restarts)
let cachedWasmModule: WebAssembly.Module | null = null
// Cached boot harness code
let cachedBootHarness: string | null = null

// loadQuickJSModule fetches and compiles the QuickJS WASM module.
async function loadQuickJSModule(): Promise<WebAssembly.Module> {
  if (cachedWasmModule) {
    return cachedWasmModule
  }
  const response = await fetch('/b/qjs/qjs-wasi.wasm')
  if (!response.ok) {
    throw new Error(`Failed to fetch QuickJS WASM: ${response.status}`)
  }
  cachedWasmModule = await WebAssembly.compileStreaming(response)
  return cachedWasmModule
}

// loadBootHarness fetches the boot harness JavaScript code.
async function loadBootHarness(): Promise<string> {
  if (cachedBootHarness) {
    return cachedBootHarness
  }
  const response = await fetch('/b/qjs/plugin-quickjs.esm.js')
  if (!response.ok) {
    throw new Error(`Failed to fetch boot harness: ${response.status}`)
  }
  cachedBootHarness = await response.text()
  return cachedBootHarness
}

// fetchPluginScript fetches the plugin script from the HTTP server.
async function fetchPluginScript(scriptPath: string): Promise<string> {
  const response = await fetch(scriptPath)
  if (!response.ok) {
    throw new Error(
      `Failed to fetch plugin script ${scriptPath}: ${response.status}`,
    )
  }
  return response.text()
}

// runQuickJSPlugin runs a plugin using the QuickJS reactor model.
async function runQuickJSPlugin(
  scriptPath: string,
  startInfo: PluginStartInfo,
): Promise<void> {
  console.log('shw-quickjs: loading QuickJS and boot harness...')

  // Load WASM module, boot harness, and plugin script in parallel
  const [wasmModule, bootHarness, pluginScript] = await Promise.all([
    loadQuickJSModule(),
    loadBootHarness(),
    fetchPluginScript(scriptPath),
  ])

  console.log('shw-quickjs: setting up WASI environment...')

  // Create pollable stdin for yamux communication (host -> plugin)
  const stdin = new PollableStdin()

  // Track /dev/out writes for yamux communication (plugin -> host)
  const devOutStream = pushable<Uint8Array>({ objectMode: true })

  // Build virtual filesystem with boot harness and plugin script
  // The scriptPath is expected to be like /b/pd/plugin-name/plugin-HASH.mjs
  const files = new Map<string, string | Uint8Array>()

  // Add boot harness at /boot/plugin-quickjs.esm.js
  files.set('/boot/plugin-quickjs.esm.js', bootHarness)

  // Add plugin script at its expected path
  files.set(scriptPath, pluginScript)

  const fs = buildFileSystem(files)

  // Encode start info for the plugin
  const startInfoB64 = btoa(PluginStartInfo.toJsonString(startInfo))

  console.log('shw-quickjs: instantiating QuickJS reactor...')

  // Create QuickJS instance using the new API
  const qjs = new QuickJS(wasmModule, {
    args: ['qjs'],
    env: [
      `BLDR_SCRIPT_PATH=${scriptPath}`,
      `BLDR_PLUGIN_START_INFO=${startInfoB64}`,
    ],
    fs,
    stdin,
    stdout: (line) => console.log('[QuickJS stdout]', line),
    stderr: (line) => console.error('[QuickJS stderr]', line),
    onDevOut: (data) => devOutStream.push(new Uint8Array(data)),
  })

  console.log('shw-quickjs: initializing QuickJS...')

  // Initialize with std modules (std, os, bjson as globals)
  qjs.initStdModule()

  // Evaluate the boot harness as an ES module
  qjs.eval(bootHarness, true, '/boot/plugin-quickjs.esm.js')

  console.log('shw-quickjs: starting reactor event loop...')

  // Set up yamux connection for RPC
  // The host side (us) is 'outbound' - we initiate streams to WebRuntime
  const hostConn = new StreamConn(
    { handlePacketStream: handleIncomingStream },
    {
      direction: 'outbound',
      yamuxParams: {
        enableKeepAlive: false,
        maxMessageSize: 32 * 1024,
      },
    },
  )

  // Pipe devOut to hostConn, and hostConn output to stdin
  pipe(devOutStream, hostConn, async (source) => {
    for await (const chunk of source) {
      // chunk may be Uint8Array or Uint8ArrayList, normalize to Uint8Array
      const data =
        chunk instanceof Uint8Array ? chunk : new Uint8Array(chunk.subarray())
      qjs.pushStdin(data)
    }
  }).catch((err) => {
    console.error('shw-quickjs: yamux pipe error:', err)
  })

  // Connect to WebRuntime via pluginWorker
  const openStream = pluginWorker.webRuntimeClient.openStream.bind(
    pluginWorker.webRuntimeClient,
  )

  // Set up stream handling from plugin to host
  handleIncomingStreamCtr.set(async (stream) => {
    // When plugin opens a stream, forward it to WebRuntime
    const hostStream = await openStream()
    pipe(stream, hostStream, stream).catch((err) => {
      console.error('shw-quickjs: stream pipe error:', err)
    })
  })

  // Run the reactor event loop
  let running = true
  let exitCode = 0

  const runLoop = () => {
    if (!running) return

    let result: number
    try {
      result = qjs.loopOnce()
    } catch (e) {
      running = false
      exitCode = 1
      console.error('shw-quickjs: error in loopOnce:', e)
      return
    }

    if (result === LOOP_ERROR) {
      console.error('shw-quickjs: JavaScript error occurred')
      running = false
      return
    }

    if (result === 0) {
      // More microtasks pending, continue immediately
      queueMicrotask(runLoop)
      return
    }

    if (result > 0) {
      // Timer pending - but also check stdin
      if (qjs.hasStdinData()) {
        // Data available - poll I/O to invoke read handlers
        try {
          qjs.pollIO(0)
        } catch (e) {
          running = false
          exitCode = 1
          console.error('shw-quickjs: error in pollIO:', e)
          return
        }
        queueMicrotask(runLoop)
        return
      }
      // Wait for timer
      setTimeout(runLoop, result)
      return
    }

    if (result === LOOP_IDLE) {
      // Idle - check if stdin has data
      if (qjs.hasStdinData()) {
        // Data available - poll I/O to invoke read handlers
        try {
          qjs.pollIO(0)
        } catch (e) {
          running = false
          exitCode = 1
          console.error('shw-quickjs: error in pollIO:', e)
          return
        }
        queueMicrotask(runLoop)
        return
      }
      // No data - wait a bit and check again
      // In the browser, we can't truly block, so we poll periodically
      setTimeout(runLoop, 10)
      return
    }
  }

  // Start the event loop
  runLoop()

  // Wait for the plugin to exit
  await new Promise<void>((resolve) => {
    const checkDone = () => {
      if (!running) {
        resolve()
        return
      }
      setTimeout(checkDone, 100)
    }
    checkDone()
  })

  // Cleanup
  devOutStream.end()
  qjs.destroy()

  if (exitCode !== 0) {
    throw new Error(`Plugin exited with code ${exitCode}`)
  }
}

// Function passed to PluginWorker, called when the first WebDocument connects
// and sends initialization data.
const startPluginCallback = async (startInfo: PluginStartInfo) => {
  // Parse the script path from the worker's URL hash.
  const url = new URL(self.location.href)
  let scriptPath: string | null = null
  if (url.hash && url.hash.startsWith('#s=')) {
    scriptPath = decodeURIComponent(url.hash.substring(3)) // Remove '#s=' prefix
  }
  if (!scriptPath) {
    throw new Error('shw-quickjs: Missing script hash parameter in URL.')
  }

  console.log('shw-quickjs: starting QuickJS plugin:', scriptPath)

  await runQuickJSPlugin(scriptPath, startInfo)
}

// Initialize the PluginWorker.
// For QuickJS plugins, we accept incoming streams via handleIncomingStream.
// The PluginWorker registers the onconnect callback on "self" in its constructor.
const pluginWorker = new PluginWorker(
  self,
  startPluginCallback,
  handleIncomingStream,
)
