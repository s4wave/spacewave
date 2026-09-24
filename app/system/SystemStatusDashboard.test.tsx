import { beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import type { SessionSyncStatusView } from '@s4wave/app/session/SessionSyncStatusContext.js'
import { StateNamespaceProvider, atom } from '@s4wave/web/state/index.js'

import { SystemStatusDashboard } from './SystemStatusDashboard.js'
import { buildSystemModel, type SystemModelInputs } from './useSystemModel.js'

const mocks = vi.hoisted(() => ({
  inputs: null as SystemModelInputs | null,
  navigate: vi.fn(),
  navigateSession: vi.fn(),
  setOpenMenu: vi.fn(),
}))

vi.mock('./useSystemModel.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./useSystemModel.js')>()
  return {
    ...actual,
    useSystemModel: () => actual.buildSystemModel(mocks.inputs!),
  }
})

vi.mock('@s4wave/app/hooks/useSessionMetadata.js', () => ({
  useSessionMetadata: (sessionIndex: number) => ({
    displayName: `Account ${sessionIndex}`,
    providerDisplayName: 'Local',
    providerAccountId: `acct-${sessionIndex}`,
  }),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: () => 1,
  useSessionNavigate: () => mocks.navigateSession,
}))

vi.mock('@s4wave/web/frame/bottom-bar-context.js', () => ({
  useBottomBarSetOpenMenu: () => mocks.setOpenMenu,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mocks.navigate,
}))

vi.mock('@s4wave/web/devtools/index.js', () => ({
  useSelectedResourceId: () => null,
  useTrackedResources: () => new Map([[1, {}]]),
}))

vi.mock('@s4wave/web/devtools/useStateInspectorEntries.js', () => ({
  useStateInspectorEntryMap: () => new Map(),
}))

vi.mock('@s4wave/web/devtools/StateDevToolsContext.js', () => ({
  useSelectedStateAtomId: () => null,
}))

vi.mock('@s4wave/web/devtools/ResourceTreeTab.js', () => ({
  ResourceTreeTab: () => <div>Resource Tree</div>,
}))

vi.mock('@s4wave/web/devtools/StateTreeTab.js', () => ({
  StateTreeTab: () => <div>State Tree</div>,
}))

// syncView returns an idle sync view with every label filled.
function syncView(
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
    detailLabel: 'Everything is up to date.',
    ariaLabel: 'Synced',
    transportLabel: 'Cloud relay',
    p2pLabel: 'Off',
    uploadRateLabel: '0 B/s',
    downloadRateLabel: '0 B/s',
    pendingUploadLabel: '0',
    activeUploadLabel: '0',
    pendingDownloadLabel: '0',
    packRangeLabel: '',
    packIndexTailLabel: '',
    packLookupLabel: '',
    packIndexCacheLabel: '',
    lastActivityLabel: '',
    lastError: '',
    localCopies: [],
    peerUploadLabel: '',
    peerDownloadLabel: '',
    peers: [],
    ...overrides,
  } as SessionSyncStatusView
}

// healthyInputs returns a fully loaded system with nothing wrong.
function healthyInputs(): SystemModelInputs {
  const sync = syncView()
  return {
    build: {
      mainVersion: '0.1.0',
      version: '0.1.0',
      goVersion: 'go1.26',
      goos: 'js',
      goarch: 'wasm',
      runtimeLabel: 'Browser',
      cornerLabel: '',
      browserGenerationId: 'gen-1',
    },
    sync,
    storage: {
      providerLoading: false,
      providerSupported: true,
      providerBytes: 4096n,
      blockCount: 12n,
      browserReadFailed: false,
      originUsageBytes: 100,
      originQuotaBytes: 1000,
      protectionState: 'protected',
      sync,
      safariCleanupRisk: false,
      requestProtection: async () => {},
    },
    network: {
      transportRunning: true,
      localPeerId: 'local-peer-1',
      peerCount: 1,
      linkCount: 1,
      peers: [],
    },
    controllers: {
      controllerCount: 1,
      controllers: [{ id: 'controller/a', version: '1' }],
    },
    directives: {
      directiveCount: 3,
      directives: [
        { name: 'LookupRpcService', ident: 'svc-a' },
        { name: 'LookupRpcService', ident: 'svc-b' },
        { name: 'EstablishLink', ident: 'peer-1' },
      ],
    },
    plugins: {
      pluginCount: 1,
      plugins: [{ id: 'spacewave-app', instanceKey: '', state: 'running' }],
    },
    recovery: { launcher: { updatePhase: 'idle' } },
    sessions: [{ sessionIndex: 1 }],
    spaces: [
      {
        entry: { ref: { providerResourceRef: { id: 'space-1' } } },
        spaceMeta: { name: 'Primary Space' },
      },
    ],
  }
}

function renderDashboard(onClose = vi.fn()) {
  render(
    <StateNamespaceProvider rootAtom={atom({})}>
      <SystemStatusDashboard onClose={onClose} />
    </StateNamespaceProvider>,
  )
  return onClose
}

describe('buildSystemModel', () => {
  it('reports a healthy system as nominal', () => {
    const model = buildSystemModel(healthyInputs())

    expect(model.verdict).toEqual({
      tone: 'nominal',
      headline: 'All systems nominal',
      items: [],
    })
    expect(model.directiveCount).toBe(3)
    expect(model.directives?.[0]).toEqual({
      name: 'LookupRpcService',
      idents: ['svc-a', 'svc-b'],
    })
  })

  it('waits for every watch before calling the system nominal', () => {
    const model = buildSystemModel({ ...healthyInputs(), network: null })

    expect(model.tones.network).toBe('pending')
    expect(model.verdict.headline).toBe('Reading system state')
  })

  it('lists faults before warnings and marks their subsystems', () => {
    const inputs = healthyInputs()
    inputs.network = { ...inputs.network, transportRunning: false }
    inputs.recovery = {
      launcher: { updatePhase: 'error', updateError: 'signature mismatch' },
    }
    const model = buildSystemModel(inputs)

    expect(model.verdict.tone).toBe('error')
    expect(model.verdict.headline).toBe('2 things need attention')
    expect(model.verdict.items.map((item) => item.key)).toEqual([
      'release-update',
      'network-stopped',
    ])
    expect(model.tones.release).toBe('error')
    expect(model.tones.network).toBe('warning')
    expect(model.tones.sync).toBe('nominal')
  })
})

describe('SystemStatusDashboard', () => {
  beforeEach(() => {
    cleanup()
    mocks.inputs = healthyInputs()
    mocks.navigate.mockReset()
    mocks.navigateSession.mockReset()
    mocks.setOpenMenu.mockReset()
  })

  it('shows the verdict and a live tile for every subsystem', () => {
    renderDashboard()

    expect(screen.getByRole('status').textContent).toBe('All systems nominal')
    for (const title of [
      'Sync',
      'Storage',
      'Network',
      'Runtime',
      'Release',
      'Spaces & Accounts',
    ]) {
      expect(
        screen.getByRole('button', {
          name: new RegExp(`^${title}\\b.*Open inspector`),
        }),
      ).toBeTruthy()
    }
    expect(screen.getByText('1 of 1 plugin running')).toBeTruthy()
  })

  it('opens an inspector in place and returns to its tile on Escape', () => {
    const onClose = renderDashboard()
    const tile = screen.getByRole('button', {
      name: /^Runtime\b.*Open inspector/,
    })

    fireEvent.click(tile)
    expect(
      screen.getByRole('heading', { level: 2, name: 'Runtime' }),
    ).toBeTruthy()
    expect(screen.getByText('spacewave-app')).toBeTruthy()

    fireEvent.keyDown(document.activeElement!, { key: 'Escape' })
    expect(screen.queryByRole('heading', { level: 2, name: 'Runtime' })).toBe(
      null,
    )
    expect(document.activeElement).toBe(
      screen.getByRole('button', { name: /^Runtime\b.*Open inspector/ }),
    )
    expect(onClose).not.toHaveBeenCalled()
  })

  it('links each attention item to the inspector that explains it', () => {
    mocks.inputs!.network = {
      ...mocks.inputs!.network,
      transportRunning: false,
    }
    renderDashboard()

    expect(screen.getByRole('status').textContent).toBe(
      '1 thing needs attention',
    )
    fireEvent.click(
      screen.getByRole('button', { name: /Peer transport is not running/ }),
    )
    expect(
      screen.getByRole('heading', { level: 2, name: 'Network' }),
    ).toBeTruthy()
  })

  it('opens the resource tree from under the hood', () => {
    renderDashboard()

    fireEvent.click(screen.getByRole('button', { name: /Resources/ }))
    expect(screen.getByText('Resource Tree')).toBeTruthy()
  })
})
