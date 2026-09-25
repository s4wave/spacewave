import {
  detectWorkerCommsConfig,
  holdWebDocumentLock,
  runGoScriptWorker,
  type WorkerComms,
} from './_goscript-worker.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      workerReady: boolean
      write: boolean
      reloadRead: boolean
      cleanup: boolean
      failureReason?: string
    }
  }
}

const pluginId = 'goscript-opfs-storage-proof'
const documentId = 'goscript-opfs-storage-proof-doc'

async function runOpfsWorker(
  detect: WorkerComms,
  mode: string,
): Promise<{ workerReady: boolean; done: boolean; failureReason?: string }> {
  const result = await runGoScriptWorker(detect, {
    script: 'goscript-opfs-storage-plugin.js',
    pluginId,
    documentId,
    mode,
    doneType: 'opfs-done',
    failedType: 'opfs-failed',
    timeoutMs: 30000,
  })
  return { ...result, done: true }
}

async function run() {
  const log = document.getElementById('log')!
  let releaseLock: (() => void) | undefined
  try {
    const detect = await detectWorkerCommsConfig()
    releaseLock = await holdWebDocumentLock(`bldr-doc-${documentId}`)
    const write = await runOpfsWorker(detect, 'write')
    const read = await runOpfsWorker(detect, 'read')
    const cleanup = await verifyCleanup()
    const failureReason = write.failureReason ?? read.failureReason
    const workerReady = write.workerReady && read.workerReady
    const pass =
      workerReady && write.done && read.done && cleanup && !failureReason
    window.__results = {
      pass,
      detail:
        pass ?
          'all tests passed'
        : [
            `workerReady=${workerReady}`,
            `write=${write.done}`,
            `reloadRead=${read.done}`,
            `cleanup=${cleanup}`,
            `failureReason=${failureReason ?? ''}`,
          ].join('; '),
      workerReady,
      write: write.done,
      reloadRead: read.done,
      cleanup,
      failureReason,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${String(err)}`,
      workerReady: false,
      write: false,
      reloadRead: false,
      cleanup: false,
      failureReason: undefined,
    }
  } finally {
    releaseLock?.()
    log.textContent = 'DONE'
  }
}

async function verifyCleanup(): Promise<boolean> {
  const root = await navigator.storage.getDirectory()
  try {
    await root.getDirectoryHandle('goscript-opfs-storage-proof', {
      create: false,
    })
    return false
  } catch (err) {
    return err instanceof DOMException && err.name === 'NotFoundError'
  }
}

run()
