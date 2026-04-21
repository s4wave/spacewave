// shared-worker.ts is the unified worker entry point for all worker types.
//
// It parses URL hash parameters to determine:
// - s: script path (the worker script to load)
// - t: worker type ('native' or 'quickjs', defaults to 'native')
// - p: plugin mode ('1' = plugin worker, absent = custom worker)
//
// Plugin workers (p=1): creates PluginWorker wrapper which manages
// WebDocumentTracker, BackendApiImpl, and calls the script's main(api, signal).
//
// Custom workers (no p): imports the script directly and lets it self-manage.
// The script provides its own message listeners and WebDocumentTracker.

import { HandleStreamCtr, HandleStreamFunc } from 'starpc'

import {
  checkSharedWorker,
  PluginWorker,
  type PluginStartOpts,
} from '../runtime/plugin-worker.js'
import { BackendApiImpl } from '../../sdk/impl/backend-api.js'
import { createTransportFactory } from './plugin-transport.js'
import { detectWorkerCommsConfig } from './worker-comms-detect.js'
import { SabBusEndpoint } from './sab-bus.js'

declare let self: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope

// parseUrlParams parses the URL hash parameters.
// Format: #s=<scriptPath>&t=<workerType>&p=<plugin>
function parseUrlParams(): {
  scriptPath: string
  workerType: string
  isPlugin: boolean
} {
  const url = new URL(self.location.href)
  const hash = url.hash

  if (!hash || !hash.startsWith('#')) {
    throw new Error('shared-worker: Missing hash parameters in URL.')
  }

  // Parse hash as query string (remove leading #)
  const params = new URLSearchParams(hash.substring(1))

  const scriptPath = params.get('s')
  if (!scriptPath) {
    throw new Error('shared-worker: Missing script path (s) in URL hash.')
  }

  const workerType = params.get('t') ?? 'native'
  const isPlugin = params.get('p') === '1'

  return { scriptPath: decodeURIComponent(scriptPath), workerType, isPlugin }
}

const { isPlugin } = parseUrlParams()

if (isPlugin) {
  // Plugin mode: use PluginWorker wrapper with BackendApiImpl lifecycle.
  const handleIncomingStreamCtr = new HandleStreamCtr()
  const handleIncomingStream: HandleStreamFunc =
    handleIncomingStreamCtr.handleStreamFunc

  const startPluginCallback = async (opts: PluginStartOpts) => {
    const { startInfo, busSab, busPluginId } = opts
    const { scriptPath, workerType } = parseUrlParams()

    // Set up SAB bus endpoint if the bus SAB was provided.
    // Falls back to MessagePort-only transport if bus initialization fails.
    let busEndpoint: SabBusEndpoint | undefined
    if (busSab && busPluginId != null) {
      try {
        busEndpoint = new SabBusEndpoint(busSab, busPluginId)
        busEndpoint.register()
        console.log('shared-worker: registered on SAB bus with pluginId', busPluginId)
      } catch (err) {
        console.warn('shared-worker: SAB bus init failed, falling back to MessagePort', err)
        busEndpoint = undefined
      }
    }

    // Use the detection result from the WebDocument init message (authoritative).
    // Falls back to local detection for standalone test fixtures.
    const detect = opts.workerCommsDetect ?? await detectWorkerCommsConfig()
    const transport = createTransportFactory(detect, {
      openStream: pluginWorker.webRuntimeClient.openStream.bind(
        pluginWorker.webRuntimeClient,
      ),
      handleIncomingStream: handleIncomingStream,
      busEndpoint,
    })

    const abortController = new AbortController()
    const abortSignal = abortController.signal

    const backendAPI = new BackendApiImpl(
      startInfo,
      transport.openStream,
      handleIncomingStreamCtr,
      abortSignal,
    )

    if (workerType === 'quickjs') {
      console.log('shared-worker: starting QuickJS plugin:', scriptPath)
      const quickjsRunner =
        await import('../runtime/quickjs/plugin-host-quickjs.js')
      await quickjsRunner.default(backendAPI, abortSignal, scriptPath)
    } else {
      console.log('shared-worker: starting native plugin:', scriptPath)
      const pluginModule = await import(scriptPath)
      if (typeof pluginModule.default !== 'function') {
        throw new Error(
          `shared-worker: Imported module "${scriptPath}" does not have a default export function.`,
        )
      }
      await pluginModule.default(backendAPI, abortSignal)
    }
  }

  const pluginWorker = new PluginWorker(
    self,
    startPluginCallback,
    handleIncomingStream,
  )
} else {
  // Custom worker mode: import script directly and let it self-manage.
  // Buffer messages that arrive during the async import. The script registers
  // its own message/connect listeners at module evaluation time, but the init
  // postMessage from WebDocument may arrive before the import completes.
  const { scriptPath } = parseUrlParams()
  const buffered: MessageEvent[] = []

  const bufferHandler = (ev: MessageEvent) => {
    buffered.push(ev)
  }

  if (checkSharedWorker(self)) {
    self.addEventListener('connect', bufferHandler as EventListener)
  } else {
    self.addEventListener('message', bufferHandler)
  }

  console.log('shared-worker: loading custom worker script:', scriptPath)
  import(scriptPath)
    .then(() => {
      if (checkSharedWorker(self)) {
        self.removeEventListener('connect', bufferHandler as EventListener)
        for (const ev of buffered) {
          self.dispatchEvent(new MessageEvent('connect', { ports: [...ev.ports] }))
        }
      } else {
        self.removeEventListener('message', bufferHandler)
        for (const ev of buffered) {
          self.dispatchEvent(new MessageEvent('message', { data: ev.data }))
        }
      }
      buffered.length = 0
    })
    .catch((err) => {
      console.error('shared-worker: failed to load custom worker script:', err)
      self.close()
    })
}
