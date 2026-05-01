import { describe, expect, it } from 'vitest'

import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthLayer,
  SharedObjectHealthRemediationHint,
  SharedObjectHealthStatus,
} from '@s4wave/core/sobject/sobject.pb.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { SessionSyncStatusView } from '@s4wave/app/session/SessionSyncStatusContext.js'

import { toLockView } from './lock.js'
import { toPairingView } from './pairing.js'
import { toResourceView } from './resource.js'
import { toSessionSyncView } from './session-sync.js'
import { toSharedObjectView } from './shared-object.js'
import { toUnixFSPathView } from './unixfs-path.js'
import { toWizardView } from './wizard.js'

// Helpers --------------------------------------------------------------------

function fakeSyncStatus(
  overrides: Partial<SessionSyncStatusView> = {},
): SessionSyncStatusView {
  return {
    snapshot: null,
    visualState: 'synced',
    loading: false,
    active: false,
    error: false,
    local: false,
    summaryLabel: 'Synced',
    detailLabel: 'All sync work complete.',
    ariaLabel: 'Session sync status: Synced',
    transportLabel: 'Cloud online',
    p2pLabel: 'P2P idle',
    uploadRateLabel: '0 B/s',
    downloadRateLabel: '0 B/s',
    pendingUploadLabel: '0 B',
    activeUploadLabel: '0 B',
    pendingDownloadLabel: '0 B',
    packRangeLabel: '0 / 0 B',
    packIndexTailLabel: '0 / 0 B',
    packLookupLabel: '0 opened / 0 candidates',
    packIndexCacheLabel: '0 hits / 0 misses',
    lastActivityLabel: 'No recent activity',
    lastError: '',
    ...overrides,
  }
}

function fakeResource<T>(overrides: Partial<Resource<T>>): Resource<T> {
  return {
    value: null,
    loading: true,
    error: null,
    retry: () => {},
    ...overrides,
  } as Resource<T>
}

// Tests ----------------------------------------------------------------------

describe('toSessionSyncView', () => {
  it('maps synced idle status to a synced view with rate pills', () => {
    const view = toSessionSyncView(fakeSyncStatus())
    expect(view.state).toBe('synced')
    expect(view.title).toBe('Synced')
    expect(view.rate?.up).toBe('0 B/s')
    expect(view.rate?.down).toBe('0 B/s')
  })

  it('maps an active state to an active view', () => {
    const view = toSessionSyncView(
      fakeSyncStatus({
        visualState: 'active',
        active: true,
        summaryLabel: 'Uploading changes',
        detailLabel: 'Uploading to cloud.',
        uploadRateLabel: '1.5 MiB/s',
      }),
    )
    expect(view.state).toBe('active')
    expect(view.title).toBe('Uploading changes')
    expect(view.rate?.up).toBe('1.5 MiB/s')
  })

  it('maps error state to an error view with the last error text', () => {
    const view = toSessionSyncView(
      fakeSyncStatus({
        visualState: 'error',
        error: true,
        summaryLabel: 'Sync needs attention',
        detailLabel: 'Transport disconnected.',
        lastError: 'WebSocket closed (1006).',
      }),
    )
    expect(view.state).toBe('error')
    expect(view.error).toBe('WebSocket closed (1006).')
  })
})

describe('toSharedObjectView', () => {
  it('renders a loading view when health is null', () => {
    const view = toSharedObjectView(null)
    expect(view.state).toBe('loading')
    expect(view.title).toBe('Loading space')
  })

  it('renders an active view for LOADING status on the SO layer', () => {
    const view = toSharedObjectView({
      status: SharedObjectHealthStatus.LOADING,
      layer: SharedObjectHealthLayer.SHARED_OBJECT,
      commonReason: SharedObjectHealthCommonReason.UNKNOWN,
      remediationHint: SharedObjectHealthRemediationHint.NONE,
      error: '',
    })
    expect(view.state).toBe('active')
    expect(view.title).toBe('Loading space')
  })

  it('renders a synced view for READY status', () => {
    const view = toSharedObjectView({
      status: SharedObjectHealthStatus.READY,
      layer: SharedObjectHealthLayer.SHARED_OBJECT,
      commonReason: SharedObjectHealthCommonReason.UNKNOWN,
      remediationHint: SharedObjectHealthRemediationHint.NONE,
      error: '',
    })
    expect(view.state).toBe('synced')
  })

  it('renders an error view with the message for NOT_FOUND closed state', () => {
    const view = toSharedObjectView({
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.SHARED_OBJECT,
      commonReason: SharedObjectHealthCommonReason.NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
      error: 'shared object not found',
    })
    expect(view.state).toBe('error')
    expect(view.title).toBe('Shared object not found')
    expect(view.error).toBe('shared object not found')
  })
})

describe('toPairingView', () => {
  it('shows a stage-specific detail for VERIFYING_EMOJI', () => {
    const view = toPairingView({
      status: PairingStatus.PairingStatus_VERIFYING_EMOJI,
    })
    expect(view.state).toBe('active')
    expect(view.title).toBe('Verify the emoji sequence')
  })

  it('flips to synced for VERIFIED', () => {
    const view = toPairingView({ status: PairingStatus.PairingStatus_VERIFIED })
    expect(view.state).toBe('synced')
  })

  it('flips to error on CONNECTION_TIMEOUT with supplied errorMessage', () => {
    const view = toPairingView({
      status: PairingStatus.PairingStatus_CONNECTION_TIMEOUT,
      errorMessage: 'timeout waiting for peer',
    })
    expect(view.state).toBe('error')
    expect(view.error).toBe('timeout waiting for peer')
  })

  it('embeds the pairing code in detail for CODE_GENERATED', () => {
    const view = toPairingView({
      status: PairingStatus.PairingStatus_CODE_GENERATED,
      pairingCode: 'ABCD-1234',
    })
    expect(view.detail).toContain('ABCD-1234')
  })
})

describe('toLockView', () => {
  it('is synced when the session is unlocked', () => {
    expect(toLockView({ locked: false }).state).toBe('synced')
  })

  it('is loading when PIN unlock is pending', () => {
    const view = toLockView({
      mode: SessionLockMode.PIN_ENCRYPTED,
      locked: true,
    })
    expect(view.state).toBe('loading')
    expect(view.detail).toContain('PIN')
  })

  it('is active while unlocking', () => {
    const view = toLockView({
      mode: SessionLockMode.PIN_ENCRYPTED,
      locked: true,
      unlocking: true,
    })
    expect(view.state).toBe('active')
  })

  it('surfaces the error message on unlock failure', () => {
    const view = toLockView({
      mode: SessionLockMode.PIN_ENCRYPTED,
      locked: true,
      errorMessage: 'invalid PIN',
    })
    expect(view.state).toBe('error')
    expect(view.error).toBe('invalid PIN')
  })
})

describe('toUnixFSPathView', () => {
  const loading = fakeResource({ loading: true })
  const done = fakeResource({ value: {} as unknown, loading: false })

  it('surfaces the first pending stage in detail', () => {
    const view = toUnixFSPathView({
      root: done,
      lookup: loading,
      stat: null,
      entries: null,
      path: '/foo',
    })
    expect(view.state).toBe('active')
    expect(view.detail).toContain('path')
    expect(view.detail).toContain('/foo')
  })

  it('is synced when all four stages resolve', () => {
    const view = toUnixFSPathView({
      root: done,
      lookup: done,
      stat: done,
      entries: done,
    })
    expect(view.state).toBe('synced')
  })

  it('is error when any stage errors, surfacing the stage name', () => {
    const err = fakeResource({
      loading: false,
      error: new Error('block not found'),
    })
    const view = toUnixFSPathView({
      root: done,
      lookup: done,
      stat: err,
      entries: null,
    })
    expect(view.state).toBe('error')
    expect(view.title).toContain('Reading metadata')
    expect(view.error).toBe('block not found')
  })
})

describe('toWizardView', () => {
  it('is loading when no state and loading is true', () => {
    const view = toWizardView({ state: null, loading: true })
    expect(view.state).toBe('loading')
  })

  it('shows Step N of M for determinate wizards', () => {
    const view = toWizardView({
      state: { step: 1, targetTypeId: 'git/repo' },
      loading: false,
      totalSteps: 3,
      activeStepLabel: 'Clone origin',
    })
    expect(view.state).toBe('active')
    expect(view.detail).toContain('Step 2 of 3')
    expect(view.detail).toContain('Clone origin')
  })

  it('surfaces errors with retry support', () => {
    const view = toWizardView({
      state: null,
      loading: false,
      errorMessage: 'failed to load',
    })
    expect(view.state).toBe('error')
    expect(view.error).toBe('failed to load')
  })
})

describe('toResourceView', () => {
  it('defaults to loading when the resource is null', () => {
    const view = toResourceView(null, { title: 'Org list' })
    expect(view.state).toBe('loading')
  })

  it('is active while the resource is loading', () => {
    const view = toResourceView(fakeResource({ loading: true }), {
      title: 'Org list',
      loadingDetail: 'Fetching orgs',
    })
    expect(view.state).toBe('active')
    expect(view.detail).toBe('Fetching orgs')
  })

  it('is error when the resource errors', () => {
    const view = toResourceView(
      fakeResource({
        loading: false,
        error: new Error('network error'),
      }),
      { title: 'Org list' },
    )
    expect(view.state).toBe('error')
    expect(view.error).toBe('network error')
  })

  it('is synced when the resource value is loaded', () => {
    const view = toResourceView(
      fakeResource({
        loading: false,
        value: { orgs: [] } as unknown,
      }),
      { title: 'Org list' },
    )
    expect(view.state).toBe('synced')
  })
})
