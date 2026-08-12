// shared-worker.ts is the unified worker entry point.
//
// It parses URL hash parameters to determine:
// - s: script path (the worker script to load)
// - p: plugin mode ('1' = plugin worker, absent = custom worker)
//
// Plugin workers (p=1): creates PluginWorker wrapper which manages
// WebDocumentTracker, BackendApiImpl, and calls the script's main(api, signal).
//
// Custom workers (no p): imports the script directly and lets it self-manage.
// The script provides its own message listeners and WebDocumentTracker.

import { HandleStreamCtr, HandleStreamFunc } from 'starpc'
import { isBackendEntrypointFunc } from '@aptre/bldr-sdk'

import {
  checkSharedWorker,
  PluginWorker,
  type PluginStartOpts,
} from '../runtime/plugin-worker.js'
import { BackendApiImpl } from '../../sdk/impl/backend-api.js'
import { startWorkerPluginEntrypoint } from './plugin-entrypoint.js'
import { createTransportFactory } from './plugin-transport.js'
import { detectWorkerCommsConfig } from './worker-comms-detect.js'

declare let self: (SharedWorkerGlobalScope | DedicatedWorkerGlobalScope) & {
  name?: string
}

const pluginWorkerGlobal = globalThis as typeof globalThis & {
  BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?: (err: unknown) => void
}

// parseUrlParams parses the URL parameters.
// Format: ?s=<scriptPath>&p=<plugin>
function parseUrlParams(): {
  scriptPath: string
  isPlugin: boolean
} {
  const url = new URL(self.location.href)
  const nameParams =
    typeof self.name === 'string' ? self.name.split('?')[1] : ''
  const raw = url.search || url.hash || (nameParams ? `?${nameParams}` : '')

  if (!raw || (raw[0] !== '?' && raw[0] !== '#')) {
    throw new Error('shared-worker: Missing parameters in URL.')
  }

  const params = new URLSearchParams(raw.substring(1))

  const scriptPath = params.get('s')
  if (!scriptPath) {
    throw new Error('shared-worker: Missing script path (s) in URL parameters.')
  }

  const isPlugin = params.get('p') === '1'

  return { scriptPath: decodeURIComponent(scriptPath), isPlugin }
}

const { isPlugin } = parseUrlParams()

if (isPlugin) {
  // Plugin mode: use PluginWorker wrapper with BackendApiImpl lifecycle.
  const handleIncomingStreamCtr = new HandleStreamCtr()
  const handleIncomingStream: HandleStreamFunc =
    handleIncomingStreamCtr.handleStreamFunc

  const startPluginCallback = async (opts: PluginStartOpts) => {
    const { startInfo } = opts
    const { scriptPath } = parseUrlParams()

    // Use the detection result from the WebDocument init message (authoritative).
    // Falls back to local detection for standalone test fixtures.
    const detect = opts.workerCommsDetect ?? (await detectWorkerCommsConfig())
    const transport = createTransportFactory(detect, {
      openStream: pluginWorker.webRuntimeClient.openStream.bind(
        pluginWorker.webRuntimeClient,
      ),
      handleIncomingStream: handleIncomingStream,
      openPairEndpoint: pluginWorker.webDocumentTracker.requestSabPair.bind(
        pluginWorker.webDocumentTracker,
      ),
      closePairEndpoint: pluginWorker.webDocumentTracker.closeSabPair.bind(
        pluginWorker.webDocumentTracker,
      ),
    })

    const backendAPI = new BackendApiImpl(
      startInfo,
      transport.openStream,
      handleIncomingStreamCtr,
      opts.signal,
    )

    console.log('shared-worker: starting plugin:', scriptPath)

    pluginWorker.notifyStartupMark('plugin.script-import-start', {
      path: scriptPath,
    })
    // Plugin modules are selected by manifest at runtime, so this import cannot
    // be static.
    const pluginModule = await import(scriptPath)
    pluginWorker.notifyStartupMark('plugin.script-import-ready', {
      path: scriptPath,
    })
    if (!isBackendEntrypointFunc(pluginModule.default)) {
      throw new Error(
        `shared-worker: Imported module "${scriptPath}" does not have a default export function.`,
      )
    }
    await startWorkerPluginEntrypoint(
      pluginModule.default,
      backendAPI,
      opts.signal,
      opts.runtimeWasmEnv,
      (err) => {
        console.warn('shared-worker: plugin runtime failed:', err)
        void pluginWorker.reportRuntimeFailure(err)
      },
    )
  }

  const pluginWorker = new PluginWorker(
    self,
    startPluginCallback,
    handleIncomingStream,
  )
  pluginWorkerGlobal.BLDR_PLUGIN_REPORT_RUNTIME_FAILURE = (err: unknown) => {
    console.warn('shared-worker: plugin runtime failed:', err)
    void pluginWorker.reportRuntimeFailure(err)
  }
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
          self.dispatchEvent(
            new MessageEvent('connect', { ports: [...ev.ports] }),
          )
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
