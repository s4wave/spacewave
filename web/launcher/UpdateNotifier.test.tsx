import { cleanup, render, waitFor } from '@testing-library/react'
import { Client as SRPCClient } from 'starpc'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  LauncherInfo,
  UpdatePhase,
} from '@s4wave/core/provider/spacewave/launcher/launcher.pb.js'
import { LauncherServiceName } from '@s4wave/core/provider/spacewave/launcher/launcher_srpc.pb.js'
import { asyncValues } from '@s4wave/web/test/async-values.js'
import type { toast } from '@s4wave/web/ui/toaster.js'

const mocks = vi.hoisted(() => ({
  buildWebViewHostOpenStream: vi.fn(() => vi.fn()),
  serverStreamingRequest: vi.fn(),
  request: vi.fn(),
  toast: vi.fn<typeof toast>(),
  toastError: vi.fn(),
}))

vi.mock('@aptre/bldr', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@aptre/bldr')>()),
  isDesktop: true,
}))

vi.mock('@aptre/bldr-react', async (importOriginal) => {
  // Keep the WebView identity stable while the real watch hook publishes state.
  const context = {
    webDocument: {
      buildWebViewHostOpenStream: mocks.buildWebViewHostOpenStream,
    },
    webView: { getUuid: () => 'update-webview' },
  }

  // Preserve the subscription lifecycle exercised by the notifier.
  return {
    ...(await importOriginal<typeof import('@aptre/bldr-react')>()),
    useBldrContext: () => context,
  }
})

vi.mock('starpc', async (importOriginal) => ({
  ...(await importOriginal<typeof import('starpc')>()),
  Client: vi.fn(function () {
    return {
      serverStreamingRequest: mocks.serverStreamingRequest,
      request: mocks.request,
    }
  }),
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: Object.assign(mocks.toast, { error: mocks.toastError }),
}))

import { UpdateNotifier } from './UpdateNotifier.js'

describe('UpdateNotifier', () => {
  beforeEach(() => {
    // Supply a staged update through the generated launcher's binary codec.
    vi.clearAllMocks()
    mocks.serverStreamingRequest.mockImplementation(() =>
      asyncValues(
        LauncherInfo.toBinary({
          updateState: { phase: UpdatePhase.STAGED, version: '1.2.3' },
        }),
      ),
    )
    mocks.request.mockResolvedValue(new Uint8Array())
  })

  afterEach(() => cleanup())

  it('watches and applies updates through the plugin service on the WebView host', async () => {
    // Mount the notifier without a root resource and wait for the update toast.
    const { unmount } = render(<UpdateNotifier />)
    await waitFor(() => expect(mocks.toast).toHaveBeenCalledTimes(1))
    expect(mocks.buildWebViewHostOpenStream).toHaveBeenCalledWith(
      'update-webview',
    )
    expect(SRPCClient).toHaveBeenCalledWith(
      mocks.buildWebViewHostOpenStream.mock.results[0].value,
    )
    expect(mocks.serverStreamingRequest).toHaveBeenCalledWith(
      'plugin/spacewave-launcher/' + LauncherServiceName,
      'WatchLauncherInfo',
      new Uint8Array(),
      expect.any(AbortSignal),
    )

    // Follow the toast action through the same generated Launcher client.
    const [title, options] = mocks.toast.mock.calls[0]
    expect(title).toBe('Update ready')
    expect(options?.description).toBe('Version 1.2.3 is ready to install.')
    const action = options?.action
    if (!action || typeof action !== 'object' || !('onClick' in action)) {
      throw new Error('Update toast has no restart action')
    }
    expect(action.label).toBe('Restart now')
    action.onClick({} as React.MouseEvent<HTMLButtonElement>)
    await waitFor(() =>
      expect(mocks.request).toHaveBeenCalledWith(
        'plugin/spacewave-launcher/' + LauncherServiceName,
        'ApplyUpdate',
        new Uint8Array(),
        undefined,
      ),
    )
    expect(SRPCClient).toHaveBeenCalledTimes(1)

    // Unmounting releases the watch through its existing abort signal.
    const signal = mocks.serverStreamingRequest.mock.calls[0][3] as AbortSignal
    unmount()
    expect(signal.aborted).toBe(true)
  })
})
