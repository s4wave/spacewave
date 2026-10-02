import { cleanup, render, waitFor } from '@testing-library/react'
import { Client as SRPCClient } from 'starpc'
import { UpdateTarget } from '@aptre/bldr'
import { ApplyUpdateRequest } from '../../bldr/desktop/update/update.pb.js'
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
  applyElectronAppUpdate: vi.fn(),
  installedElectronAppVersion: vi.fn(),
}))

vi.mock('@aptre/bldr', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@aptre/bldr')>()),
  isDesktop: true,
  applyElectronAppUpdate: mocks.applyElectronAppUpdate,
  installedElectronAppVersion: mocks.installedElectronAppVersion,
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
    mocks.applyElectronAppUpdate.mockResolvedValue(undefined)
    mocks.installedElectronAppVersion.mockResolvedValue('1.2.2')
    mocks.request.mockResolvedValue(new Uint8Array())
    mocks.serverStreamingRequest.mockImplementation(() =>
      asyncValues(
        LauncherInfo.toBinary({
          updateState: {
            phase: UpdatePhase.STAGED,
            version: '1.2.3',
            stagedPath: '/staged/Spacewave.app',
            target: UpdateTarget.APP,
          },
        }),
      ),
    )
  })

  afterEach(() => cleanup())

  it('watches launcher state and sends the app action to Electron', async () => {
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

    // Follow the app action through Electron's own update custody.
    const [title, options] = mocks.toast.mock.calls[0]
    expect(title).toBe('App update ready')
    expect(options?.description).toContain('shared daemon will keep running')
    const action = options?.action
    if (!action || typeof action !== 'object' || !('onClick' in action)) {
      throw new Error('Update toast has no restart action')
    }
    expect(action.label).toBe('Update app')
    action.onClick({} as React.MouseEvent<HTMLButtonElement>)
    await waitFor(() =>
      expect(mocks.applyElectronAppUpdate).toHaveBeenCalledWith(
        'update-webview',
      ),
    )
    expect(mocks.request).not.toHaveBeenCalled()
    expect(SRPCClient).toHaveBeenCalledTimes(1)

    // Unmounting releases the watch through its existing abort signal.
    const signal = mocks.serverStreamingRequest.mock.calls[0][3] as AbortSignal
    unmount()
    expect(signal.aborted).toBe(true)
  })

  it('does not offer the app release it already runs', async () => {
    // Stage the installed version with a daemon update to mark the state seen.
    mocks.installedElectronAppVersion.mockResolvedValue('1.2.3')
    mocks.serverStreamingRequest.mockImplementation(() =>
      asyncValues(
        LauncherInfo.toBinary({
          updateState: {
            phase: UpdatePhase.STAGED,
            version: '1.2.3',
            stagedPath: '/staged/Spacewave.app',
            target: UpdateTarget.APP,
          },
          daemonUpdateState: {
            phase: UpdatePhase.STAGED,
            version: '1.2.3',
            target: UpdateTarget.DAEMON,
          },
        }),
      ),
    )
    render(<UpdateNotifier />)
    await waitFor(() =>
      expect(mocks.toast).toHaveBeenCalledWith(
        'Daemon update available',
        expect.anything(),
      ),
    )
    expect(mocks.installedElectronAppVersion).toHaveBeenCalled()
    expect(mocks.toast).not.toHaveBeenCalledWith(
      'App update ready',
      expect.anything(),
    )
  })

  it('accepts the daemon artifact through its explicit launcher target', async () => {
    mocks.serverStreamingRequest.mockImplementation(() =>
      asyncValues(
        LauncherInfo.toBinary({
          daemonUpdateState: {
            phase: UpdatePhase.STAGED,
            version: '1.2.3',
            stagedPath: '/staged/cli',
            target: UpdateTarget.DAEMON,
            artifactManifestId: 'spacewave-cli',
          },
        }),
      ),
    )

    render(<UpdateNotifier />)
    await waitFor(() => expect(mocks.toast).toHaveBeenCalledTimes(1))
    const [title, options] = mocks.toast.mock.calls[0]
    expect(title).toBe('Daemon update available')
    const action = options?.action
    if (!action || typeof action !== 'object' || !('onClick' in action)) {
      throw new Error('Daemon update toast has no action')
    }
    expect(action.label).toBe('Update daemon')
    action.onClick({} as React.MouseEvent<HTMLButtonElement>)
    await waitFor(() =>
      expect(mocks.request).toHaveBeenCalledWith(
        'plugin/spacewave-launcher/' + LauncherServiceName,
        'ApplyUpdate',
        ApplyUpdateRequest.toBinary({ target: UpdateTarget.DAEMON }),
        undefined,
      ),
    )
  })
  it('shows what an accepted daemon update waits for and restarts now', async () => {
    mocks.serverStreamingRequest.mockImplementation(() =>
      asyncValues(
        LauncherInfo.toBinary({
          daemonUpdateState: {
            phase: UpdatePhase.APPLYING,
            version: '1.2.3',
            target: UpdateTarget.DAEMON,
          },
          daemonUpdateWait: {
            otherWork: [
              'glados',
              'spacewave (2)',
              'web listener at http://127.0.0.1:8080',
            ],
          },
        }),
      ),
    )

    render(<UpdateNotifier />)
    await waitFor(() => expect(mocks.toast).toHaveBeenCalledTimes(1))
    const [title, options] = mocks.toast.mock.calls[0]
    expect(title).toBe('Daemon update accepted')
    expect(options?.id).toBe('daemon-update')
    expect(options?.description).toBe(
      'Waiting for glados, spacewave (2), and web listener at http://127.0.0.1:8080 to finish before restarting.',
    )
    const action = options?.action
    if (!action || typeof action !== 'object' || !('onClick' in action)) {
      throw new Error('Waiting daemon update toast has no action')
    }
    expect(action.label).toBe('Restart now')
    action.onClick({} as React.MouseEvent<HTMLButtonElement>)
    await waitFor(() =>
      expect(mocks.request).toHaveBeenCalledWith(
        'plugin/spacewave-launcher/' + LauncherServiceName,
        'RestartDaemonUpdateNow',
        new Uint8Array(),
        undefined,
      ),
    )
  })
})
