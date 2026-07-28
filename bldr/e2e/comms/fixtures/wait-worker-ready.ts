const workerReadyTimeoutMs = 30_000

export function waitWorkerReady(port: MessagePort): Promise<boolean> {
  const { promise, resolve } = Promise.withResolvers<boolean>()
  const timer = globalThis.setTimeout(() => {
    cleanup()
    resolve(false)
  }, workerReadyTimeoutMs)
  const handler = (ev: MessageEvent) => {
    const data = ev.data
    if (!isRecord(data) || data.ready !== true) {
      return
    }
    cleanup()
    resolve(true)
  }
  const cleanup = () => {
    globalThis.clearTimeout(timer)
    port.removeEventListener('message', handler)
  }
  port.addEventListener('message', handler)
  return promise
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}
