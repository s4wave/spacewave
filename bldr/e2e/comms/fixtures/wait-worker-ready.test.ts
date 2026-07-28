import { afterEach, describe, expect, it, vi } from 'vitest'

import { waitWorkerReady } from './wait-worker-ready.js'

type MessageHandler = (event: MessageEvent) => void

class FakeMessagePort {
  private handler: MessageHandler | undefined

  addEventListener(_type: 'message', handler: MessageHandler): void {
    this.handler = handler
  }

  removeEventListener(_type: 'message', handler: MessageHandler): void {
    if (this.handler === handler) {
      this.handler = undefined
    }
  }

  postMessage(data: unknown): void {
    this.handler?.({ data } as MessageEvent)
  }
}

describe('worker readiness wait', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('accepts readiness after the five-second startup window', async () => {
    vi.useFakeTimers()
    const port = new FakeMessagePort()
    const ready = waitWorkerReady(port as unknown as MessagePort)
    let settled = false
    ready.then(() => {
      settled = true
    })

    await vi.advanceTimersByTimeAsync(5001)
    expect(settled).toBe(false)

    port.postMessage({ ready: true })
    await expect(ready).resolves.toBe(true)
  })
})
