import {
  HandleStreamCtr,
  HandleStreamFunc,
  OpenStreamCtr,
  StreamConn,
} from 'starpc'
import { writeSourceToFd, type QuickjsGlobalScope } from './quickjs/quickjs.js'
import { pushable } from 'it-pushable'
import { pipe } from 'it-pipe'
import { applyPolyfills } from './quickjs/polyfill.js'
import { BackendApiImpl } from '../../../sdk/impl/backend-api.js'

// globalThis is the top level quickjs global scope.
declare const globalThis: QuickjsGlobalScope

// expect the script path via the environment variable.
const scriptPath = globalThis.std.getenv('BLDR_SCRIPT_PATH')!

// bail out if scriptPath is not set
if (!scriptPath) {
  globalThis.console.log('BLDR_SCRIPT_PATH must be defined')
  globalThis.std.exit(1)
}

// polyfill Event, AbortController, etc.
applyPolyfills(globalThis)

// asynchronously import the script module
const scriptPromise = import(scriptPath)
scriptPromise.catch((err) => {
  console.error('error importing script: ' + scriptPath, err)
  globalThis.std.exit(1)
})

// expect the start info via the environment variable.
const startInfoB58 = globalThis.std.getenv('BLDR_PLUGIN_START_INFO') ?? ''

// handleIncomingStreamCtr is the container for the plugin handle stream func.
const handleIncomingStreamCtr = new HandleStreamCtr()
// handleIncomingStream waits for a handler to be registered in handleIncomingStreamCtr.
const handleIncomingStream: HandleStreamFunc =
  handleIncomingStreamCtr.handleStreamFunc

// openStreamCtr is the container for the function to open streams with /dev/out
const openStreamCtr = new OpenStreamCtr()
// openStream opens a stream with the plugin host via /dev/out
const openStream = openStreamCtr.openStreamFunc

// open stdin for incoming data and /dev/out for outgoing data with yamux.
const stdinFd = 0 // stdin file descriptor
const stdinReadBuffer = new Uint8Array(32 * 1024)

// construct the yamux connection with the host
const runtimeConn = new StreamConn(
  { handlePacketStream: handleIncomingStream },
  {
    direction: 'inbound',
    yamuxParams: {
      enableKeepAlive: false,
      maxMessageSize: 32 * 1024,
    },
  },
)
const stdinStream = pushable<Uint8Array>({ objectMode: true })

// handle data being ready to read from stdin
function stdinReadHandler() {
  const bytesRead = globalThis.os.read(
    stdinFd,
    stdinReadBuffer.buffer,
    0,
    stdinReadBuffer.length,
  )
  if (bytesRead === 0) {
    return
  }

  // copy the data out of the read buffer to a Uint8Array
  const readData = stdinReadBuffer.slice(0, bytesRead)
  stdinStream.push(readData)
}
globalThis.os.setReadHandler(stdinFd, stdinReadHandler)

// pipe stdin to the runtimeConn and then out to /dev/out.
pipe(stdinStream, runtimeConn, async (source) =>
  writeSourceToFd(globalThis.os, source, '/dev/out'),
).catch((err) => {
  console.error('caught error in pipe', err)
  globalThis.std.exit(1)
})

// start outgoing streams
openStreamCtr.set(runtimeConn.buildOpenStreamFunc())

// start the plugin by importing the script file and calling the default export.
async function startPlugin() {
  // Dynamically import the specified plugin module.
  const script = await scriptPromise
  if (typeof script.default !== 'function') {
    throw new Error(
      `shared-worker: Imported module "${scriptPath}" does not have a default export function.`,
    )
  }

  // Construct the backend api
  const backendAPI = new BackendApiImpl(
    startInfoB58,
    openStream,
    handleIncomingStreamCtr,
  )

  // Garbage collect
  globalThis.gc?.()

  // Call the imported module's main function, passing the API implementation.
  await script.default(backendAPI)
}

// immediately call startPlugin
startPlugin().catch((err) => {
  console.error(err)
  globalThis.std.exit(1)
})
