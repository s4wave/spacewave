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

const documentId = 'goscript-volume-replay-doc'

// run runs the GoScript volume worker in the mode named by the page's "mode"
// query parameter and publishes its report.
async function run() {
  const log = document.getElementById('log')!
  let releaseLock: (() => void) | undefined
  try {
    const mode = new URLSearchParams(location.search).get('mode') ?? 'check'
    const detect = await detectWorkerCommsConfig()
    releaseLock = await holdWebDocumentLock(`bldr-doc-${documentId}`)
    const result = await runGoScriptWorker(detect, {
      script: 'goscript-volume-replay-plugin.js',
      pluginId: 'goscript-volume-replay',
      documentId,
      mode,
      doneType: 'volume-done',
      failedType: 'volume-failed',
      timeoutMs: 90000,
    })
    const pass = result.workerReady && !result.failureReason
    window.__results = {
      pass,
      detail:
        pass ? 'done' : (
          `workerReady=${result.workerReady}; failureReason=${result.failureReason ?? ''}`
        ),
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
