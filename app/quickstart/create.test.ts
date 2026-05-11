import { beforeEach, describe, expect, it, vi } from 'vitest'

import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import {
  INIT_UNIXFS_OP_ID,
  UNIXFS_OBJECT_KEY,
} from '@s4wave/core/space/world/ops/init-unixfs.js'
import {
  InitCanvasDemoOp,
  InitUnixFSOp,
  SetSpaceSettingsOp,
} from '@s4wave/core/space/world/ops/ops.pb.js'
import {
  CANVAS_DEMO_OBJECT_KEY,
  INIT_CANVAS_DEMO_OP_ID,
} from '@s4wave/core/space/world/ops/init-canvas-demo.js'
import {
  V86WizardConfig,
  V86WizardConfig_Source,
} from '@s4wave/sdk/vm/v86-wizard.pb.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { InitChatDemoOp } from '@s4wave/sdk/chat/chat.pb.js'
import {
  CHAT_DEMO_CHANNEL_KEY,
  INIT_CHAT_DEMO_OP_ID,
} from '@s4wave/sdk/chat/init-chat-demo.js'
import { InitForgeQuickstartOp } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import { INIT_FORGE_QUICKSTART_OP_ID } from '@s4wave/sdk/forge/dashboard/init-forge-quickstart.js'
import { BLOG_OBJECT_KEY } from '../../plugin/notes/proto/create-blog.js'
import { InitDriveOp } from '@s4wave/sdk/space/drive/drive.pb.js'
import {
  DRIVE_OBJECT_KEY,
  INIT_DRIVE_OP_ID,
} from '@s4wave/sdk/space/drive/drive.js'
import { UnixFSTypeID } from '@s4wave/web/hooks/useUnixFSHandle.js'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { QuickstartSpaceCreateId } from './options.js'
import {
  approveSpacePlugins,
  createLocalSession,
  createQuickstartSetup,
  createDrive,
  createSpaceSettingsObject,
  executeDynamicQuickstart,
  getQuickstartSpaceName,
  populateSpace,
  type QuickstartProgressState,
} from './create.js'
import { NOTEBOOK_OBJECT_KEY } from '../../plugin/notes/proto/init-notebook.js'

const seedMocks = vi.hoisted(() => ({
  createBlogClientSide: vi.fn().mockResolvedValue(undefined),
  createDocsClientSide: vi.fn().mockResolvedValue(undefined),
  createNotebookClientSide: vi.fn().mockResolvedValue(undefined),
}))

const quickstartRegistryMocks = vi.hoisted(() => ({
  ExecuteQuickstart: vi.fn(),
}))

const localProviderMocks = vi.hoisted(() => ({
  createAccount: vi.fn(),
}))

const spaceMocks = vi.hoisted(() => ({
  mountSpace: vi.fn(),
}))

vi.mock('../../plugin/notes/blog-seed.js', () => ({
  createBlogClientSide: seedMocks.createBlogClientSide,
}))

vi.mock('../../plugin/notes/content-seed.js', () => ({
  createDocsClientSide: seedMocks.createDocsClientSide,
  createNotebookClientSide: seedMocks.createNotebookClientSide,
}))

vi.mock('@s4wave/sdk/quickstart/registry/registry_srpc.pb.js', () => ({
  QuickstartRegistryResourceServiceClient: vi.fn(function () {
    return quickstartRegistryMocks
  }),
}))

vi.mock('@s4wave/sdk/provider/local/local.js', () => ({
  LocalProvider: vi.fn(function () {
    return localProviderMocks
  }),
}))

vi.mock('@s4wave/app/space/space.js', () => ({
  mountSpace: spaceMocks.mountSpace,
}))

type ApplyWorldOp = (
  opTypeId: string,
  opData: Uint8Array,
  sender?: string,
  abortSignal?: AbortSignal,
) => Promise<{ seqno: bigint; sysErr: boolean }>

function buildQuickstartWorld() {
  const applyWorldOp = vi.fn<ApplyWorldOp>().mockResolvedValue({
    seqno: 1n,
    sysErr: false,
  })
  const releaseCursor = vi.fn()
  return {
    world: {
      applyWorldOp,
      getObject: vi.fn().mockResolvedValue(null),
      lookupGraphQuads: vi.fn().mockResolvedValue({ quads: [] }),
      deleteGraphQuad: vi.fn().mockResolvedValue(undefined),
      setGraphQuad: vi.fn().mockResolvedValue(undefined),
      buildStorageCursor: vi.fn(() =>
        Promise.resolve({
          putBlock: vi.fn().mockResolvedValue({ ref: {} }),
          getRef: vi.fn().mockResolvedValue({ ref: {} }),
          release: releaseCursor,
          [Symbol.dispose]: releaseCursor,
        }),
      ),
      createObject: vi.fn().mockResolvedValue({}),
    },
    applyWorldOp,
  }
}

function getSettingsIndexPath(applyWorldOp: ReturnType<typeof vi.fn>) {
  const call = applyWorldOp.mock.calls.find(
    (call) => call[0] === SET_SPACE_SETTINGS_OP_ID,
  )
  if (!call) {
    throw new Error('expected settings op call')
  }
  return (
    SetSpaceSettingsOp.fromBinary(call[1] as Uint8Array).settings?.indexPath ??
    ''
  )
}

describe('quickstart create', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
    localProviderMocks.createAccount.mockReset()
    spaceMocks.mountSpace.mockReset()
  })

  it('skips existing session lookup when local storage has no session hint', async () => {
    const storage = new Map<string, string>()
    const localStorage = {
      getItem: vi.fn((key: string) => storage.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => {
        storage.set(key, value)
      }),
      removeItem: vi.fn((key: string) => {
        storage.delete(key)
      }),
    }
    vi.stubGlobal('localStorage', localStorage)
    localProviderMocks.createAccount.mockResolvedValue({
      sessionListEntry: {
        sessionIndex: 7,
        sessionRef: { providerResourceRef: { providerId: 'local' } },
      },
    })
    const root = {
      listSessions: vi.fn().mockResolvedValue({ sessions: [] }),
      lookupProvider: vi.fn().mockResolvedValue({
        resourceRef: { providerId: 'local' },
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
      mountSessionByIdx: vi.fn(),
      mountSession: vi.fn().mockResolvedValue({
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
    }
    const cleanup: RegisterCleanup = (value) => value

    await createLocalSession(
      root as never,
      new AbortController().signal,
      cleanup,
    )

    expect(root.listSessions).not.toHaveBeenCalled()
    expect(root.lookupProvider).toHaveBeenCalledWith('local')
    expect(localProviderMocks.createAccount).toHaveBeenCalled()
    expect(root.mountSession).toHaveBeenCalledWith(
      {
        sessionRef: { providerResourceRef: { providerId: 'local' } },
      },
      expect.any(AbortSignal),
    )
  })

  it('mounts the first local session when first-run create account aborts after side effects', async () => {
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
    })
    const sessionRef = { providerResourceRef: { providerId: 'local' } }
    localProviderMocks.createAccount.mockRejectedValue(
      new Error('ERR_RPC_ABORT'),
    )
    const session = {
      release: vi.fn(),
      [Symbol.dispose]: vi.fn(),
    }
    const root = {
      listSessions: vi.fn().mockResolvedValue({
        sessions: [
          {
            sessionIndex: 8,
            sessionRef,
          },
        ],
      }),
      lookupProvider: vi.fn().mockResolvedValue({
        resourceRef: { providerId: 'local' },
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
      mountSessionByIdx: vi.fn().mockResolvedValue({
        session,
        sessionRef,
      }),
      mountSession: vi.fn().mockResolvedValue({
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
    }
    const cleanup: RegisterCleanup = (value) => value

    const setup = await createLocalSession(
      root as never,
      new AbortController().signal,
      cleanup,
    )

    expect(setup.sessionIndex).toBe(1)
    expect(localProviderMocks.createAccount).toHaveBeenCalledTimes(1)
    expect(root.listSessions).not.toHaveBeenCalled()
    expect(root.mountSessionByIdx).toHaveBeenCalledWith(
      { sessionIdx: 1 },
      expect.any(AbortSignal),
    )
    expect(root.mountSession).not.toHaveBeenCalled()
  })

  it('executes dynamic quickstarts through the registry and applies returned routing', async () => {
    quickstartRegistryMocks.ExecuteQuickstart.mockResolvedValue({
      indexPath: 'glados/org-chart',
      pluginIds: ['glados-core', 'glados-web'],
    })
    const { world, applyWorldOp } = buildQuickstartWorld()
    const spaceContents = {
      setPluginApproval: vi.fn().mockResolvedValue({}),
    }

    await executeDynamicQuickstart(
      { client: {} } as never,
      'glados-workspace',
      {
        space: { id: 42 },
        spaceWorld: world,
        spaceContents,
      } as never,
    )

    expect(quickstartRegistryMocks.ExecuteQuickstart).toHaveBeenCalledWith(
      { quickstartId: 'glados-workspace', spaceResourceId: 42 },
      undefined,
    )
    expect(getSettingsIndexPath(applyWorldOp)).toBe('glados/org-chart')
    const settingsCall = applyWorldOp.mock.calls.find(
      (call) => call[0] === SET_SPACE_SETTINGS_OP_ID,
    )
    const settings = SetSpaceSettingsOp.fromBinary(settingsCall?.[1]).settings
    expect(settings?.pluginIds).toEqual(['glados-core', 'glados-web'])
    expect(spaceContents.setPluginApproval).toHaveBeenCalledTimes(2)
    expect(spaceContents.setPluginApproval).toHaveBeenCalledWith(
      'glados-core',
      true,
      undefined,
    )
    expect(spaceContents.setPluginApproval).toHaveBeenCalledWith(
      'glados-web',
      true,
      undefined,
    )
  })

  it('maps quickstarts to friendly seeded space names', () => {
    const cases: [QuickstartSpaceCreateId, string][] = [
      ['space', 'My Space'],
      ['drive', 'My Drive'],
      ['git', 'My Git Repository'],
      ['notebook', 'My Notebook'],
      ['canvas', 'My Canvas'],
      ['chat', 'My Chat'],
      ['docs', 'My Docs'],
      ['blog', 'My Blog'],
      ['v86', 'My V86 VM'],
      ['forge', 'My Forge Dashboard'],
    ]

    for (const [quickstartId, name] of cases) {
      expect(getQuickstartSpaceName(quickstartId)).toBe(name)
    }
  })

  it('publishes quickstart progress-ready before Drive content seeding finishes', async () => {
    vi.stubGlobal('__s4waveQuickstartTiming', undefined)
    vi.stubGlobal('__s4wave_debug', {})
    const abortSignal = new AbortController().signal
    const cleanup: RegisterCleanup = (value) => value
    localProviderMocks.createAccount.mockResolvedValue({
      sessionListEntry: {
        sessionIndex: 3,
        sessionRef: { providerResourceRef: { providerId: 'local' } },
      },
    })
    const root = {
      listSessions: vi.fn().mockResolvedValue({ sessions: [] }),
      lookupProvider: vi.fn().mockResolvedValue({
        resourceRef: { providerId: 'local' },
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
      mountSession: vi.fn().mockResolvedValue({
        createSpace: vi.fn().mockResolvedValue({
          sharedObjectRef: { providerResourceRef: { id: 'space-1' } },
        }),
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
    }
    const { world, applyWorldOp } = buildQuickstartWorld()
    const spaceWorldState = {
      release: vi.fn(),
      [Symbol.dispose]: vi.fn(),
    }
    const spaceContents = {
      release: vi.fn(),
      [Symbol.dispose]: vi.fn(),
    }
    Object.assign(world, {
      accessWorldState: vi.fn().mockResolvedValue(spaceWorldState),
    })
    spaceMocks.mountSpace.mockResolvedValue({
      accessWorldState: vi.fn().mockResolvedValue(world),
      mountSpaceContents: vi.fn().mockResolvedValue(spaceContents),
    })

    const progressEvents: QuickstartProgressState[] = []

    await createQuickstartSetup(
      root as never,
      'drive',
      abortSignal,
      cleanup,
      (state) => {
        progressEvents.push(state)
      },
    )

    const timing = globalThis.__s4waveQuickstartTiming
    expect(timing?.state).toBe('content-ready')
    expect(timing?.progressReadyMs).toEqual(expect.any(Number))
    expect(timing?.contentReadyMs).toEqual(timing?.finishedMs)
    expect(timing?.finishedMs).toEqual(expect.any(Number))
    expect(timing?.finishedMs ?? 0).toBeGreaterThanOrEqual(
      timing?.progressReadyMs ?? 0,
    )
    const populatePhase = timing?.phases.find(
      (phase) => phase.name === 'populate-space',
    )
    expect(populatePhase?.startedMs ?? 0).toBeGreaterThanOrEqual(
      timing?.progressReadyMs ?? 0,
    )
    expect(applyWorldOp.mock.calls[0]?.[0]).toBe(INIT_UNIXFS_OP_ID)
    expect(globalThis.__s4wave_debug?.quickstartTiming?.progressReadyMs).toBe(
      timing?.progressReadyMs,
    )
    expect(progressEvents.map((event) => event.step)).toEqual(
      expect.arrayContaining(['session', 'space', 'frame', 'content']),
    )
    expect(
      progressEvents.findIndex((event) => event.step === 'content'),
    ).toBeGreaterThan(
      progressEvents.findIndex((event) => event.step === 'frame'),
    )
    expect(progressEvents.at(-1)).toEqual(
      expect.objectContaining({
        step: 'content',
        detail: 'Seeding My Drive content',
      }),
    )
  })

  it('records a setup error after progress-ready without marking content-ready', async () => {
    vi.stubGlobal('__s4waveQuickstartTiming', undefined)
    vi.stubGlobal('__s4wave_debug', {})
    const abortSignal = new AbortController().signal
    const cleanup: RegisterCleanup = (value) => value
    localProviderMocks.createAccount.mockResolvedValue({
      sessionListEntry: {
        sessionIndex: 3,
        sessionRef: { providerResourceRef: { providerId: 'local' } },
      },
    })
    const root = {
      listSessions: vi.fn().mockResolvedValue({ sessions: [] }),
      lookupProvider: vi.fn().mockResolvedValue({
        resourceRef: { providerId: 'local' },
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
      mountSession: vi.fn().mockResolvedValue({
        createSpace: vi.fn().mockResolvedValue({
          sharedObjectRef: { providerResourceRef: { id: 'space-1' } },
        }),
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
    }
    const { world, applyWorldOp } = buildQuickstartWorld()
    const seedError = new Error('drive seed failed')
    applyWorldOp.mockRejectedValue(seedError)
    Object.assign(world, {
      accessWorldState: vi.fn().mockResolvedValue({
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
    })
    spaceMocks.mountSpace.mockResolvedValue({
      accessWorldState: vi.fn().mockResolvedValue(world),
      mountSpaceContents: vi.fn().mockResolvedValue({
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
    })

    await expect(
      createQuickstartSetup(root as never, 'drive', abortSignal, cleanup),
    ).rejects.toThrow('drive seed failed')

    const timing = globalThis.__s4waveQuickstartTiming
    expect(timing?.state).toBe('error')
    expect(timing?.progressReadyMs).toEqual(expect.any(Number))
    expect(timing?.contentReadyMs).toBeUndefined()
    expect(timing?.finishedMs).toEqual(expect.any(Number))
    expect(timing?.error).toBe('drive seed failed')
    expect(globalThis.__s4wave_debug?.quickstartTiming?.state).toBe('error')
  })

  it('records aborted setup as cancelled without progress-ready or content-ready', async () => {
    vi.stubGlobal('__s4waveQuickstartTiming', undefined)
    vi.stubGlobal('__s4wave_debug', {})
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
    })
    const abort = new AbortController()
    abort.abort()
    const cleanup: RegisterCleanup = (value) => value
    localProviderMocks.createAccount.mockRejectedValue(
      new DOMException('Aborted', 'AbortError'),
    )
    const root = {
      listSessions: vi.fn().mockResolvedValue({ sessions: [] }),
      lookupProvider: vi.fn().mockResolvedValue({
        resourceRef: { providerId: 'local' },
        release: vi.fn(),
        [Symbol.dispose]: vi.fn(),
      }),
      mountSession: vi.fn(),
    }

    await expect(
      createQuickstartSetup(root as never, 'drive', abort.signal, cleanup),
    ).rejects.toThrow('Aborted')

    const timing = globalThis.__s4waveQuickstartTiming
    expect(timing?.state).toBe('cancelled')
    expect(timing?.progressReadyMs).toBeUndefined()
    expect(timing?.contentReadyMs).toBeUndefined()
    expect(timing?.finishedMs).toEqual(expect.any(Number))
    expect(timing?.error).toBe('Aborted')
    expect(globalThis.__s4wave_debug?.quickstartTiming?.state).toBe('cancelled')
  })

  it('creates Drive storage before pointing the index at the Drive object', async () => {
    const putBlock = vi.fn((_arg: { data: Uint8Array }) =>
      Promise.resolve({ ref: {} }),
    )
    const getRef = vi.fn().mockResolvedValue({ ref: {} })
    const releaseCursor = vi.fn()
    const applyWorldOp = vi.fn<ApplyWorldOp>().mockResolvedValue({
      seqno: 1n,
      sysErr: false,
    })
    const spaceWorld = {
      getObject: vi.fn(() => Promise.resolve(null)),
      buildStorageCursor: vi.fn(() =>
        Promise.resolve({
          putBlock,
          getRef,
          release: releaseCursor,
          [Symbol.dispose]: releaseCursor,
        }),
      ),
      createObject: vi.fn().mockResolvedValue({}),
      lookupGraphQuads: vi.fn().mockResolvedValue({ quads: [] }),
      setGraphQuad: vi.fn().mockResolvedValue(undefined),
      applyWorldOp,
    }

    await createDrive(spaceWorld as never)

    expect(applyWorldOp).toHaveBeenCalledTimes(3)
    expect(applyWorldOp.mock.calls[0]?.[0]).toBe(INIT_UNIXFS_OP_ID)
    expect(applyWorldOp.mock.calls[1]?.[0]).toBe(INIT_DRIVE_OP_ID)

    const drive = InitDriveOp.fromBinary(applyWorldOp.mock.calls[1]?.[1])
    expect(drive.objectKey).toBe(DRIVE_OBJECT_KEY)
    expect(drive.roots?.[0]).toMatchObject({
      rootId: 'default',
      name: 'My Files',
      rootObjectKey: UNIXFS_OBJECT_KEY,
      rootType: UnixFSTypeID,
    })

    const settingsCall = applyWorldOp.mock.calls[2]
    if (!settingsCall) {
      throw new Error('expected settings op call')
    }
    expect(settingsCall[0]).toBe(SET_SPACE_SETTINGS_OP_ID)
    const settings = SetSpaceSettingsOp.fromBinary(settingsCall[1]).settings
    if (!settings) {
      throw new Error('expected settings')
    }
    expect(settings.indexPath).toBe(DRIVE_OBJECT_KEY)
  })

  it('indexes every quickstart to the object it creates or seeds', async () => {
    {
      const { world, applyWorldOp } = buildQuickstartWorld()
      await populateSpace('space', { spaceWorld: world } as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe('')
    }
    {
      const { world, applyWorldOp } = buildQuickstartWorld()
      await populateSpace('drive', { spaceWorld: world } as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe(DRIVE_OBJECT_KEY)
      const unixfsCall = applyWorldOp.mock.calls.find(
        (call) => call[0] === INIT_UNIXFS_OP_ID,
      )
      expect(InitUnixFSOp.fromBinary(unixfsCall?.[1]).objectKey).toBe(
        UNIXFS_OBJECT_KEY,
      )
      const driveCall = applyWorldOp.mock.calls.find(
        (call) => call[0] === INIT_DRIVE_OP_ID,
      )
      const drive = InitDriveOp.fromBinary(driveCall?.[1])
      expect(drive.objectKey).toBe(DRIVE_OBJECT_KEY)
      expect(drive.roots?.[0]?.rootObjectKey).toBe(UNIXFS_OBJECT_KEY)
      const settingsIndex = applyWorldOp.mock.calls.findIndex(
        (call) => call[0] === SET_SPACE_SETTINGS_OP_ID,
      )
      const driveIndex = applyWorldOp.mock.calls.findIndex(
        (call) => call[0] === INIT_DRIVE_OP_ID,
      )
      expect(driveIndex).toBeGreaterThanOrEqual(0)
      expect(settingsIndex).toBeGreaterThan(driveIndex)
    }
    {
      seedMocks.createNotebookClientSide.mockClear()
      const { world, applyWorldOp } = buildQuickstartWorld()
      await populateSpace('notebook', { spaceWorld: world } as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe(NOTEBOOK_OBJECT_KEY)
      expect(seedMocks.createNotebookClientSide).toHaveBeenCalledWith(
        world,
        NOTEBOOK_OBJECT_KEY,
        UNIXFS_OBJECT_KEY,
        'Notes',
        expect.any(Date),
        undefined,
      )
      expect(
        seedMocks.createNotebookClientSide.mock.invocationCallOrder[0],
      ).toBeLessThan(applyWorldOp.mock.invocationCallOrder[0] ?? 0)
    }
    {
      const { world, applyWorldOp } = buildQuickstartWorld()
      await populateSpace('canvas', { spaceWorld: world } as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe(CANVAS_DEMO_OBJECT_KEY)
      const canvasCall = applyWorldOp.mock.calls.find(
        (call) => call[0] === INIT_CANVAS_DEMO_OP_ID,
      )
      expect(
        InitCanvasDemoOp.fromBinary(canvasCall?.[1] as Uint8Array).objectKey,
      ).toBe(CANVAS_DEMO_OBJECT_KEY)
      const settingsIndex = applyWorldOp.mock.calls.findIndex(
        (call) => call[0] === SET_SPACE_SETTINGS_OP_ID,
      )
      const canvasIndex = applyWorldOp.mock.calls.findIndex(
        (call) => call[0] === INIT_CANVAS_DEMO_OP_ID,
      )
      expect(canvasIndex).toBeGreaterThanOrEqual(0)
      expect(settingsIndex).toBeGreaterThan(canvasIndex)
    }
    {
      const { world, applyWorldOp } = buildQuickstartWorld()
      await populateSpace('chat', { spaceWorld: world } as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe(CHAT_DEMO_CHANNEL_KEY)
      const chatCall = applyWorldOp.mock.calls.find(
        (call) => call[0] === INIT_CHAT_DEMO_OP_ID,
      )
      expect(
        InitChatDemoOp.fromBinary(chatCall?.[1] as Uint8Array).channelObjectKey,
      ).toBe(CHAT_DEMO_CHANNEL_KEY)
      const settingsIndex = applyWorldOp.mock.calls.findIndex(
        (call) => call[0] === SET_SPACE_SETTINGS_OP_ID,
      )
      const chatIndex = applyWorldOp.mock.calls.findIndex(
        (call) => call[0] === INIT_CHAT_DEMO_OP_ID,
      )
      expect(chatIndex).toBeGreaterThanOrEqual(0)
      expect(settingsIndex).toBeGreaterThan(chatIndex)
    }
    {
      seedMocks.createDocsClientSide.mockClear()
      const { world, applyWorldOp } = buildQuickstartWorld()
      await populateSpace('docs', { spaceWorld: world } as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe('documentation')
      expect(seedMocks.createDocsClientSide).toHaveBeenCalledWith(
        world,
        'documentation',
        'Documentation',
        '',
        expect.any(Date),
        undefined,
      )
      expect(
        seedMocks.createDocsClientSide.mock.invocationCallOrder[0],
      ).toBeLessThan(applyWorldOp.mock.invocationCallOrder[0] ?? 0)
    }
    {
      seedMocks.createBlogClientSide.mockClear()
      const { world, applyWorldOp } = buildQuickstartWorld()
      await populateSpace('blog', { spaceWorld: world } as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe(BLOG_OBJECT_KEY)
      expect(seedMocks.createBlogClientSide).toHaveBeenCalledWith(
        world,
        BLOG_OBJECT_KEY,
        'Blog',
        '',
        '',
        expect.any(Date),
        undefined,
      )
    }
    {
      const { world, applyWorldOp } = buildQuickstartWorld()
      await populateSpace('forge', {
        spaceWorld: world,
        session: {
          getSessionInfo: vi
            .fn()
            .mockResolvedValue({ peerId: '12D3KooWForgePeer' }),
        },
      } as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe('forge')
      const forgeCall = applyWorldOp.mock.calls.find(
        (call) => call[0] === INIT_FORGE_QUICKSTART_OP_ID,
      )
      expect(
        InitForgeQuickstartOp.fromBinary(forgeCall?.[1] as Uint8Array)
          .layoutKey,
      ).toBe('forge')
    }
  })

  it('overwrites an existing unreadable settings object instead of failing setup', async () => {
    const getBlock = vi.fn(() =>
      Promise.reject(new Error('object must be a block')),
    )
    const release = vi.fn()
    const markDirty = vi.fn().mockResolvedValue(undefined)
    const setBlock = vi.fn((_arg: { data: Uint8Array }) =>
      Promise.resolve(undefined),
    )
    const write = vi.fn().mockResolvedValue({ rootRef: {} })
    const existingCursorRelease = vi.fn()
    const blockCursorRelease = vi.fn()
    const txRelease = vi.fn()
    const getObject = vi.fn(() =>
      Promise.resolve({
        accessWorldState: vi
          .fn()
          .mockResolvedValueOnce({
            getBlock,
            release,
            [Symbol.dispose]: release,
          })
          .mockResolvedValueOnce({
            buildTransaction: vi.fn(() =>
              Promise.resolve({
                transaction: {
                  write,
                  release: txRelease,
                },
                cursor: {
                  markDirty,
                  setBlock,
                  release: blockCursorRelease,
                },
              }),
            ),
            getRef: vi.fn().mockResolvedValue({ ref: {} }),
            release: existingCursorRelease,
            [Symbol.dispose]: existingCursorRelease,
          }),
        setRootRef: vi.fn().mockResolvedValue(undefined),
        release,
        [Symbol.dispose]: release,
      }),
    )
    const spaceWorld = {
      applyWorldOp: vi.fn<ApplyWorldOp>().mockResolvedValue({
        seqno: 1n,
        sysErr: false,
      }),
      getObject,
      lookupGraphQuads: vi.fn().mockResolvedValue({ quads: [] }),
      createObject: vi.fn().mockResolvedValue({}),
      setGraphQuad: vi.fn().mockResolvedValue(undefined),
    }

    await createSpaceSettingsObject(spaceWorld as never, undefined, 'blog', [
      'spacewave-app',
    ])

    expect(getObject).toHaveBeenCalledWith('settings', undefined)
    expect(markDirty).not.toHaveBeenCalled()
    expect(write).not.toHaveBeenCalled()
    const settingsCall = spaceWorld.applyWorldOp.mock.calls[0]
    if (!settingsCall) {
      throw new Error('expected settings op call')
    }
    expect(settingsCall[0]).toBe(SET_SPACE_SETTINGS_OP_ID)
    const op = SetSpaceSettingsOp.fromBinary(settingsCall[1])
    const settings = op.settings
    if (!settings) {
      throw new Error('expected settings')
    }
    expect(op.objectKey).toBe('settings')
    expect(op.overwrite).toBe(true)
    expect(settings.indexPath).toBe('blog')
    expect(settings.pluginIds).toEqual(['spacewave-app'])
  })

  it('approves required space plugins once per unique plugin id', async () => {
    const setPluginApproval = vi.fn().mockResolvedValue({})

    await approveSpacePlugins({ setPluginApproval } as never, [
      'spacewave-app',
      'spacewave-app',
    ])

    expect(setPluginApproval).toHaveBeenCalledTimes(1)
    expect(setPluginApproval).toHaveBeenCalledWith(
      'spacewave-app',
      true,
      undefined,
    )
  })

  it('seeds the v86 quickstart as a persistent wizard and indexes the space to it', async () => {
    const putBlock = vi.fn((_arg: { data: Uint8Array }) =>
      Promise.resolve({ ref: {} }),
    )
    const getRef = vi.fn().mockResolvedValue({ ref: {} })
    const releaseCursor = vi.fn()
    const applyWorldOp = vi.fn<ApplyWorldOp>().mockResolvedValue({
      seqno: 1n,
      sysErr: false,
    })
    const createObject = vi.fn().mockResolvedValue({})
    const getObject = vi.fn().mockResolvedValue(null)
    const lookupGraphQuads = vi.fn().mockResolvedValue({ quads: [] })
    const setGraphQuad = vi.fn().mockResolvedValue(undefined)
    const deleteGraphQuad = vi.fn().mockResolvedValue(undefined)
    const spaceWorld = {
      applyWorldOp,
      getObject,
      lookupGraphQuads,
      deleteGraphQuad,
      setGraphQuad,
      buildStorageCursor: vi.fn(() =>
        Promise.resolve({
          putBlock,
          getRef,
          release: releaseCursor,
          [Symbol.dispose]: releaseCursor,
        }),
      ),
      createObject,
    }

    await populateSpace(
      'v86',
      {
        spaceWorld,
      } as never,
      undefined,
    )

    expect(applyWorldOp).toHaveBeenCalledTimes(2)
    const call = applyWorldOp.mock.calls[0]
    if (!call) {
      throw new Error('expected applyWorldOp call')
    }
    const opTypeId = call[0]
    const opData = call[1]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)
    const op = CreateWizardObjectOp.fromBinary(opData)
    expect(op.objectKey).toMatch(/^wizard\/v86-vm-[a-z0-9]+-\d+$/)
    expect(op.wizardTypeId).toBe('wizard/v86')
    expect(op.targetTypeId).toBe('v86')
    expect(op.targetKeyPrefix).toBe('vm/v86/')
    expect(op.initialStep).toBe(1)

    const cfg = V86WizardConfig.fromBinary(op.initialConfigData)
    expect(cfg.name ?? '').toBe('')
    expect(cfg.imageObjectKey).toBe('vm-image/default')
    expect(cfg.source).toBe(V86WizardConfig_Source.COPY_FROM_CDN)
    expect(cfg.cdnSourceObjectKey ?? '').toBe('')
    expect(cfg.cdnId ?? '').toBe('')
    expect(cfg.memoryMb).toBe(256)
    expect(cfg.vgaMemoryMb).toBe(8)

    const settingsCall = applyWorldOp.mock.calls[1]
    if (!settingsCall) {
      throw new Error('expected settings op call')
    }
    expect(settingsCall[0]).toBe(SET_SPACE_SETTINGS_OP_ID)
    const settings = SetSpaceSettingsOp.fromBinary(settingsCall[1]).settings
    if (!settings) {
      throw new Error('expected settings')
    }
    expect(settings.indexPath).toBe(op.objectKey)
  })

  it('seeds the git quickstart as a persistent create/clone wizard', async () => {
    const putBlock = vi.fn((_arg: { data: Uint8Array }) =>
      Promise.resolve({ ref: {} }),
    )
    const getRef = vi.fn().mockResolvedValue({ ref: {} })
    const releaseCursor = vi.fn()
    const applyWorldOp = vi.fn<ApplyWorldOp>().mockResolvedValue({
      seqno: 1n,
      sysErr: false,
    })
    const spaceWorld = {
      applyWorldOp,
      getObject: vi.fn().mockResolvedValue(null),
      lookupGraphQuads: vi.fn().mockResolvedValue({ quads: [] }),
      deleteGraphQuad: vi.fn().mockResolvedValue(undefined),
      setGraphQuad: vi.fn().mockResolvedValue(undefined),
      buildStorageCursor: vi.fn(() =>
        Promise.resolve({
          putBlock,
          getRef,
          release: releaseCursor,
          [Symbol.dispose]: releaseCursor,
        }),
      ),
      createObject: vi.fn().mockResolvedValue({}),
    }

    await populateSpace(
      'git',
      {
        spaceWorld,
      } as never,
      undefined,
    )

    expect(applyWorldOp).toHaveBeenCalledTimes(2)
    const call = applyWorldOp.mock.calls[0]
    if (!call) {
      throw new Error('expected applyWorldOp call')
    }
    const opTypeId = call[0]
    const opData = call[1]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)
    const op = CreateWizardObjectOp.fromBinary(opData)
    expect(op.objectKey).toMatch(/^wizard\/repository-[a-z0-9]+-\d+$/)
    expect(op.wizardTypeId).toBe('wizard/git/repo')
    expect(op.targetTypeId).toBe('git/repo')
    expect(op.targetKeyPrefix).toBe('git/repo/')
    expect(op.name).toBe('Repository')

    const settingsCall = applyWorldOp.mock.calls[1]
    if (!settingsCall) {
      throw new Error('expected settings op call')
    }
    expect(settingsCall[0]).toBe(SET_SPACE_SETTINGS_OP_ID)
    const settings = SetSpaceSettingsOp.fromBinary(settingsCall[1]).settings
    if (!settings) {
      throw new Error('expected settings')
    }
    expect(settings.indexPath).toBe(op.objectKey)
  })
})
