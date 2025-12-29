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
// 5. Call qjs_init_argv() with boot harness path
// 6. Run event loop with qjs_loop_once()
//    - Returns >0: setTimeout(loop, ms)
//    - Returns 0: queueMicrotask(loop)
//    - Returns -1: idle, wait for I/O
//    - Returns -2: error
// 7. Yields to browser event loop between iterations

import { HandleStreamCtr, HandleStreamFunc, StreamConn } from 'starpc'
import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'

import { PluginWorker } from '../runtime/plugin-worker.js'
import { PluginStartInfo } from '../../plugin/plugin.pb.js'
import {
  WASI,
  WASIProcExit,
  File,
  Directory,
  PreopenDirectory,
  ConsoleStdout,
  PollableStdin,
  DevOut,
  DevDirectory,
} from '../wasi-shim/index.js'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope

console.log('shw-quickjs: SharedWorker loaded')

// QuickJS reactor model loop result values
const LOOP_ERROR = -2
const LOOP_IDLE = -1

// QuickJS WASM reactor exports interface
interface QuickJSReactorExports {
  // Standard WASI reactor
  _initialize(): void
  memory: WebAssembly.Memory

  // QuickJS reactor-specific
  qjs_init_argv(argc: number, argv: number): number
  qjs_loop_once(): number // Returns: 0=more, >0=timer_ms, -1=idle, -2=error
  qjs_poll_io(timeout_ms: number): number
}

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
    throw new Error(`Failed to fetch plugin script ${scriptPath}: ${response.status}`)
  }
  return response.text()
}

// buildPluginFileSystem creates a WASI directory structure from a script path.
// The scriptPath is expected to be like /b/pd/plugin-name/plugin-HASH.mjs
// We create a nested directory structure: /b/pd/plugin-name/plugin-HASH.mjs
function buildPluginFileSystem(
  scriptPath: string,
  scriptContent: string,
  bootHarness: string,
): Map<string, File | Directory> {
  const rootContents = new Map<string, File | Directory>()

  // Add boot harness at /boot/plugin-quickjs.esm.js
  rootContents.set(
    'boot',
    new Directory(
      new Map([
        ['plugin-quickjs.esm.js', new File(new TextEncoder().encode(bootHarness))],
      ]),
    ),
  )

  // Parse the script path and create nested directories
  // e.g., /b/pd/spacewave-web/plugin-43DC6NFD.mjs
  // becomes: b/pd/spacewave-web/plugin-43DC6NFD.mjs
  const parts = scriptPath.split('/').filter((p) => p.length > 0)
  if (parts.length === 0) {
    throw new Error(`Invalid script path: ${scriptPath}`)
  }

  // Build nested directory structure
  let currentMap = rootContents
  for (let i = 0; i < parts.length - 1; i++) {
    const part = parts[i]
    const existing = currentMap.get(part)
    if (existing instanceof Directory) {
      currentMap = existing.contents as Map<string, File | Directory>
    } else {
      const newDir = new Map<string, File | Directory>()
      currentMap.set(part, new Directory(newDir))
      currentMap = newDir
    }
  }

  // Add the script file at the end
  const fileName = parts[parts.length - 1]
  currentMap.set(fileName, new File(new TextEncoder().encode(scriptContent)))

  return rootContents
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
  const devOut = new DevOut((data) => {
    devOutStream.push(new Uint8Array(data))
  })
  const devDir = new DevDirectory('/dev', new Map([['out', devOut]]))

  // Create stdout/stderr handlers that log to console
  const stdout = ConsoleStdout.lineBuffered((line) => {
    console.log('[QuickJS stdout]', line)
  })
  const stderr = ConsoleStdout.lineBuffered((line) => {
    console.error('[QuickJS stderr]', line)
  })

  // Build virtual filesystem with boot harness and plugin script
  const rootContents = buildPluginFileSystem(scriptPath, pluginScript, bootHarness)
  const rootDir = new PreopenDirectory('/', rootContents)

  // Encode start info for the plugin
  const startInfoB64 = btoa(PluginStartInfo.toJsonString(startInfo))

  // WASI args and environment
  const args = ['qjs', '--std', '/boot/plugin-quickjs.esm.js']
  const env = [
    `BLDR_SCRIPT_PATH=${scriptPath}`,
    `BLDR_PLUGIN_START_INFO=${startInfoB64}`,
  ]

  // File descriptors: stdin, stdout, stderr, preopened dirs
  const fds = [
    stdin, // fd 0 - pollable stdin
    stdout, // fd 1 - stdout
    stderr, // fd 2 - stderr
    rootDir, // fd 3 - preopened /
    devDir, // fd 4 - preopened /dev
  ]

  const wasi = new WASI(args, env, fds, { debug: false })

  console.log('shw-quickjs: instantiating QuickJS reactor...')

  // Instantiate the WASM module with WASI imports
  const instance = await WebAssembly.instantiate(wasmModule, {
    wasi_snapshot_preview1: wasi.wasiImport,
  })

  const exports = instance.exports as unknown as QuickJSReactorExports

  // Initialize the WASI reactor
  wasi.initialize(instance as { exports: { memory: WebAssembly.Memory } })

  console.log('shw-quickjs: initializing QuickJS argv...')

  // Allocate and set up argv for QuickJS
  // We need to write the args to WASM memory and pass pointers
  const memory = exports.memory
  const encoder = new TextEncoder()

  // Allocate space for argv array (pointers) + arg strings
  // Use a simple bump allocator starting at a high address
  // This is a simplified approach - in production, use proper memory management
  const ARGV_BASE = 65536 // Start at 64KB
  const STRINGS_BASE = ARGV_BASE + args.length * 4 + 4 // +4 for null terminator

  const view = new DataView(memory.buffer)
  const bytes = new Uint8Array(memory.buffer)

  let stringOffset = STRINGS_BASE
  for (let i = 0; i < args.length; i++) {
    // Write pointer to argv[i]
    view.setUint32(ARGV_BASE + i * 4, stringOffset, true)

    // Write string
    const encoded = encoder.encode(args[i])
    bytes.set(encoded, stringOffset)
    bytes[stringOffset + encoded.length] = 0 // null terminator
    stringOffset += encoded.length + 1
  }
  // Write null terminator for argv
  view.setUint32(ARGV_BASE + args.length * 4, 0, true)

  // Call qjs_init_argv
  const initResult = exports.qjs_init_argv(args.length, ARGV_BASE)
  if (initResult !== 0) {
    throw new Error(`qjs_init_argv failed with code ${initResult}`)
  }

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
      stdin.push(data)
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
      result = exports.qjs_loop_once()
    } catch (e) {
      if (e instanceof WASIProcExit) {
        running = false
        exitCode = e.code
        console.log(`shw-quickjs: plugin exited with code ${exitCode}`)
        return
      }
      throw e
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
      if (stdin.hasData()) {
        // Data available - poll I/O to invoke read handlers
        try {
          exports.qjs_poll_io(0)
        } catch (e) {
          if (e instanceof WASIProcExit) {
            running = false
            exitCode = e.code
            return
          }
          throw e
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
      if (stdin.hasData()) {
        // Data available - poll I/O to invoke read handlers
        try {
          exports.qjs_poll_io(0)
        } catch (e) {
          if (e instanceof WASIProcExit) {
            running = false
            exitCode = e.code
            return
          }
          throw e
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
  stdin.close()

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
