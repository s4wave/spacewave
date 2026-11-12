import { expect, test, vi } from 'vitest'
import type { BackendAPI } from '@aptre/bldr-sdk'

import { PluginHostResourceServiceClient } from '../../../sdk/plugin/host/host_srpc.pb.js'
import { createPluginEntrypoint } from '../../../web/bldr/plugin-entrypoint.js'

test('repository entrypoint uses compiled configuration without build-time globals', async () => {
  // Complete capability registration through the same client as ordinary plugins.
  const complete = vi
    .spyOn(
      PluginHostResourceServiceClient.prototype,
      'CompleteInitialCapabilityRegistration',
    )
    .mockResolvedValue({})
  const release = vi.fn()
  const api = {
    pluginHost: {
      GetPluginInfo: vi
        .fn()
        .mockResolvedValue({ pluginId: 'repository-fixture' }),
    },
    resourceClient: {
      accessRootResource: vi.fn().mockResolvedValue({ client: {}, release }),
    },
  } as unknown as BackendAPI
  const signal = new AbortController().signal

  // Empty module sets still complete startup without any compiler or injected constants.
  try {
    const entrypoint = createPluginEntrypoint({ webPluginId: 'web' })
    await entrypoint(api, signal).startup
    expect(complete).toHaveBeenCalledWith({}, signal)
    expect(release).toHaveBeenCalledOnce()
  } finally {
    complete.mockRestore()
  }
})
