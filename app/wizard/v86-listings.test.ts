import { describe, expect, it, vi } from 'vitest'

import { resourceOf } from '@aptre/bldr-sdk/hooks/index.js'
import type { KeyedSet } from '@s4wave/sdk/world/entity-set.js'

import { v86ListingsStatus } from './v86-listings.js'

// retryImages and retryVms stand in for each listing's Retry, so a failure
// wired to the other listing's Retry fails toEqual.
const retryImages = vi.fn()
const retryVms = vi.fn()

// imageSet is a set of one image, complete or partway through its first read.
function imageSet(complete: boolean): KeyedSet<unknown> {
  return {
    seqno: 3n,
    keys: ['vm/image/1'],
    rows: new Map([['vm/image/1', {}]]),
    complete,
  }
}

const vmsRead = resourceOf<unknown>({
  value: [],
  loading: false,
  error: null,
  retry: retryVms,
})

describe('v86ListingsStatus', () => {
  it('stays pending while the image set is partial', () => {
    const images = resourceOf({
      value: imageSet(false),
      loading: false,
      error: null,
      retry: retryImages,
    })
    expect(v86ListingsStatus(images, vmsRead)).toEqual({
      pending: true,
      failures: [],
    })
  })

  it('is ready once both listings are complete', () => {
    const images = resourceOf({
      value: imageSet(true),
      loading: false,
      error: null,
      retry: retryImages,
    })
    expect(v86ListingsStatus(images, vmsRead)).toEqual({
      pending: false,
      failures: [],
    })
  })

  it('names the listing that failed', () => {
    const images = resourceOf({
      value: imageSet(true),
      loading: false,
      error: null,
      retry: retryImages,
    })
    const vms = resourceOf<unknown>({
      value: null,
      loading: false,
      error: new Error('world closed'),
      retry: retryVms,
    })
    expect(v86ListingsStatus(images, vms)).toEqual({
      pending: true,
      failures: [
        {
          title: 'Could not read the VMs in this Space',
          message: 'world closed',
          retry: retryVms,
        },
      ],
    })

    const noImages = resourceOf<KeyedSet<unknown>>({
      value: null,
      loading: false,
      error: new Error('stream reset'),
      retry: retryImages,
    })
    expect(
      v86ListingsStatus(noImages, vmsRead).failures.map((f) => f.title),
    ).toEqual(['Could not read the VM images in this Space'])
  })

  it('fails an image read that stops after a partial page', () => {
    // At the reconnect cap the stream fails with the partial set as last.
    const images = resourceOf({
      value: null,
      last: imageSet(false),
      loading: false,
      error: new Error('stream reset'),
      retry: retryImages,
    })
    expect(v86ListingsStatus(images, vmsRead)).toEqual({
      pending: true,
      failures: [
        {
          title: 'Could not read the VM images in this Space',
          message: 'stream reset',
          retry: retryImages,
        },
      ],
    })
  })

  it('stays quiet when a complete image set later fails', () => {
    const images = resourceOf({
      value: null,
      last: imageSet(true),
      loading: true,
      error: new Error('stream reset'),
      retry: retryImages,
    })
    expect(v86ListingsStatus(images, vmsRead)).toEqual({
      pending: false,
      failures: [],
    })
  })

  it('names both listings when both fail', () => {
    const images = resourceOf({
      value: null,
      last: imageSet(false),
      loading: false,
      error: new Error('stream reset'),
      retry: retryImages,
    })
    const vms = resourceOf<unknown>({
      value: null,
      loading: false,
      error: new Error('world closed'),
      retry: retryVms,
    })
    const failures = v86ListingsStatus(images, vms).failures
    expect(failures.map((f) => f.title)).toEqual([
      'Could not read the VM images in this Space',
      'Could not read the VMs in this Space',
    ])
    retryImages.mockClear()
    retryVms.mockClear()
    failures[0].retry()
    expect(retryImages).toHaveBeenCalledOnce()
    expect(retryVms).not.toHaveBeenCalled()
    failures[1].retry()
    expect(retryVms).toHaveBeenCalledOnce()
  })
})
