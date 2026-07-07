import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, waitFor } from '@testing-library/react'

import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { InitObjectLayoutOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { INIT_OBJECT_LAYOUT_OP_ID } from '@s4wave/core/space/world/ops/init-object-layout.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import type { SubItemsCallback } from '@s4wave/web/command/CommandContext.js'
import {
  SharedObjectContext,
  SpaceContext,
} from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import {
  EXPERIMENTAL_CREATORS_STORAGE_KEY,
  setExperimentalCreatorsEnabled,
} from '../creator-visibility.js'
import {
  AddDeviceDefaultName,
  AddDeviceWizardTargetKeyPrefix,
  AddDeviceWizardTypeID,
} from '../device/add-device-wizard.js'
import { SpaceCommands } from './SpaceCommands.js'

interface RegisteredCommand {
  commandId?: string
  enabled?: boolean
  subItems?: SubItemsCallback
  handler?: (args: Record<string, string>) => void
}

type ApplyWorldOpResult = { seqno: bigint; sysErr: boolean }

const registeredCommands: RegisteredCommand[] = []

const h = vi.hoisted(() => ({
  applyWorldOp: vi.fn(
    (
      _opTypeId: string,
      _opData: Uint8Array,
      _sender?: string,
    ): Promise<ApplyWorldOpResult> =>
      Promise.resolve({ seqno: 1n, sysErr: false }),
  ),
  navigateToObjects: vi.fn((_objectKeys: string[]) => undefined),
  openCommand: vi.fn((_commandId: string) => undefined),
  navigate: vi.fn((_opts: unknown) => undefined),
  wizards: [] as unknown[],
}))

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (opts: RegisteredCommand) => {
    registeredCommands.push(opts)
  },
}))

vi.mock('@s4wave/web/command/CommandContext.js', () => ({
  useOpenCommand: () => h.openCommand,
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: { wizards: h.wizards },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/contexts/TabActiveContext.js', () => ({
  useIsTabActive: () => true,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => h.navigate,
}))

vi.mock('@s4wave/web/contexts/contexts.js', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@s4wave/web/contexts/contexts.js')>()
  return {
    ...actual,
    useSessionIndex: () => 1,
    useSessionNavigate: () => h.navigate,
  }
})

describe('SpaceCommands', () => {
  const mockSpace = {}

  beforeEach(() => {
    h.wizards = [
      {
        typeId: 'notes/notebook',
        displayName: 'Notebook',
        category: 'Content',
        createOpId: 'notes/notebook/init',
        keyPrefix: 'notebook/',
        persistent: true,
        wizardTypeId: 'wizard/notes/notebook',
        defaultNamePattern: 'Notebook',
      },
      {
        typeId: 'notes/docs',
        displayName: 'Documentation',
        category: 'Content',
        createOpId: 'notes/docs/create',
        keyPrefix: 'docs/',
        persistent: true,
        wizardTypeId: 'wizard/notes/docs',
        defaultNamePattern: 'Documentation',
      },
      {
        typeId: 'notes/blog',
        displayName: 'Blog',
        category: 'Content',
        createOpId: 'notes/blog/create',
        keyPrefix: 'blog/',
        persistent: true,
        wizardTypeId: 'wizard/notes/blog',
        defaultNamePattern: 'Blog',
      },
      {
        typeId: 'alpha/object-layout',
        displayName: 'Object Layout',
        category: 'Layout',
        createOpId: 'space/world/init-object-layout',
        keyPrefix: 'object-layout/',
        defaultNamePattern: 'Layout',
      },
      {
        typeId: 'forge/job',
        displayName: 'Forge Job',
        category: 'Forge',
        createOpId: 'spacewave/forge/job/create',
        keyPrefix: 'forge/job/',
        persistent: true,
        wizardTypeId: 'wizard/forge/job',
        defaultNamePattern: 'Job',
      },
      {
        typeId: 'git/repo',
        displayName: 'Git Repository',
        category: 'Files',
        createOpId: 'spacewave/git/repo/create',
        keyPrefix: 'git/repo/',
        persistent: true,
        wizardTypeId: 'wizard/git/repo',
        defaultNamePattern: 'Repository',
      },
      {
        typeId: 'forge/task',
        displayName: 'Forge Task',
        category: 'Forge',
        createOpId: 'spacewave/forge/task/create',
        keyPrefix: 'forge/task/',
        persistent: true,
        wizardTypeId: 'wizard/forge/task',
        defaultNamePattern: 'Task',
      },
    ]
  })

  function renderCommands({
    canShare = true,
    onShareSpace = vi.fn(),
  }: {
    canShare?: boolean
    onShareSpace?: () => void
  } = {}) {
    return render(
      <SharedObjectContext.Provider
        resource={{
          value: { meta: { sharedObjectId: 'so-1' } } as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      >
        <SpaceContext.Provider
          resource={{
            value: mockSpace as never,
            loading: false,
            error: null,
            retry: vi.fn(),
          }}
        >
          <SpaceContainerContext.Provider
            spaceId="space-1"
            spaceState={{ ready: true }}
            spaceWorldResource={{
              value: { applyWorldOp: h.applyWorldOp } as never,
              loading: false,
              error: null,
              retry: vi.fn(),
            }}
            spaceWorld={{ applyWorldOp: h.applyWorldOp } as never}
            navigateToRoot={vi.fn()}
            navigateToObjects={h.navigateToObjects}
            buildObjectUrls={vi.fn()}
            navigateToSubPath={vi.fn()}
          >
            <SpaceCommands
              canRename={true}
              canShare={canShare}
              onRenameSpace={vi.fn()}
              onShareSpace={onShareSpace}
            />
          </SpaceContainerContext.Provider>
        </SpaceContext.Provider>
      </SharedObjectContext.Provider>,
    )
  }

  function getShareSpaceCommand() {
    const shareSpaceCommand = [...registeredCommands]
      .reverse()
      .find((cmd) => cmd.commandId === 'spacewave.share-space')
    if (!shareSpaceCommand) {
      throw new Error('expected share-space command to be registered')
    }
    if (typeof shareSpaceCommand.handler !== 'function') {
      throw new Error('expected share-space command handler')
    }
    return shareSpaceCommand
  }

  function getCreateObjectCommandHandlers() {
    const createObjectCommand = [...registeredCommands]
      .reverse()
      .find((cmd) => cmd.commandId === 'spacewave.create-object')
    if (!createObjectCommand) {
      throw new Error('expected create-object command to be registered')
    }

    const { subItems, handler } = createObjectCommand
    if (typeof subItems !== 'function' || typeof handler !== 'function') {
      throw new Error('expected create-object command handlers')
    }

    return { subItems, handler }
  }

  afterEach(() => {
    registeredCommands.length = 0
    vi.unstubAllEnvs()
    localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
    vi.clearAllMocks()
  })

  it('opens the Space owner sharing dialog from the share-space command', () => {
    const onShareSpace = vi.fn()
    renderCommands({ onShareSpace })

    const command = getShareSpaceCommand()
    expect(command.enabled).toBe(true)
    command.handler?.({})

    expect(onShareSpace).toHaveBeenCalledOnce()
  })

  it('disables the share-space command when sharing cannot be managed', () => {
    const onShareSpace = vi.fn()
    renderCommands({ canShare: false, onShareSpace })

    const command = getShareSpaceCommand()
    expect(command.enabled).toBe(false)
    command.handler?.({})

    expect(onShareSpace).not.toHaveBeenCalled()
  })

  it('launches a persistent notes notebook wizard from the create-object command', async () => {
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('notes/notebook')

    handler({ subItemId: 'notes/notebook' })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/notebook-1')
    expect(decoded.wizardTypeId).toBe('wizard/notes/notebook')
    expect(decoded.targetTypeId).toBe('notes/notebook')
    expect(decoded.targetKeyPrefix).toBe('notebook/')
    expect(decoded.name).toBe('Notebook')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('launches a persistent notes docs wizard from the create-object command', async () => {
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('notes/docs')

    handler({ subItemId: 'notes/docs' })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/documentation-1')
    expect(decoded.wizardTypeId).toBe('wizard/notes/docs')
    expect(decoded.targetTypeId).toBe('notes/docs')
    expect(decoded.targetKeyPrefix).toBe('docs/')
    expect(decoded.name).toBe('Documentation')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('launches a persistent notes blog wizard from the create-object command', async () => {
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('notes/blog')

    handler({ subItemId: 'notes/blog' })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/blog-1')
    expect(decoded.wizardTypeId).toBe('wizard/notes/blog')
    expect(decoded.targetTypeId).toBe('notes/blog')
    expect(decoded.targetKeyPrefix).toBe('blog/')
    expect(decoded.name).toBe('Blog')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('launches a persistent forge job wizard from the create-object command', async () => {
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('forge/job')

    handler({ subItemId: 'forge/job' })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/job-1')
    expect(decoded.wizardTypeId).toBe('wizard/forge/job')
    expect(decoded.targetTypeId).toBe('forge/job')
    expect(decoded.targetKeyPrefix).toBe('forge/job/')
    expect(decoded.name).toBe('Job')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('creates an object layout from the create-object command', async () => {
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('alpha/object-layout')

    handler({ subItemId: 'alpha/object-layout' })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(INIT_OBJECT_LAYOUT_OP_ID)

    const decoded = InitObjectLayoutOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('layout-1')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('launches a persistent forge task wizard from the create-object command', async () => {
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('forge/task')

    handler({ subItemId: 'forge/task' })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/task-1')
    expect(decoded.wizardTypeId).toBe('wizard/forge/task')
    expect(decoded.targetTypeId).toBe('forge/task')
    expect(decoded.targetKeyPrefix).toBe('forge/task/')
    expect(decoded.name).toBe('Task')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('launches a persistent git repository wizard from the create-object command', async () => {
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('git/repo')

    handler({ subItemId: 'git/repo' })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/repository-1')
    expect(decoded.wizardTypeId).toBe('wizard/git/repo')
    expect(decoded.targetTypeId).toBe('git/repo')
    expect(decoded.targetKeyPrefix).toBe('git/repo/')
    expect(decoded.name).toBe('Repository')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('lists and launches the Add Device wizard from release-visible registry data', async () => {
    vi.stubEnv('DEV', false)
    h.wizards = [
      {
        typeId: DeviceTypeID,
        displayName: 'Add Device',
        category: 'Devices',
        persistent: true,
        wizardTypeId: AddDeviceWizardTypeID,
        keyPrefix: AddDeviceWizardTargetKeyPrefix,
        defaultNamePattern: AddDeviceDefaultName,
      },
    ]
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('device', new AbortController().signal)
    expect(items).toEqual([
      {
        id: DeviceTypeID,
        label: 'Add Device',
        description: 'Devices',
      },
    ])

    handler({ subItemId: DeviceTypeID })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/device-1')
    expect(decoded.wizardTypeId).toBe(AddDeviceWizardTypeID)
    expect(decoded.targetTypeId).toBe(DeviceTypeID)
    expect(decoded.targetKeyPrefix).toBe(AddDeviceWizardTargetKeyPrefix)
    expect(decoded.name).toBe(AddDeviceDefaultName)
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('launches a dynamic persistent wizard with an exact wizard type id', async () => {
    h.wizards = [
      ...h.wizards,
      {
        typeId: 'glados/workfront',
        displayName: 'Workfront',
        category: 'Glados',
        persistent: true,
        wizardTypeId: 'wizard/glados/workfront',
        keyPrefix: 'glados/workfront/',
        defaultNamePattern: 'Workfront',
      },
    ]
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('workfront', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('glados/workfront')

    handler({ subItemId: 'glados/workfront' })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/workfront-1')
    expect(decoded.wizardTypeId).toBe('wizard/glados/workfront')
    expect(decoded.targetTypeId).toBe('glados/workfront')
    expect(decoded.targetKeyPrefix).toBe('glados/workfront/')
    expect(decoded.name).toBe('Workfront')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('hides experimental create-object subitems until the browser opts in', async () => {
    vi.stubEnv('DEV', false)
    h.wizards = [
      {
        typeId: 'git/repo',
        displayName: 'Git Repository',
        category: 'Files',
        createOpId: 'spacewave/git/repo/create',
        keyPrefix: 'git/repo/',
        persistent: true,
        wizardTypeId: 'wizard/git/repo',
      },
      {
        typeId: 'forge/task',
        displayName: 'Forge Task',
        category: 'Forge',
        createOpId: 'spacewave/forge/task/create',
        keyPrefix: 'forge/task/',
        persistent: true,
        wizardTypeId: 'wizard/forge/task',
        experimental: true,
      },
    ]
    renderCommands()

    const { subItems } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)

    expect(items.map((item) => item.id)).toContain('git/repo')
    expect(items.map((item) => item.id)).not.toContain('forge/task')
  })

  it('shows experimental create-object subitems when the browser opts in', async () => {
    vi.stubEnv('DEV', false)
    setExperimentalCreatorsEnabled(true)
    h.wizards = [
      {
        typeId: 'forge/task',
        displayName: 'Forge Task',
        category: 'Forge',
        createOpId: 'spacewave/forge/task/create',
        keyPrefix: 'forge/task/',
        persistent: true,
        wizardTypeId: 'wizard/forge/task',
        experimental: true,
      },
    ]
    renderCommands()

    const { subItems } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)

    expect(items.map((item) => item.id)).toContain('forge/task')
  })
})
