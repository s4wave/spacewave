import { beforeEach, describe, expect, it, vi } from 'vitest'

import { TypePred, buildTypeObjectKey } from '@s4wave/sdk/world/types/types.js'
import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { SPACE_SETTINGS_BLOCK_TYPE } from '@s4wave/core/space/world/world.js'
import { SpaceSettings } from '@s4wave/core/space/world/world.pb.js'
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
import { V86_DEFAULT_CDN_IMAGE_OBJECT_KEY } from '@s4wave/app/vm/v86-wizard-config.js'
import {
  CreateWizardObjectOp,
  IntroWizardConfig,
} from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { IntroWizardTypeID } from '@s4wave/app/wizard/intro.js'
import { UnixFSTypeID } from '@s4wave/sdk/unixfs/type.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import { CreateComputersDashboardOp } from '@s4wave/sdk/device/device.pb.js'
import { CREATE_COMPUTERS_DASHBOARD_OP_ID } from '@s4wave/sdk/device/computers/create-computers-dashboard.js'
import {
  AddDeviceDefaultName,
  AddDeviceWizardTargetKeyPrefix,
  AddDeviceWizardTypeID,
} from '@s4wave/app/device/add-device-wizard.js'
import { InitChatDemoOp } from '@s4wave/sdk/chat/chat.pb.js'
import {
  CHAT_DEMO_CHANNEL_KEY,
  INIT_CHAT_DEMO_OP_ID,
} from '@s4wave/sdk/chat/init-chat-demo.js'
import { InitForgeQuickstartOp } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import { INIT_FORGE_QUICKSTART_OP_ID } from '@s4wave/sdk/forge/dashboard/init-forge-quickstart.js'
import { Query } from '@s4wave/sdk/sql/query/query.pb.js'
import { CreateVmV86Op, SetV86StateOp, VmState } from '@s4wave/sdk/vm/v86.pb.js'
import { CREATE_VM_V86_OP_ID } from '@s4wave/sdk/vm/create-vm-v86.js'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { QuickstartSpaceCreateId } from './options.js'
import {
  buildQuickstartSpaceRoutePath,
  createLocalSession,
  createQuickstartSetupFromSession,
  createQuickstartSetup,
  createDrive,
  createSpaceSettingsObject,
  executeDynamicQuickstart,
  getQuickstartInitialObjectRouteHandoff,
  getQuickstartSpaceName,
  populateSpace,
  type QuickstartProgressState,
  type QuickstartSetupTiming,
} from './create.js'

const quickstartRegistryMocks = vi.hoisted(() => ({
  ListQuickstarts: vi.fn(),
  WatchQuickstarts: vi.fn(),
  ExecuteQuickstart: vi.fn(),
}))

const localProviderMocks = vi.hoisted(() => ({
  createAccount: vi.fn(),
}))

const spaceMocks = vi.hoisted(() => ({
  mountSpace: vi.fn(),
}))

const fsHandleMocks = vi.hoisted(() => ({
  mknod: vi.fn(),
  lookup: vi.fn(),
  writeAt: vi.fn(),
  uploadFile: vi.fn(),
  release: vi.fn(),
}))

const fsFileHandleMocks = vi.hoisted(() => ({
  writeAt: vi.fn(),
  release: vi.fn(),
}))

const uploadedFiles = vi.hoisted(
  (): Array<{
    name: string
    totalSize: bigint
    bytes: Uint8Array
    mode?: number
    abortSignal?: AbortSignal
  }> => [],
)

const kvStoreMocks = vi.hoisted(() => ({
  constructor: vi.fn(),
  withTransaction: vi.fn(),
  release: vi.fn(),
  tx: {
    set: vi.fn(),
  },
}))

const sqlDbMocks = vi.hoisted(() => ({
  constructor: vi.fn(),
  withTransaction: vi.fn(),
  release: vi.fn(),
  tx: {
    exec: vi.fn(),
  },
}))

const sqlQueryMocks = vi.hoisted(() => ({
  constructor: vi.fn(),
  setQueryText: vi.fn(),
  setParameters: vi.fn(),
  release: vi.fn(),
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

vi.mock('@s4wave/sdk/unixfs/index.js', () => ({
  MknodType: {
    FILE: 1,
  },
  FSHandle: vi.fn(function () {
    return fsHandleMocks
  }),
}))

vi.mock('@s4wave/sdk/kv/index.js', () => ({
  KvStoreTypeID: 'kv/store',
  KvStore: vi.fn(function () {
    kvStoreMocks.constructor()
    return kvStoreMocks
  }),
}))

vi.mock('@s4wave/sdk/sql/index.js', () => ({
  SqlDbTypeID: 'sql/db',
  SqlQueryBlockTypeID: 'github.com/s4wave/spacewave/sdk/sql/query.Query',
  SqlQueryTypeID: 'sql/query',
  SqlDatabase: vi.fn(function () {
    sqlDbMocks.constructor()
    return sqlDbMocks
  }),
  SqlQuery: vi.fn(function () {
    sqlQueryMocks.constructor()
    return sqlQueryMocks
  }),
}))

type ApplyWorldOp = (
  opTypeId: string,
  opData: Uint8Array,
  sender?: string,
  abortSignal?: AbortSignal,
) => Promise<{ seqno: bigint; sysErr: boolean }>

interface TypedAccessFixture {
  resourceId: number
  typeId: string
}

function buildQuickstartWorld(
  accessByKey: Record<string, TypedAccessFixture> = {},
) {
  const applyWorldOp = vi.fn<ApplyWorldOp>().mockResolvedValue({
    seqno: 1n,
    sysErr: false,
  })
  const releaseCursor = vi.fn()
  const releaseObject = vi.fn()
  const transactionWrite = vi.fn().mockResolvedValue({
    rootRef: { hash: { hashType: 1, hash: new Uint8Array([1]) } },
  })
  const blockCursorSetBlock = vi.fn().mockResolvedValue(undefined)
  const blockCursorMarkDirty = vi.fn().mockResolvedValue(undefined)
  const buildTransaction = vi.fn().mockResolvedValue({
    transaction: { write: transactionWrite },
    cursor: {
      markDirty: blockCursorMarkDirty,
      setBlock: blockCursorSetBlock,
    },
  })
  const createObject = vi.fn().mockResolvedValue({
    release: releaseObject,
    [Symbol.dispose]: releaseObject,
  })
  const setGraphQuad = vi.fn().mockResolvedValue(undefined)
  const accessTypedObject = vi.fn((objectKey: string) =>
    Promise.resolve(
      accessByKey[objectKey] ?? {
        resourceId: 71,
        typeId: 'unixfs/fs-node',
      },
    ),
  )
  return {
    world: {
      applyWorldOp,
      getObject: vi.fn().mockResolvedValue(null),
      lookupGraphQuads: vi.fn().mockResolvedValue({ quads: [] }),
      deleteGraphQuad: vi.fn().mockResolvedValue(undefined),
      setGraphQuad,
      buildStorageCursor: vi.fn(() =>
        Promise.resolve({
          buildTransaction,
          putBlock: vi.fn().mockResolvedValue({ ref: {} }),
          getRef: vi.fn().mockResolvedValue({ ref: { bucketId: 'world' } }),
          release: releaseCursor,
          [Symbol.dispose]: releaseCursor,
        }),
      ),
      createObject,
      accessTypedObject,
      getResourceRef: vi.fn(() => ({
        createRef: vi.fn((resourceId: number) => ({
          resourceId,
          client: {},
        })),
      })),
    },
    applyWorldOp,
    accessTypedObject,
    blockCursorSetBlock,
    createObject,
    setGraphQuad,
  }
}

function getSettingsIndexPath(applyWorldOp: ReturnType<typeof vi.fn>) {
  return getLastSettings(applyWorldOp).indexPath ?? ''
}

function getLastSettings(applyWorldOp: ReturnType<typeof vi.fn>) {
  const call = applyWorldOp.mock.calls
    .filter((call) => call[0] === SET_SPACE_SETTINGS_OP_ID)
    .at(-1)
  if (!call) {
    throw new Error('expected settings op call')
  }
  const settings = SetSpaceSettingsOp.fromBinary(call[1] as Uint8Array).settings
  if (!settings) {
    throw new Error('expected settings')
  }
  return settings
}

function getSettingsCalls(applyWorldOp: ReturnType<typeof vi.fn>) {
  const calls: SetSpaceSettingsOp[] = []
  for (const call of applyWorldOp.mock.calls) {
    if (call[0] === SET_SPACE_SETTINGS_OP_ID) {
      calls.push(SetSpaceSettingsOp.fromBinary(call[1] as Uint8Array))
    }
  }
  return calls
}

function notesQuickstartSetup(
  world: unknown,
  spaceResourceId: number,
): {
  root: { client: Record<string, never> }
  space: { id: number }
  spaceWorld: unknown
} {
  return {
    root: { client: {} },
    space: { id: spaceResourceId },
    spaceWorld: world,
  }
}

function registeredNotesQuickstart(id: string) {
  return { quickstartId: id, pluginId: 'spacewave-notes' }
}

function watchQuickstartRegistrations(
  ...registrations: Array<Array<{ quickstartId: string; pluginId: string }>>
) {
  const cursor = { index: 0 }
  return {
    [Symbol.asyncIterator]() {
      return {
        next() {
          if (cursor.index >= registrations.length) {
            return Promise.resolve({
              done: true,
              value: undefined,
            } as IteratorResult<{
              registrations: Array<{ quickstartId: string; pluginId: string }>
            }>)
          }
          const batch = registrations[cursor.index]
          cursor.index += 1
          return Promise.resolve({
            done: false,
            value: { registrations: batch },
          } as IteratorResult<{
            registrations: Array<{ quickstartId: string; pluginId: string }>
          }>)
        },
      }
    },
  }
}

function mockNotesQuickstart(quickstartId: string, indexPath: string): void {
  quickstartRegistryMocks.ListQuickstarts.mockResolvedValue({
    registrations: [registeredNotesQuickstart(quickstartId)],
  })
  quickstartRegistryMocks.ExecuteQuickstart.mockResolvedValue({
    indexPath,
    pluginIds: ['spacewave-notes'],
  })
}

describe('quickstart create', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
    localProviderMocks.createAccount.mockReset()
    spaceMocks.mountSpace.mockReset()
    fsHandleMocks.mknod.mockReset()
    fsHandleMocks.lookup.mockReset()
    fsHandleMocks.writeAt.mockReset()
    fsHandleMocks.uploadFile.mockReset()
    fsHandleMocks.release.mockReset()
    fsFileHandleMocks.writeAt.mockReset()
    fsFileHandleMocks.release.mockReset()
    uploadedFiles.length = 0
    fsHandleMocks.mknod.mockResolvedValue(undefined)
    fsHandleMocks.lookup.mockResolvedValue(fsFileHandleMocks)
    fsFileHandleMocks.writeAt.mockResolvedValue(0n)
    fsHandleMocks.uploadFile.mockImplementation(
      async (
        name: string,
        totalSize: bigint,
        stream: ReadableStream<Uint8Array>,
        mode?: number,
        _onProgress?: (bytesWritten: bigint) => void,
        abortSignal?: AbortSignal,
      ) => {
        const bytes = new Uint8Array(await new Response(stream).arrayBuffer())
        uploadedFiles.push({ name, totalSize, bytes, mode, abortSignal })
        return BigInt(bytes.byteLength)
      },
    )
    kvStoreMocks.constructor.mockReset()
    kvStoreMocks.withTransaction.mockReset()
    kvStoreMocks.release.mockReset()
    kvStoreMocks.tx.set.mockReset()
    kvStoreMocks.tx.set.mockResolvedValue(undefined)
    kvStoreMocks.withTransaction.mockImplementation(
      async (_write: boolean, fn: (tx: typeof kvStoreMocks.tx) => unknown) =>
        await fn(kvStoreMocks.tx),
    )
    sqlDbMocks.constructor.mockReset()
    sqlDbMocks.withTransaction.mockReset()
    sqlDbMocks.release.mockReset()
    sqlDbMocks.tx.exec.mockReset()
    sqlDbMocks.tx.exec.mockResolvedValue({})
    sqlDbMocks.withTransaction.mockImplementation(
      async (
        _write: boolean,
        _dsn: string,
        fn: (tx: typeof sqlDbMocks.tx) => unknown,
      ) => await fn(sqlDbMocks.tx),
    )
    sqlQueryMocks.constructor.mockReset()
    sqlQueryMocks.setQueryText.mockReset()
    sqlQueryMocks.setQueryText.mockResolvedValue(undefined)
    sqlQueryMocks.setParameters.mockReset()
    sqlQueryMocks.setParameters.mockResolvedValue(undefined)
    sqlQueryMocks.release.mockReset()
    quickstartRegistryMocks.ListQuickstarts.mockReset()
    quickstartRegistryMocks.WatchQuickstarts.mockReset()
    quickstartRegistryMocks.ExecuteQuickstart.mockReset()
    quickstartRegistryMocks.ListQuickstarts.mockResolvedValue({
      registrations: [],
    })
    quickstartRegistryMocks.WatchQuickstarts.mockReturnValue(
      watchQuickstartRegistrations([]),
    )
    quickstartRegistryMocks.ExecuteQuickstart.mockResolvedValue({
      indexPath: '',
      pluginIds: [],
    })
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
    const timeoutSpy = vi.spyOn(AbortSignal, 'timeout')

    await createLocalSession(
      root as never,
      new AbortController().signal,
      cleanup,
    )

    expect(timeoutSpy).toHaveBeenNthCalledWith(1, 120000)
    timeoutSpy.mockRestore()
    expect(root.listSessions).not.toHaveBeenCalled()
    expect(root.lookupProvider).toHaveBeenCalledWith(
      'local',
      expect.any(AbortSignal),
    )
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
      indexPath: 'glados/operator-home',
      pluginIds: ['glados-core', 'glados-web'],
    })
    const { world, applyWorldOp } = buildQuickstartWorld()
    await executeDynamicQuickstart(
      { client: {} } as never,
      'glados-workspace',
      {
        space: { id: 42 },
        spaceWorld: world,
        spaceContents: {},
      } as never,
    )

    expect(quickstartRegistryMocks.ExecuteQuickstart).toHaveBeenCalledWith(
      { quickstartId: 'glados-workspace', spaceResourceId: 42 },
      undefined,
    )
    expect(getSettingsIndexPath(applyWorldOp)).toBe('glados/operator-home')
    expect(getSettingsIndexPath(applyWorldOp)).not.toBe('glados/org-chart')
    const settingsCall = applyWorldOp.mock.calls.find(
      (call) => call[0] === SET_SPACE_SETTINGS_OP_ID,
    )
    const settings = SetSpaceSettingsOp.fromBinary(settingsCall?.[1]).settings
    expect(settings?.pluginIds).toEqual(['glados-core', 'glados-web'])
  })

  it('waits for the Notes plugin quickstart before executing public Notes launchers', async () => {
    quickstartRegistryMocks.ListQuickstarts.mockResolvedValue({
      registrations: [],
    })
    quickstartRegistryMocks.WatchQuickstarts.mockReturnValue(
      watchQuickstartRegistrations([], [registeredNotesQuickstart('notebook')]),
    )
    quickstartRegistryMocks.ExecuteQuickstart.mockResolvedValue({
      indexPath: 'notebook',
      pluginIds: ['spacewave-notes'],
    })
    const { world, applyWorldOp } = buildQuickstartWorld()

    await populateSpace('notebook', notesQuickstartSetup(world, 55) as never)

    expect(quickstartRegistryMocks.WatchQuickstarts).toHaveBeenCalledWith(
      {},
      expect.any(AbortSignal),
    )
    expect(quickstartRegistryMocks.ExecuteQuickstart).toHaveBeenCalledWith(
      { quickstartId: 'notebook', spaceResourceId: 55 },
      undefined,
    )
    expect(getSettingsIndexPath(applyWorldOp)).toBe('notebook')
  })

  it('surfaces a controlled timeout when Notes registration never arrives', async () => {
    const timeoutController = new AbortController()
    const timeoutSpy = vi
      .spyOn(AbortSignal, 'timeout')
      .mockReturnValue(timeoutController.signal)
    const watchAbort = new Error('watch aborted')
    quickstartRegistryMocks.ListQuickstarts.mockResolvedValue({
      registrations: [],
    })
    quickstartRegistryMocks.WatchQuickstarts.mockReturnValue({
      [Symbol.asyncIterator]() {
        return {
          next() {
            timeoutController.abort(watchAbort)
            return Promise.reject(watchAbort)
          },
        }
      },
    })
    const { world, applyWorldOp } = buildQuickstartWorld()

    try {
      await expect(
        populateSpace('notebook', notesQuickstartSetup(world, 56) as never),
      ).rejects.toThrow(
        'Timed out waiting for quickstart registration: notebook',
      )

      expect(timeoutSpy).toHaveBeenCalledWith(120000)
      expect(quickstartRegistryMocks.WatchQuickstarts).toHaveBeenCalledWith(
        {},
        timeoutController.signal,
      )
      expect(quickstartRegistryMocks.ExecuteQuickstart).not.toHaveBeenCalled()
      expect(getLastSettings(applyWorldOp).pluginIds).toEqual([
        'spacewave-notes',
      ])
    } finally {
      timeoutSpy.mockRestore()
    }
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
      ['device', 'My Computers'],
      ['forge', 'My Forge Dashboard'],
      ['kv', 'My Key-Value Store'],
      ['sql', 'My SQL Database'],
    ]

    for (const [quickstartId, name] of cases) {
      expect(getQuickstartSpaceName(quickstartId)).toBe(name)
    }
  })

  it('routes Drive quickstart through the Space default route', () => {
    expect(buildQuickstartSpaceRoutePath('/u/2/so/space-1', 'drive')).toBe(
      '/u/2/so/space-1',
    )
    expect(getQuickstartInitialObjectRouteHandoff('drive')).toBeUndefined()
    expect(getQuickstartInitialObjectRouteHandoff('notebook')).toBeUndefined()
    expect(getQuickstartInitialObjectRouteHandoff('docs')).toBeUndefined()
    expect(getQuickstartInitialObjectRouteHandoff('blog')).toBeUndefined()
    expect(getQuickstartInitialObjectRouteHandoff('git')).toBeUndefined()
    expect(buildQuickstartSpaceRoutePath('/u/2/so/space-1/', 'canvas')).toBe(
      '/u/2/so/space-1/-/canvas-1',
    )
    expect(getQuickstartInitialObjectRouteHandoff('canvas')).toEqual({
      objectKey: 'canvas-1',
      objectType: 'canvas',
    })
    expect(buildQuickstartSpaceRoutePath('/u/2/so/space-1', 'kv')).toBe(
      '/u/2/so/space-1/-/kv/store',
    )
    expect(getQuickstartInitialObjectRouteHandoff('kv')).toEqual({
      objectKey: 'kv/store',
      objectType: 'kv/store',
    })
    expect(buildQuickstartSpaceRoutePath('/u/2/so/space-1', 'sql')).toBe(
      '/u/2/so/space-1/-/sql/db',
    )
    expect(getQuickstartInitialObjectRouteHandoff('sql')).toEqual({
      objectKey: 'sql/db',
      objectType: 'sql/db',
    })
    expect(getQuickstartInitialObjectRouteHandoff('forge')).toEqual({
      objectKey: 'forge',
      objectType: 'alpha/object-layout',
    })
    expect(buildQuickstartSpaceRoutePath('/u/2/so/space-1', 'space')).toBe(
      '/u/2/so/space-1',
    )
  })

  it('does not hold a world-state cursor while Drive content is seeded', async () => {
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
    const spaceContents = {
      release: vi.fn(),
      [Symbol.dispose]: vi.fn(),
    }
    const accessWorldState = vi.fn().mockResolvedValue(world)
    spaceMocks.mountSpace.mockResolvedValue({
      accessWorldState,
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
    expect(populatePhase?.finishedMs).toEqual(expect.any(Number))
    expect(timing?.progressReadyMs ?? 0).toBeGreaterThanOrEqual(
      populatePhase?.finishedMs ?? 0,
    )
    expect(applyWorldOp.mock.calls[0]?.[0]).toBe(INIT_UNIXFS_OP_ID)
    expect(accessWorldState).toHaveBeenCalledTimes(1)
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

  it('records a seed error without marking progress-ready or content-ready', async () => {
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
    expect(timing?.progressReadyMs).toBeUndefined()
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

  it('creates Drive storage and indexes the intro wizard before raw files', async () => {
    const createRef = vi.fn((resourceId: number) => ({
      resourceId,
      client: {},
    }))
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
      accessTypedObject: vi.fn().mockResolvedValue({
        resourceId: 71,
        typeId: 'unixfs/fs-node',
      }),
      getResourceRef: vi.fn(() => ({ createRef })),
      applyWorldOp,
    }

    await createDrive(spaceWorld as never)

    expect(applyWorldOp).toHaveBeenCalledTimes(3)
    expect(applyWorldOp.mock.calls[0]?.[0]).toBe(INIT_UNIXFS_OP_ID)

    const wizardCall = applyWorldOp.mock.calls[1]
    if (!wizardCall) {
      throw new Error('expected intro wizard op call')
    }
    expect(wizardCall[0]).toBe(CREATE_WIZARD_OBJECT_OP_ID)
    const wizardOp = CreateWizardObjectOp.fromBinary(wizardCall[1])
    expect(wizardOp.wizardTypeId).toBe(IntroWizardTypeID)
    expect(wizardOp.targetTypeId).toBe(UnixFSTypeID)
    expect(wizardOp.targetKeyPrefix).toBe(UNIXFS_OBJECT_KEY)
    const introConfig = IntroWizardConfig.fromBinary(wizardOp.initialConfigData)
    expect(introConfig.headline).toBe('Welcome to your Drive')
    expect(introConfig.callouts?.length).toBe(3)

    const settingsCall = applyWorldOp.mock.calls[2]
    if (!settingsCall) {
      throw new Error('expected settings op call')
    }
    expect(settingsCall[0]).toBe(SET_SPACE_SETTINGS_OP_ID)
    const settings = SetSpaceSettingsOp.fromBinary(settingsCall[1]).settings
    if (!settings) {
      throw new Error('expected settings')
    }
    expect(settings.indexPath).toBe(wizardOp.objectKey)
    expect(spaceWorld.accessTypedObject).toHaveBeenCalledWith(
      UNIXFS_OBJECT_KEY,
      undefined,
    )
    expect(createRef).toHaveBeenCalledWith(71)
    const starterGuide = uploadedFiles[0]
    if (!starterGuide) {
      throw new Error('expected starter guide upload')
    }
    expect(fsHandleMocks.uploadFile).toHaveBeenCalledWith(
      'getting-started.md',
      BigInt(starterGuide.bytes.byteLength),
      expect.any(ReadableStream),
      0o644,
      undefined,
      undefined,
    )
    expect(fsHandleMocks.mknod).not.toHaveBeenCalled()
    expect(fsHandleMocks.lookup).not.toHaveBeenCalled()
    expect(fsFileHandleMocks.writeAt).not.toHaveBeenCalled()
    const expectedStarterGuide = `# Getting Started

Welcome to your new drive! This starter guide is written by the Drive
Quickstart after the generic UnixFS filesystem is initialized.

## Next steps

Try uploading a few files and opening them here. Video files are the best ones
to try first.
`
    expect(starterGuide.name).toBe('getting-started.md')
    expect(starterGuide.totalSize).toBe(
      BigInt(new TextEncoder().encode(expectedStarterGuide).byteLength),
    )
    expect(starterGuide.mode).toBe(0o644)
    expect(starterGuide.abortSignal).toBeUndefined()
    expect(new TextDecoder().decode(starterGuide.bytes)).toBe(
      expectedStarterGuide,
    )
    expect(fsFileHandleMocks.release).not.toHaveBeenCalled()
    expect(fsHandleMocks.release).toHaveBeenCalled()
  })

  it('reuses the CreateSpace world resource instead of remounting it', async () => {
    const abortSignal = new AbortController().signal
    const cleanup: RegisterCleanup = (value) => value
    const engine = {
      release: vi.fn(),
      [Symbol.dispose]: vi.fn(),
    }
    const createResource = vi.fn(() => engine)
    const accessWorldState = vi.fn()
    const spaceContents = {
      release: vi.fn(),
      [Symbol.dispose]: vi.fn(),
    }
    spaceMocks.mountSpace.mockResolvedValue({
      accessWorldState,
      mountSpaceContents: vi.fn().mockResolvedValue(spaceContents),
    })

    const setup = await createQuickstartSetupFromSession({
      session: {
        resourceRef: {
          createResource,
        },
      } as never,
      spaceResp: {
        spaceWorldResourceId: 99,
      },
      abortSignal,
      cleanup,
    })

    expect(createResource).toHaveBeenCalledWith(99, expect.any(Function))
    expect(accessWorldState).not.toHaveBeenCalled()
    expect(setup.spaceWorld.getEngine()).toBe(engine)
  })

  it('records Drive UnixFS transaction subphases when timing is available', async () => {
    const txApplyWorldOp = vi.fn<ApplyWorldOp>().mockResolvedValue({
      seqno: 1n,
      sysErr: false,
    })
    const commit = vi.fn().mockResolvedValue(undefined)
    const discard = vi.fn().mockResolvedValue(undefined)
    const newTransaction = vi.fn().mockResolvedValue({
      applyWorldOp: txApplyWorldOp,
      commit,
      discard,
    })
    const applyWorldOp = vi.fn<ApplyWorldOp>().mockResolvedValue({
      seqno: 2n,
      sysErr: false,
    })
    const spaceWorld = {
      getEngine: vi.fn(() => ({ newTransaction })),
      getObject: vi.fn(() => Promise.resolve(null)),
      lookupGraphQuads: vi.fn().mockResolvedValue({ quads: [] }),
      setGraphQuad: vi.fn().mockResolvedValue(undefined),
      accessTypedObject: vi.fn().mockResolvedValue({
        resourceId: 71,
        typeId: 'unixfs/fs-node',
      }),
      getResourceRef: vi.fn(() => ({
        createRef: vi.fn((resourceId: number) => ({
          resourceId,
          client: {},
        })),
      })),
      applyWorldOp,
    }
    const timing: QuickstartSetupTiming = {
      quickstartId: 'drive',
      state: 'loading',
      startedMs: 0,
      phases: [],
    }

    await createDrive(spaceWorld as never, undefined, timing)

    expect(newTransaction).toHaveBeenCalledTimes(3)
    expect(newTransaction).toHaveBeenCalledWith(true, undefined)
    expect(txApplyWorldOp).toHaveBeenNthCalledWith(
      1,
      INIT_UNIXFS_OP_ID,
      expect.any(Uint8Array),
      '',
      undefined,
    )
    expect(txApplyWorldOp).toHaveBeenNthCalledWith(
      2,
      CREATE_WIZARD_OBJECT_OP_ID,
      expect.any(Uint8Array),
      '',
      undefined,
    )
    expect(txApplyWorldOp).toHaveBeenNthCalledWith(
      3,
      SET_SPACE_SETTINGS_OP_ID,
      expect.any(Uint8Array),
      '',
      undefined,
    )
    expect(commit).toHaveBeenCalledTimes(3)
    expect(discard).toHaveBeenCalledTimes(3)
    expect(applyWorldOp).not.toHaveBeenCalled()
    expect(timing.phases.map((phase) => phase.name)).toEqual([
      'init-drive-unixfs',
      'init-drive-unixfs-new-transaction',
      'init-drive-unixfs-apply-op',
      'init-drive-unixfs-commit',
      'init-drive-unixfs-discard',
      'write-drive-starter-guide-access',
      'write-drive-starter-guide-upload',
      'create-drive-intro-wizard-new-transaction',
      'create-drive-intro-wizard-apply-op',
      'create-drive-intro-wizard-commit',
      'create-drive-intro-wizard-discard',
      'create-drive-settings',
      'create-drive-settings-get-object',
      'create-drive-settings-new-transaction',
      'create-drive-settings-apply-op',
      'create-drive-settings-commit',
      'create-drive-settings-discard',
    ])
  })

  it('seeds the KV quickstart with examples and indexes the store', async () => {
    const { world, applyWorldOp, createObject, setGraphQuad } =
      buildQuickstartWorld({
        'kv/store': { resourceId: 201, typeId: 'kv/store' },
      })

    await populateSpace('kv', { spaceWorld: world } as never)

    expect(createObject).toHaveBeenCalledWith('kv/store', {}, undefined)
    expect(createObject).toHaveBeenCalledWith(
      buildTypeObjectKey('kv/store'),
      {},
      undefined,
    )
    expect(setGraphQuad).toHaveBeenCalledWith(
      keyToIRI('kv/store'),
      TypePred,
      keyToIRI(buildTypeObjectKey('kv/store')),
      undefined,
      undefined,
    )
    expect(kvStoreMocks.constructor).toHaveBeenCalledTimes(1)
    expect(kvStoreMocks.withTransaction).toHaveBeenCalledTimes(1)
    expect(kvStoreMocks.withTransaction.mock.calls[0]?.[0]).toBe(true)
    expect(kvStoreMocks.tx.set).toHaveBeenCalledTimes(3)

    const decoder = new TextDecoder()
    const entries = kvStoreMocks.tx.set.mock.calls.map((call) => ({
      key: decoder.decode(call[0]),
      value: call[1],
    }))
    expect(
      entries.map((entry) => [
        entry.key,
        entry.key === 'binary/blob'
          ? Array.from(entry.value)
          : decoder.decode(entry.value),
      ]),
    ).toEqual([
      ['hello', 'world'],
      [
        'profile.json',
        '{"name":"Ada Lovelace","role":"analyst","active":true}',
      ],
      ['binary/blob', [0, 1, 2, 3, 5, 8, 13]],
    ])
    expect(kvStoreMocks.release).toHaveBeenCalledTimes(1)
    expect(getSettingsIndexPath(applyWorldOp)).toBe('kv/store')
  })

  it('seeds the SQL quickstart with schema, rows, a linked query, and index', async () => {
    const {
      world,
      applyWorldOp,
      blockCursorSetBlock,
      createObject,
      setGraphQuad,
    } = buildQuickstartWorld({
      'sql/db': { resourceId: 301, typeId: 'sql/db' },
      'sql/query/example': { resourceId: 302, typeId: 'sql/query' },
    })

    await populateSpace('sql', { spaceWorld: world } as never)

    expect(createObject).toHaveBeenCalledWith('sql/db', {}, undefined)
    expect(createObject).toHaveBeenCalledWith(
      buildTypeObjectKey('sql/db'),
      {},
      undefined,
    )
    expect(createObject).toHaveBeenCalledWith(
      'sql/query/example',
      {
        bucketId: 'world',
        rootRef: { hash: { hashType: 1, hash: new Uint8Array([1]) } },
      },
      undefined,
    )
    expect(createObject).toHaveBeenCalledWith(
      buildTypeObjectKey('sql/query'),
      {},
      undefined,
    )
    expect(setGraphQuad).toHaveBeenCalledWith(
      keyToIRI('sql/db'),
      TypePred,
      keyToIRI(buildTypeObjectKey('sql/db')),
      undefined,
      undefined,
    )
    expect(setGraphQuad).toHaveBeenCalledWith(
      keyToIRI('sql/query/example'),
      TypePred,
      keyToIRI(buildTypeObjectKey('sql/query')),
      undefined,
      undefined,
    )
    expect(sqlDbMocks.constructor).toHaveBeenCalledTimes(1)
    expect(
      sqlDbMocks.withTransaction.mock.calls.map((call) => call[1]),
    ).toEqual(['', '/quickstart'])
    expect(sqlDbMocks.tx.exec.mock.calls.map((call) => call[0])).toEqual([
      'CREATE DATABASE quickstart',
      'CREATE TABLE people (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL, role TEXT NOT NULL)',
      "INSERT INTO people (id, name, role) VALUES (1, 'ada', 'analyst')",
      "INSERT INTO people (id, name, role) VALUES (2, 'grace', 'engineer')",
      'CREATE TABLE projects (id BIGINT NOT NULL PRIMARY KEY, owner_id BIGINT NOT NULL, title TEXT NOT NULL)',
      "INSERT INTO projects (id, owner_id, title) VALUES (10, 1, 'difference engine notes')",
      "INSERT INTO projects (id, owner_id, title) VALUES (11, 2, 'compiler logbook')",
    ])
    expect(sqlDbMocks.release).toHaveBeenCalledTimes(1)

    const queryBlockReq = blockCursorSetBlock.mock.calls[0]?.[0]
    expect(Query.fromBinary(queryBlockReq.data ?? new Uint8Array())).toEqual({})
    expect(queryBlockReq.markDirty).toBe(true)
    expect(sqlQueryMocks.constructor).toHaveBeenCalledTimes(1)
    expect(sqlQueryMocks.setQueryText).toHaveBeenCalledWith(
      'SELECT name, role FROM quickstart.people WHERE id = ?',
      'mysql',
      'sql/db',
      undefined,
    )
    expect(sqlQueryMocks.setParameters).toHaveBeenCalledWith(
      [{ value: { case: 'intValue', value: 1n } }],
      undefined,
    )
    expect(sqlQueryMocks.release).toHaveBeenCalledTimes(1)
    expect(getSettingsIndexPath(applyWorldOp)).toBe('sql/db')
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
      const wizardCall = applyWorldOp.mock.calls.find(
        (call) => call[0] === CREATE_WIZARD_OBJECT_OP_ID,
      )
      const wizardKey = CreateWizardObjectOp.fromBinary(
        wizardCall?.[1] as Uint8Array,
      ).objectKey
      expect(getSettingsIndexPath(applyWorldOp)).toBe(wizardKey)
      const unixfsCall = applyWorldOp.mock.calls.find(
        (call) => call[0] === INIT_UNIXFS_OP_ID,
      )
      expect(InitUnixFSOp.fromBinary(unixfsCall?.[1]).objectKey).toBe(
        UNIXFS_OBJECT_KEY,
      )
      const settingsIndex = applyWorldOp.mock.calls.findIndex(
        (call) => call[0] === SET_SPACE_SETTINGS_OP_ID,
      )
      const unixfsIndex = applyWorldOp.mock.calls.findIndex(
        (call) => call[0] === INIT_UNIXFS_OP_ID,
      )
      const wizardIndex = applyWorldOp.mock.calls.findIndex(
        (call) => call[0] === CREATE_WIZARD_OBJECT_OP_ID,
      )
      expect(unixfsIndex).toBeGreaterThanOrEqual(0)
      expect(wizardIndex).toBeGreaterThan(unixfsIndex)
      expect(settingsIndex).toBeGreaterThan(wizardIndex)
    }
    {
      const { world, applyWorldOp } = buildQuickstartWorld()
      mockNotesQuickstart('notebook', 'notebook')
      await populateSpace('notebook', notesQuickstartSetup(world, 101) as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe('notebook')
      expect(getLastSettings(applyWorldOp).pluginIds).toEqual([
        'spacewave-notes',
      ])
      expect(quickstartRegistryMocks.ListQuickstarts).toHaveBeenCalledWith(
        {},
        undefined,
      )
      expect(quickstartRegistryMocks.ExecuteQuickstart).toHaveBeenCalledWith(
        { quickstartId: 'notebook', spaceResourceId: 101 },
        undefined,
      )
      const settings = getSettingsCalls(applyWorldOp).map((op) => op.settings)
      expect(settings[0]?.pluginIds).toEqual(['spacewave-notes'])
      expect(settings[1]?.indexPath).toBe('notebook')
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
      const { world, applyWorldOp } = buildQuickstartWorld({
        'kv/store': { resourceId: 201, typeId: 'kv/store' },
      })
      await populateSpace('kv', { spaceWorld: world } as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe('kv/store')
    }
    {
      const { world, applyWorldOp } = buildQuickstartWorld({
        'sql/db': { resourceId: 301, typeId: 'sql/db' },
        'sql/query/example': { resourceId: 302, typeId: 'sql/query' },
      })
      await populateSpace('sql', { spaceWorld: world } as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe('sql/db')
    }
    {
      const { world, applyWorldOp } = buildQuickstartWorld()
      mockNotesQuickstart('docs', 'documentation')
      await populateSpace('docs', notesQuickstartSetup(world, 102) as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe('documentation')
      expect(getLastSettings(applyWorldOp).pluginIds).toEqual([
        'spacewave-notes',
      ])
      expect(quickstartRegistryMocks.ExecuteQuickstart).toHaveBeenCalledWith(
        { quickstartId: 'docs', spaceResourceId: 102 },
        undefined,
      )
    }
    {
      const { world, applyWorldOp } = buildQuickstartWorld()
      mockNotesQuickstart('blog', 'blog/site')
      await populateSpace('blog', notesQuickstartSetup(world, 103) as never)
      expect(getSettingsIndexPath(applyWorldOp)).toBe('blog/site')
      expect(getLastSettings(applyWorldOp).pluginIds).toEqual([
        'spacewave-notes',
      ])
      expect(quickstartRegistryMocks.ExecuteQuickstart).toHaveBeenCalledWith(
        { quickstartId: 'blog', spaceResourceId: 103 },
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
    const unmarshal = vi.fn(() =>
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
            unmarshal,
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
    expect(unmarshal).toHaveBeenCalledWith(
      { blockType: SPACE_SETTINGS_BLOCK_TYPE },
      undefined,
    )
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

  it('does not re-persist invalid plugin ids from existing settings', async () => {
    const invalidPluginId = '\b\x02\x1aBbinary-plugin-id'
    const unmarshal = vi.fn(() =>
      Promise.resolve({
        found: true,
        data: SpaceSettings.toBinary({
          indexPath: 'old-index',
          pluginIds: [invalidPluginId, 'spacewave-app'],
        }),
      }),
    )
    const release = vi.fn()
    const getObject = vi.fn(() =>
      Promise.resolve({
        accessWorldState: vi.fn().mockResolvedValue({
          unmarshal,
          release,
          [Symbol.dispose]: release,
        }),
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
    }

    await createSpaceSettingsObject(spaceWorld as never, undefined, undefined, [
      'spacewave-notes',
    ])

    const settingsCall = spaceWorld.applyWorldOp.mock.calls[0]
    if (!settingsCall) {
      throw new Error('expected settings op call')
    }
    const settings = SetSpaceSettingsOp.fromBinary(settingsCall[1]).settings
    expect(settings?.indexPath).toBe('old-index')
    expect(settings?.pluginIds).toEqual(['spacewave-app', 'spacewave-notes'])
  })

  it('rejects invalid plugin ids returned by dynamic quickstarts', async () => {
    const { world, applyWorldOp } = buildQuickstartWorld()

    await expect(
      createSpaceSettingsObject(world as never, undefined, 'blog', [
        'spacewave-notes',
        'not/a-plugin',
      ]),
    ).rejects.toThrow('quickstart returned invalid plugin id')

    expect(applyWorldOp).not.toHaveBeenCalled()
  })

  it('creates and starts the v86 quickstart from the default CDN image', async () => {
    const copyV86ImageToSpace = vi.fn().mockResolvedValue(undefined)
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
    const setProcessBinding = vi.fn().mockResolvedValue(undefined)
    const cdnDispose = vi.fn()
    const root = {
      getCdn: vi.fn().mockResolvedValue({
        cdn: {
          copyV86ImageToSpace,
          [Symbol.dispose]: cdnDispose,
        },
      }),
    }

    await populateSpace(
      'v86',
      {
        root,
        sessionIndex: 7,
        spaceResp: {
          sharedObjectRef: { providerResourceRef: { id: 'space-1' } },
        },
        spaceWorld,
        spaceContents: { setProcessBinding },
      } as never,
      undefined,
    )

    expect(root.getCdn).toHaveBeenCalledWith('', undefined)
    expect(copyV86ImageToSpace).toHaveBeenCalledWith(
      7,
      'space-1',
      V86_DEFAULT_CDN_IMAGE_OBJECT_KEY,
      'vm-image/default',
      undefined,
    )
    expect(cdnDispose).toHaveBeenCalledTimes(1)
    expect(applyWorldOp).toHaveBeenCalledTimes(3)
    const call = applyWorldOp.mock.calls[0]
    if (!call) {
      throw new Error('expected applyWorldOp call')
    }
    const opTypeId = call[0]
    const opData = call[1]
    expect(opTypeId).toBe(CREATE_VM_V86_OP_ID)
    const op = CreateVmV86Op.fromBinary(opData)
    expect(op.objectKey).toMatch(/^v86-vm-[a-z0-9]+-\d+$/)
    expect(op.name).toBe('V86 VM')
    expect(op.imageObjectKey).toBe('vm-image/default')
    expect(op.config?.memoryMb).toBe(256)
    expect(op.config?.vgaMemoryMb).toBe(8)
    expect(op.config?.networking ?? false).toBe(false)
    expect(op.config?.serialEnabled).toBe(true)

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
    expect(settings.pluginIds).toEqual(['spacewave-v86'])
    expect(setProcessBinding).toHaveBeenCalledWith(
      op.objectKey,
      'vm/v86',
      true,
      undefined,
    )

    const startCall = applyWorldOp.mock.calls[2]
    if (!startCall) {
      throw new Error('expected start op call')
    }
    expect(startCall[0]).toBe('vm/v86/set-state')
    const startOp = SetV86StateOp.fromBinary(startCall[1])
    expect(startOp.objectKey).toBe(op.objectKey)
    expect(startOp.state).toBe(VmState.VmState_STARTING)
  })

  it('seeds the Device quickstart with Computers and the Add Device wizard', async () => {
    const { world, applyWorldOp } = buildQuickstartWorld()

    await populateSpace(
      'device',
      {
        spaceWorld: world,
      } as never,
      undefined,
    )

    expect(applyWorldOp).toHaveBeenCalledTimes(3)
    const dashboardCall = applyWorldOp.mock.calls[0]
    if (!dashboardCall) {
      throw new Error('expected dashboard op call')
    }
    expect(dashboardCall[0]).toBe(CREATE_COMPUTERS_DASHBOARD_OP_ID)
    const dashboardOp = CreateComputersDashboardOp.fromBinary(dashboardCall[1])
    expect(dashboardOp.objectKey).toBe('computers')
    expect(dashboardOp.name).toBe('Computers')

    const wizardCall = applyWorldOp.mock.calls[1]
    if (!wizardCall) {
      throw new Error('expected wizard op call')
    }
    expect(wizardCall[0]).toBe(CREATE_WIZARD_OBJECT_OP_ID)
    const wizardOp = CreateWizardObjectOp.fromBinary(wizardCall[1])
    expect(wizardOp.objectKey).toMatch(/^wizard\/add-device-[a-z0-9]+-\d+$/)
    expect(wizardOp.wizardTypeId).toBe(AddDeviceWizardTypeID)
    expect(wizardOp.targetTypeId).toBe(DeviceTypeID)
    expect(wizardOp.targetKeyPrefix).toBe(AddDeviceWizardTargetKeyPrefix)
    expect(wizardOp.name).toBe(AddDeviceDefaultName)

    const settingsCall = applyWorldOp.mock.calls[2]
    if (!settingsCall) {
      throw new Error('expected settings op call')
    }
    expect(settingsCall[0]).toBe(SET_SPACE_SETTINGS_OP_ID)
    const settings = SetSpaceSettingsOp.fromBinary(settingsCall[1]).settings
    if (!settings) {
      throw new Error('expected settings')
    }
    expect(settings.indexPath).toBe('computers')
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
