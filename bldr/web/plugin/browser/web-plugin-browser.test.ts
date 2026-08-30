import { beforeEach, describe, expect, test, vi } from 'vitest'

const registration = vi.hoisted(() => ({
  construct: vi.fn(),
  complete: vi.fn(async (_request: unknown, _signal?: AbortSignal) => ({})),
}))

vi.mock('../../../sdk/plugin/host/host_srpc.pb.js', () => ({
  PluginHostResourceServiceClient: class {
    constructor(client: unknown) {
      registration.construct(client)
    }

    CompleteInitialCapabilityRegistration(
      request: unknown,
      signal?: AbortSignal,
    ) {
      return registration.complete(request, signal)
    }
  },
}))

import main from './web-plugin-browser.js'

describe('browser web plugin readiness', () => {
  beforeEach(() => {
    registration.construct.mockClear()
    registration.complete.mockClear()
  })

  test('completes initial registration after installing the RPC handler', async () => {
    const setHandler = vi.fn()
    const disposeRoot = vi.fn()
    const rootClient = {}
    const accessRootResource = vi.fn(async () => ({
      client: rootClient,
      release: disposeRoot,
    }))
    const controller = new AbortController()

    await main(
      {
        client: {},
        handleStreamCtr: { set: setHandler },
        resourceClient: { accessRootResource },
      } as never,
      controller.signal,
    )

    expect(setHandler).toHaveBeenCalledOnce()
    expect(accessRootResource).toHaveBeenCalledOnce()
    expect(registration.construct).toHaveBeenCalledWith(rootClient)
    expect(registration.complete).toHaveBeenCalledWith({}, controller.signal)
    expect(setHandler.mock.invocationCallOrder[0]).toBeLessThan(
      registration.complete.mock.invocationCallOrder[0],
    )
    expect(disposeRoot).toHaveBeenCalledOnce()
  })
})
