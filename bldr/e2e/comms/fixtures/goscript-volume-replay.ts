import {
  detectWorkerCommsConfig,
  holdWebDocumentLock,
  runGoScriptWorker,
} from './_goscript-worker.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      report?: string
    }
  }
}

// run runs the GoScript volume worker in the mode named by the page's "mode"
// query parameter and publishes its report. An "instance" query parameter
// gives the page its own plugin and document identity, so pages of one
// profile run separate workers.
async function run() {
  const log = document.getElementById('log')!
  let releaseLock: (() => void) | undefined
  try {
    const params = new URLSearchParams(location.search)
    const mode = params.get('mode') ?? 'check'
    const instance = params.get('instance')
    const pluginId = 'goscript-volume-replay' + (instance ? '-' + instance : '')
    const documentId = pluginId + '-doc'
    const detect = await detectWorkerCommsConfig()
    releaseLock = await holdWebDocumentLock(`bldr-doc-${documentId}`)
    const result = await runGoScriptWorker(detect, {
      script: 'goscript-volume-replay-plugin.js',
      pluginId,
      documentId,
      mode,
      doneType: 'volume-done',
      failedType: 'volume-failed',
      timeoutMs: 90000,
    })
    const pass = result.workerReady && !result.failureReason
    window.__results = {
      pass,
      detail: pass
        ? 'done'
        : `workerReady=${result.workerReady}; failureReason=${result.failureReason ?? ''}`,
      report: String(result.done.report),
    }
  } catch (err) {
    window.__results = { pass: false, detail: `error: ${String(err)}` }
  } finally {
    releaseLock?.()
    log.textContent = 'DONE'
  }
}

run()
