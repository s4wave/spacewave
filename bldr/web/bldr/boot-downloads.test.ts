import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  advanceBootDownload,
  beginBootDownload,
  bootDownloadEvent,
  bootDownloadFraction,
  completeBootDownload,
  failBootDownload,
  readBootDownloads,
  resetBootDownloadsForTest,
  streamResponseWithBootProgress,
  subscribeBootDownloads,
} from './boot-downloads.js'

function streamOf(chunks: number[], headers?: Record<string, string>): Response {
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const size of chunks) controller.enqueue(new Uint8Array(size))
      controller.close()
    },
  })
  return new Response(body, { status: 200, headers })
}

describe('boot download registry', () => {
  afterEach(() => {
    resetBootDownloadsForTest()
    vi.restoreAllMocks()
  })

  it('tracks the byte lifecycle of one download', () => {
    beginBootDownload('runtime', 'Runtime', 100)
    expect(readBootDownloads()).toEqual([
      { id: 'runtime', label: 'Runtime', loaded: 0, total: 100, state: 'active' },
    ])

    advanceBootDownload('runtime', 40)
    expect(readBootDownloads()[0]).toMatchObject({ loaded: 40, total: 100 })
    expect(bootDownloadFraction(readBootDownloads()[0])).toBeCloseTo(0.4)

    completeBootDownload('runtime')
    const done = readBootDownloads()[0]
    expect(done).toMatchObject({ loaded: 100, state: 'complete' })
    expect(bootDownloadFraction(done)).toBe(1)
  })

  it('replaces the array reference on every mutation for change detection', () => {
    beginBootDownload('app', 'App', 10)
    const first = readBootDownloads()
    advanceBootDownload('app', 5)
    const second = readBootDownloads()
    expect(second).not.toBe(first)
    expect(second).toHaveLength(1)
  })

  it('keeps size unknown when no total is available', () => {
    beginBootDownload('app', 'App')
    advanceBootDownload('app', 2048)
    const download = readBootDownloads()[0]
    expect(download.total).toBeUndefined()
    expect(bootDownloadFraction(download)).toBeUndefined()
  })

  it('dispatches the boot download event on change', () => {
    const listener = vi.fn()
    const unsubscribe = subscribeBootDownloads(listener)
    beginBootDownload('runtime', 'Runtime', 100)
    advanceBootDownload('runtime', 50)
    expect(listener).toHaveBeenCalledTimes(2)
    unsubscribe()
    completeBootDownload('runtime')
    expect(listener).toHaveBeenCalledTimes(2)
  })

  it('records a failed download with its message', () => {
    beginBootDownload('runtime', 'Runtime', 100)
    failBootDownload('runtime', 'boom')
    expect(readBootDownloads()[0]).toMatchObject({ state: 'error', error: 'boom' })
  })

  it('streams a response with size-hint progress and returns the bytes', async () => {
    const events: string[] = []
    subscribeBootDownloads(() => {
      const download = readBootDownloads()[0]
      events.push(`${download.loaded}/${download.total ?? '?'}:${download.state}`)
    })
    const parts = await streamResponseWithBootProgress(
      'app',
      'App',
      streamOf([4, 6]),
      10,
    )
    expect(parts.reduce((sum, part) => sum + part.byteLength, 0)).toBe(10)
    expect(events).toContain('0/10:active')
    expect(events).toContain('4/10:active')
    expect(events).toContain('10/10:active')
    expect(events.at(-1)).toBe('10/10:complete')
  })

  it('derives the total from content-length when no hint is given', async () => {
    await streamResponseWithBootProgress(
      'app',
      'App',
      streamOf([2, 6], { 'content-length': '8' }),
    )
    expect(readBootDownloads()[0]).toMatchObject({ total: 8, state: 'complete' })
  })

  it('streams without a total when the size is unobservable', async () => {
    await streamResponseWithBootProgress('app', 'App', streamOf([3, 5]))
    const download = readBootDownloads()[0]
    expect(download.total).toBeUndefined()
    expect(download).toMatchObject({ loaded: 8, state: 'complete' })
  })
})

describe('boot download event name', () => {
  it('is the stable spacewave boot download event', () => {
    expect(bootDownloadEvent).toBe('spacewave-boot-download')
  })
})
