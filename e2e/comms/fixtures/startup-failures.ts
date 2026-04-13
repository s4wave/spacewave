import { PluginStartInfo } from '../../../plugin/plugin.pb.js'
import { WebDocumentTracker } from '../../../web/bldr/web-document-tracker.js'
import { timeoutPromise } from '../../../web/bldr/timeout.js'
import { WebRuntimeClientType } from '../../../web/runtime/runtime.pb.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      slowRegistrationRejected: boolean
      closeDuringStartupRejected: boolean
      importFailureClosed: boolean
      importFailureReady: boolean
    }
  }
}

interface StartupResult {
  ok: boolean
  detail: string
}

function encodeStartInfo(): Uint8Array {
  const json = PluginStartInfo.toJsonString({})
  return new TextEncoder().encode(btoa(json))
}

async function holdWebDocumentLock(name: string): Promise<() => void> {
  let releaseLock: (() => void) | undefined
  const waitReleased = new Promise<void>((resolve) => {
    releaseLock = resolve
  })
  const waitReady = new Promise<void>((resolve, reject) => {
    navigator.locks
      .request(name, async () => {
        resolve()
        await waitReleased
      })
      .catch(reject)
  })
  await waitReady
  return () => releaseLock?.()
}

function waitForPortMessage<T>(
  port: MessagePort,
  predicate: (data: unknown, ports: readonly MessagePort[]) => data is T,
  timeoutMs: number,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = globalThis.setTimeout(() => {
      cleanup()
      reject(new Error(`timeout waiting for port message after ${timeoutMs}ms`))
    }, timeoutMs)
    const handler = (ev: MessageEvent) => {
      if (!predicate(ev.data, ev.ports)) {
        return
      }
      cleanup()
      resolve(ev.data)
    }
    const cleanup = () => {
      globalThis.clearTimeout(timer)
      port.removeEventListener('message', handler)
    }
    port.addEventListener('message', handler)
    port.start()
  })
}

async function runSlowRegistrationScenario(): Promise<StartupResult> {
  const webDocumentId = 'startup-failures-slow-doc'
  const lockName = `bldr-doc-${webDocumentId}`
  const releaseLock = await holdWebDocumentLock(lockName)
  let tracker!: WebDocumentTracker
  tracker = new WebDocumentTracker(
    'startup-failures-slow-worker',
    WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
    async () => {
      tracker.close()
    },
    null,
  )
  const { port1, port2 } = new MessageChannel()
  tracker.handleWebDocumentMessage({
    from: webDocumentId,
    initPort: port1,
  })
  port2.start()
  const start = performance.now()
  try {
    await tracker.waitConn()
    return {
      ok: false,
      detail: 'slow registration unexpectedly connected',
    }
  } catch (err) {
    const elapsed = performance.now() - start
    if (elapsed > 2500) {
      return {
        ok: false,
        detail: `slow registration rejection was not bounded (${Math.round(elapsed)}ms)`,
      }
    }
    return {
      ok: true,
      detail: `slow registration rejected in ${Math.round(elapsed)}ms: ${String(err)}`,
    }
  } finally {
    tracker.close()
    port2.close()
    releaseLock()
  }
}

async function runCloseDuringStartupScenario(): Promise<StartupResult> {
  const webDocumentId = 'startup-failures-close-doc'
  const lockName = `bldr-doc-${webDocumentId}`
  const releaseLock = await holdWebDocumentLock(lockName)
  let tracker!: WebDocumentTracker
  tracker = new WebDocumentTracker(
    'startup-failures-close-worker',
    WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
    async () => {
      tracker.close()
    },
    null,
  )
  const { port1, port2 } = new MessageChannel()
  tracker.handleWebDocumentMessage({
    from: webDocumentId,
    initPort: port1,
  })
  port2.addEventListener('message', (ev) => {
    const data = ev.data
    if (typeof data !== 'object' || !data?.connectWebRuntime) {
      return
    }
    port2.postMessage({
      from: 'startup-failures-close-doc',
      close: true,
    })
    releaseLock()
  })
  port2.start()
  const start = performance.now()
  try {
    await tracker.waitConn()
    return {
      ok: false,
      detail: 'close during startup unexpectedly connected',
    }
  } catch (err) {
    const elapsed = performance.now() - start
    if (elapsed > 1500) {
      return {
        ok: false,
        detail: `close during startup took too long to reject (${Math.round(elapsed)}ms)`,
      }
    }
    return {
      ok: true,
      detail: `close during startup rejected in ${Math.round(elapsed)}ms: ${String(err)}`,
    }
  } finally {
    tracker.close()
    port2.close()
  }
}

async function runImportFailureScenario(): Promise<{
  ok: boolean
  detail: string
  ready: boolean
}> {
  const webDocumentId = 'startup-failures-import-doc'
  const lockName = `bldr-doc-${webDocumentId}`
  const releaseLock = await holdWebDocumentLock(lockName)
  const worker = new Worker(
    new URL('./workers/plugin-startup-fixture.js?mode=import-fail', import.meta.url),
    {
      type: 'module',
      name: 'startup-failures-import-worker',
    },
  )
  const { port1, port2 } = new MessageChannel()
  let ready = false
  port2.addEventListener('message', (ev) => {
    const data = ev.data
    if (typeof data !== 'object' || !data?.connectWebRuntime) {
      if (typeof data === 'object' && data?.ready) {
        ready = true
      }
      return
    }
    const ackPort = data.connectWebRuntime.port ?? ev.ports[0]
    if (!ackPort) {
      return
    }
    ackPort.start()
    const { port1: runtimePort1, port2: runtimePort2 } = new MessageChannel()
    ackPort.postMessage(
      {
        from: 'startup-failures-import-doc',
        webRuntimePort: runtimePort1,
      },
      [runtimePort1],
    )
    runtimePort2.start()
    runtimePort2.postMessage({ connected: true })
  })
  port2.start()
  worker.postMessage(
    {
      from: 'startup-failures-import-doc',
      initData: encodeStartInfo(),
      initPort: port1,
    },
    [port1],
  )

  try {
    await waitForPortMessage<{ close: true }>(
      port2,
      (data): data is { close: true } => {
        return typeof data === 'object' && !!data?.close
      },
      3000,
    )
    await timeoutPromise(50)
    return {
      ok: !ready,
      detail: ready
        ? 'worker published ready before import failure closed it'
        : 'worker closed after import failure',
      ready,
    }
  } catch (err) {
    return {
      ok: false,
      detail: `import failure did not close cleanly: ${String(err)}`,
      ready,
    }
  } finally {
    releaseLock()
    worker.terminate()
    port2.close()
  }
}

async function run() {
  const log = document.getElementById('log')
  const details: string[] = []
  try {
    const slow = await runSlowRegistrationScenario()
    details.push(slow.detail)

    const close = await runCloseDuringStartupScenario()
    details.push(close.detail)

    const importFailure = await runImportFailureScenario()
    details.push(importFailure.detail)

    const pass = slow.ok && close.ok && importFailure.ok
    window.__results = {
      pass,
      detail: details.join('; '),
      slowRegistrationRejected: slow.ok,
      closeDuringStartupRejected: close.ok,
      importFailureClosed: importFailure.ok,
      importFailureReady: importFailure.ready,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `startup failures fixture crashed: ${String(err)}`,
      slowRegistrationRejected: false,
      closeDuringStartupRejected: false,
      importFailureClosed: false,
      importFailureReady: false,
    }
  }

  if (log) {
    log.textContent = 'DONE'
  }
}

run()
