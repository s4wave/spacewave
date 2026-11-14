import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, waitFor } from '@testing-library/react'

import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { InitObjectLayoutOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { INIT_OBJECT_LAYOUT_OP_ID } from '@s4wave/core/space/world/ops/init-object-layout.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import type { SubItemsCallback } from '@s4wave/web/command/CommandContext.js'
import { SharedObjectContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import {
  keysWorld,
  listingWorld,
  type TypedObject,
} from '@s4wave/web/test/world-query.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import {
  EXPERIMENTAL_CREATORS_STORAGE_KEY,
  setExperimentalCreatorsEnabled,
} from '../creator-visibility.js'
import {
  AddDeviceDefaultName,
  AddDeviceWizardTargetKeyPrefix,
  AddDeviceWizardTypeID,
} from '../device/add-device-wizard.js'
import { WorldOperationRejection } from '@s4wave/sdk/world/errors.js'
import { SpaceCommands } from './SpaceCommands.js'
import { SpaceCreatesContext, useSpaceCreates } from './useSpaceCreates.js'

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
  downloadURL: vi.fn((_url: string): Promise<void> => Promise.resolve()),
  navigateToObjects: vi.fn((_objectKeys: string[]) => undefined),
  openCommand: vi.fn((_commandId: string) => undefined),
  navigate: vi.fn((_opts: unknown) => undefined),
  toast: {
    error: vi.fn(),
    success: vi.fn(),
    dismiss: vi.fn(),
    loading: vi.fn(),
  },
  wizards: [] as unknown[],
  listWizards: vi.fn((_signal?: AbortSignal) =>
    Promise.resolve(h.wizards as never),
  ),
}))

vi.mock('@s4wave/web/download.js', () => ({
  downloadURL: h.downloadURL,
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: h.toast,
}))

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (opts: RegisteredCommand) => {
    registeredCommands.push(opts)
  },
}))

vi.mock('@s4wave/web/command/CommandContext.js', () => ({
  useOpenCommand: () => h.openCommand,
}))

vi.mock('./useObjectWizards.js', () => ({
  useObjectWizards: () => ({
    value: { wizards: h.wizards },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
  useListObjectWizards: () => h.listWizards,
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

function SpaceHarness({
  spaceId,
  world,
  canShare,
  onShareSpace,
}: {
  spaceId: string
  world: IWorldState | null
  canShare: boolean
  onShareSpace: () => void
}) {
  const creates = useSpaceCreates(spaceId, world, h.navigateToObjects)
  return (
    <SharedObjectContext.Provider
      resource={{
        value: { meta: { sharedObjectId: 'so-1' } } as never,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
    >
      <SpaceCreatesContext.Provider value={creates}>
        {world ? (
          <SpaceContainerContext.Provider
            spaceId={spaceId}
            spaceState={{ ready: true }}
            spaceWorldResource={{
              value: world as never,
              loading: false,
              error: null,
              retry: vi.fn(),
            }}
            spaceWorld={world as never}
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
        ) : null}
      </SpaceCreatesContext.Provider>
    </SharedObjectContext.Provider>
  )
}

describe('SpaceCommands', () => {
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
    h.listWizards.mockImplementation((_signal?: AbortSignal) =>
      Promise.resolve(h.wizards as never),
    )
  })

  function renderCommands({
    canShare = true,
    onShareSpace = vi.fn(),
    world = keysWorld([]),
  }: {
    canShare?: boolean
    onShareSpace?: () => void
    world?: IWorldState
  } = {}) {
    // tree renders the commands as SpaceContainer does: the Space's create
    // owner above a guard that unmounts SpaceCommands while the world is
    // null (loading). setWorld models the world mount going to loading and
    // a new world arriving; switchSpace models the Space id changing.
    const withApply = (w: IWorldState, apply = h.applyWorldOp) =>
      ({ ...w, applyWorldOp: apply }) as IWorldState
    let current: IWorldState | null = withApply(world)
    let spaceId = 'space-1'
    const tree = () => (
      <SpaceHarness
        spaceId={spaceId}
        world={current}
        canShare={canShare}
        onShareSpace={onShareSpace}
      />
    )
    const view = render(tree())
    return Object.assign(view, {
      setWorld: (w: IWorldState | null, apply?: typeof h.applyWorldOp) => {
        current = w && withApply(w, apply)
        view.rerender(tree())
      },
      switchSpace: (id: string) => {
        spaceId = id
        view.rerender(tree())
      },
    })
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

  function getExportSpaceCommand() {
    const exportSpaceCommand = [...registeredCommands]
      .reverse()
      .find((cmd) => cmd.commandId === 'spacewave.file.export')
    if (!exportSpaceCommand) {
      throw new Error('expected export-space command to be registered')
    }
    if (typeof exportSpaceCommand.handler !== 'function') {
      throw new Error('expected export-space command handler')
    }
    return exportSpaceCommand
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

  it('does not expose rejected export payloads in the error toast', async () => {
    h.downloadURL.mockRejectedValueOnce(
      '[{"id":"unixfs-demo","type":"world_object","objectKey":"files","viewPath":"/Screenshot 2026-05-12 at 9.28.36 PM.png"}]',
    )
    renderCommands()

    getExportSpaceCommand().handler?.({})

    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledWith('Export failed', {
        description: 'Space export could not be prepared.',
      })
    })
  })

  it('re-acquires object wizards when the create-object palette opens after route transition', async () => {
    const routeWizards = h.wizards
    h.wizards = []
    h.listWizards.mockResolvedValue(routeWizards as never)
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    handler({})
    expect(h.openCommand).toHaveBeenCalledWith('spacewave.create-object')
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('notes/notebook')
    expect(h.listWizards).toHaveBeenCalled()

    handler({ subItemId: 'notes/notebook' })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)
    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.targetTypeId).toBe('notes/notebook')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
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

  // failedCreateToast returns the nth failure toast's title and options.
  function failedCreateToast(n = 0) {
    return h.toast.error.mock.calls[n] as unknown as [
      string,
      {
        id?: string
        description: string
        duration?: number
        action?: { label: string; onClick: () => void }
      },
    ]
  }

  it('shows a refused create with its reason and offers the same create again', async () => {
    h.applyWorldOp.mockRejectedValueOnce(
      new WorldOperationRejection(
        'space/layout/exists',
        'A layout with this name already exists.',
      ),
    )
    renderCommands()

    const { handler } = getCreateObjectCommandHandlers()
    handler({ subItemId: 'alpha/object-layout' })

    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledTimes(1)
    })
    const [title, options] = failedCreateToast()
    expect(title).toBe('Could not create Layout')
    expect(options.description).toBe('A layout with this name already exists.')
    expect(options.duration).toBe(Infinity)
    expect(options.action?.label).toBe('Try again')
    expect(h.navigateToObjects).not.toHaveBeenCalled()

    options.action?.onClick()
    await waitFor(() => {
      expect(h.navigateToObjects).toHaveBeenCalledTimes(1)
    })
    expect(h.applyWorldOp).toHaveBeenCalledTimes(2)
    const [first, second] = h.applyWorldOp.mock.calls
    expect(second[0]).toBe(first[0])
    expect(second[1]).toEqual(first[1])
    expect(h.toast.success).toHaveBeenCalledWith('Created Layout')
  })

  it.each([
    ['alpha/object-layout', 'alpha/object-layout', 'Created Layout'],
    ['notes/notebook', 'wizard/notes/notebook', null],
  ])(
    'finishes a %s create whose reply was lost without creating it twice',
    async (typeId, objectType, success) => {
      const objects: TypedObject[] = []
      h.applyWorldOp.mockImplementationOnce((opTypeId, opData) => {
        const objectKey =
          opTypeId === CREATE_WIZARD_OBJECT_OP_ID
            ? CreateWizardObjectOp.fromBinary(opData).objectKey
            : InitObjectLayoutOp.fromBinary(opData).objectKey
        objects.push({ objectKey: objectKey ?? '', objectType })
        return Promise.reject(new Error('reply lost'))
      })
      renderCommands({ world: listingWorld(() => objects) })

      getCreateObjectCommandHandlers().handler({ subItemId: typeId })
      await waitFor(() => {
        expect(h.toast.error).toHaveBeenCalledTimes(1)
      })
      const [, options] = failedCreateToast()
      expect(options.description).toBe('The Space could not finish the change.')

      options.action?.onClick()
      await waitFor(() => {
        expect(h.navigateToObjects).toHaveBeenCalledTimes(1)
      })
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
      expect(objects).toHaveLength(1)
      expect(objects[0].objectKey).not.toBe('')
      expect(h.navigateToObjects).toHaveBeenCalledWith([objects[0].objectKey])
      if (success) expect(h.toast.success).toHaveBeenCalledWith(success)
      else expect(h.toast.success).not.toHaveBeenCalled()
    },
  )

  it('keeps one toast per create across failed retries', async () => {
    const refusal = new WorldOperationRejection(
      'space/layout/exists',
      'A layout with this name already exists.',
    )
    h.applyWorldOp.mockRejectedValueOnce(refusal).mockRejectedValueOnce(refusal)
    renderCommands()

    getCreateObjectCommandHandlers().handler({
      subItemId: 'alpha/object-layout',
    })
    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledTimes(1)
    })
    failedCreateToast()[1].action?.onClick()
    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledTimes(2)
    })

    expect(failedCreateToast(0)[1].id).toBe('create-object:layout-1')
    expect(failedCreateToast(1)[1].id).toBe('create-object:layout-1')
    expect(h.applyWorldOp).toHaveBeenCalledTimes(2)
  })

  // inFlight makes the next send hang until settle runs, which may record
  // the object as landed before the send rejects as a lost reply.
  function inFlight() {
    let settle: () => void = () => undefined
    h.applyWorldOp.mockImplementationOnce(
      () =>
        new Promise<ApplyWorldOpResult>((_resolve, reject) => {
          settle = () => reject(new Error('reply lost'))
        }),
    )
    return () => settle()
  }

  // startInFlight starts a layout create whose send hangs, and waits until
  // it is sent.
  async function startInFlight() {
    const settle = inFlight()
    const view = renderCommands()
    getCreateObjectCommandHandlers().handler({
      subItemId: 'alpha/object-layout',
    })
    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })
    const opData = h.applyWorldOp.mock.calls[0][1]
    const objectKey = InitObjectLayoutOp.fromBinary(opData).objectKey ?? ''
    return { view, settle, objectKey }
  }

  const resolvedApply = () =>
    vi.fn((_opTypeId: string, _opData: Uint8Array, _sender?: string) =>
      Promise.resolve<ApplyWorldOpResult>({ seqno: 2n, sysErr: false }),
    )

  it('reports a create that fails while its world reloads and retries on the new world', async () => {
    const { view, settle, objectKey } = await startInFlight()

    // The transport fails: the world goes to loading, the guard unmounts
    // SpaceCommands, and the send rejects.
    view.setWorld(null)
    settle()
    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledTimes(1)
    })
    const [title, options] = failedCreateToast()
    expect(title).toBe('Could not create Layout')
    expect(options.action?.label).toBe('Try again')

    const nextApply = resolvedApply()
    view.setWorld(keysWorld([]), nextApply)
    options.action?.onClick()
    await waitFor(() => {
      expect(h.navigateToObjects).toHaveBeenCalledWith([objectKey])
    })
    expect(objectKey).toBe('layout-1')
    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    expect(nextApply).toHaveBeenCalledTimes(1)
    const [opId, opData] = h.applyWorldOp.mock.calls[0]
    expect(nextApply.mock.calls[0]).toEqual([opId, opData, ''])
    expect(h.toast.dismiss).toHaveBeenCalledWith('create-object:layout-1')
    expect(h.toast.success).toHaveBeenCalledWith('Created Layout')
  })

  it('holds a Try again pressed while the world loads until it returns', async () => {
    const { view, settle, objectKey } = await startInFlight()
    view.setWorld(null)
    settle()
    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledTimes(1)
    })

    // The press replaces the failure with a loading toast at once, then
    // with the wait for the world.
    failedCreateToast()[1].action?.onClick()
    await waitFor(() => {
      expect(h.toast.loading).toHaveBeenCalledTimes(2)
    })
    expect(h.toast.loading.mock.calls[1][1]).toMatchObject({
      id: 'create-object:layout-1',
      description: 'Waiting for the Space to reconnect.',
      action: { label: 'Cancel' },
    })
    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)

    const nextApply = resolvedApply()
    view.setWorld(keysWorld([]), nextApply)
    await waitFor(() => {
      expect(h.navigateToObjects).toHaveBeenCalledWith([objectKey])
    })
    expect(nextApply).toHaveBeenCalledTimes(1)
    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
  })

  it('opens a create that landed while its world reloaded', async () => {
    const objects: TypedObject[] = []
    const { view, settle, objectKey } = await startInFlight()
    objects.push({ objectKey, objectType: 'alpha/object-layout' })
    view.setWorld(null)
    settle()
    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledTimes(1)
    })

    const nextApply = resolvedApply()
    view.setWorld(
      listingWorld(() => objects),
      nextApply,
    )
    failedCreateToast()[1].action?.onClick()
    await waitFor(() => {
      expect(h.navigateToObjects).toHaveBeenCalledWith([objectKey])
    })
    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    expect(nextApply).not.toHaveBeenCalled()
    expect(h.toast.success).toHaveBeenCalledWith('Created Layout')
  })

  it('dismisses and silences a create when the Space switches', async () => {
    const { view, settle } = await startInFlight()
    view.setWorld(null)
    settle()
    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledTimes(1)
    })
    failedCreateToast()[1].action?.onClick()
    await waitFor(() => {
      expect(h.toast.loading).toHaveBeenCalledTimes(2)
    })

    view.switchSpace('space-2')
    expect(h.toast.dismiss).toHaveBeenCalledWith('create-object:layout-1')
    const nextApply = resolvedApply()
    view.setWorld(keysWorld([]), nextApply)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(nextApply).not.toHaveBeenCalled()
    expect(h.navigateToObjects).not.toHaveBeenCalled()
    expect(h.toast.error).toHaveBeenCalledTimes(1)
  })

  it('dismisses a failed create toast when its Space closes', async () => {
    h.applyWorldOp.mockRejectedValueOnce(new Error('offline'))
    const view = renderCommands()

    getCreateObjectCommandHandlers().handler({
      subItemId: 'alpha/object-layout',
    })
    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledTimes(1)
    })
    const id = failedCreateToast()[1].id
    expect(h.toast.dismiss).not.toHaveBeenCalled()

    view.unmount()
    expect(h.toast.dismiss).toHaveBeenCalledWith(id)
  })

  it('shows no failure toast for a create that fails after its Space closed', async () => {
    let reject: (err: Error) => void = () => undefined
    h.applyWorldOp.mockImplementationOnce(
      () =>
        new Promise<ApplyWorldOpResult>((_resolve, rejectOp) => {
          reject = rejectOp
        }),
    )
    const view = renderCommands()

    getCreateObjectCommandHandlers().handler({
      subItemId: 'alpha/object-layout',
    })
    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })
    view.unmount()
    reject(new Error('world released'))
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(h.toast.error).not.toHaveBeenCalled()
  })

  it('does not retry a create whose key another object took', async () => {
    const objects: TypedObject[] = []
    h.applyWorldOp.mockImplementationOnce((_opTypeId, opData) => {
      const objectKey = InitObjectLayoutOp.fromBinary(opData).objectKey ?? ''
      objects.push({ objectKey, objectType: 'other/type' })
      return Promise.reject(new Error('reply lost'))
    })
    renderCommands({ world: listingWorld(() => objects) })

    getCreateObjectCommandHandlers().handler({
      subItemId: 'alpha/object-layout',
    })
    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledTimes(1)
    })
    failedCreateToast()[1].action?.onClick()

    await waitFor(() => {
      expect(h.toast.error).toHaveBeenCalledTimes(2)
    })
    const [title, options] = failedCreateToast(1)
    expect(title).toBe('Could not create Layout')
    expect(options.description).toBe('The Space could not finish the change.')
    expect(options.action).toBeUndefined()
    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    expect(h.navigateToObjects).not.toHaveBeenCalled()
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
        iconName: undefined,
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

  it('maps wizard descriptions and icons into create-object sub-items', async () => {
    h.wizards = [
      {
        typeId: 'canvas',
        displayName: 'Canvas',
        description: 'Freeform canvas for drawing and layout',
        category: 'Layout',
        iconName: 'LuLayoutGrid',
        createOpId: 'space/world/init-canvas',
        keyPrefix: 'canvas/',
        defaultNamePattern: 'Canvas',
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
    ]
    renderCommands()

    const { subItems } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items).toEqual([
      {
        id: 'canvas',
        label: 'Canvas',
        description: 'Freeform canvas for drawing and layout',
        iconName: 'LuLayoutGrid',
      },
      {
        id: 'notes/blog',
        label: 'Blog',
        description: 'Content',
        iconName: undefined,
      },
    ])

    const filtered = await subItems('freeform', new AbortController().signal)
    expect(filtered.map((item) => item.id)).toEqual(['canvas'])
  })

  it('launches a dynamic persistent wizard with an exact wizard type id', async () => {
    h.wizards = [
      ...h.wizards,
      {
        typeId: 'gizmo/worklist',
        displayName: 'Worklist',
        category: 'Gizmo',
        persistent: true,
        wizardTypeId: 'wizard/gizmo/worklist',
        keyPrefix: 'gizmo/worklist/',
        defaultNamePattern: 'Worklist',
      },
    ]
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('worklist', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('gizmo/worklist')

    handler({ subItemId: 'gizmo/worklist' })

    await waitFor(() => {
      expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const [opTypeId, opData] = h.applyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/worklist-1')
    expect(decoded.wizardTypeId).toBe('wizard/gizmo/worklist')
    expect(decoded.targetTypeId).toBe('gizmo/worklist')
    expect(decoded.targetKeyPrefix).toBe('gizmo/worklist/')
    expect(decoded.name).toBe('Worklist')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('lists release-visible registered app wizards in the create-object command', async () => {
    vi.stubEnv('DEV', false)
    h.wizards = [
      {
        typeId: 'spacewave-chat/channel',
        displayName: 'Chat Channel',
        category: 'Chat',
        createOpId: 'spacewave-chat/channel/create',
        keyPrefix: 'chat/channel/',
        defaultNamePattern: 'Chat',
      },
      {
        typeId: 'notes/docs',
        displayName: 'Documentation',
        category: 'Content',
        persistent: true,
        wizardTypeId: 'wizard/notes/docs',
        keyPrefix: 'docs/',
        defaultNamePattern: 'Documentation',
      },
      {
        typeId: 'notes/blog',
        displayName: 'Blog',
        category: 'Content',
        persistent: true,
        wizardTypeId: 'wizard/notes/blog',
        keyPrefix: 'blog/',
        defaultNamePattern: 'Blog',
      },
      {
        typeId: 'spacewave/forge/dashboard',
        displayName: 'Forge Dashboard',
        category: 'Forge',
        createOpId: 'spacewave/forge/dashboard/create',
        keyPrefix: 'forge/',
        defaultNamePattern: 'Forge',
      },
      {
        typeId: 'vm/v86',
        displayName: 'x86 VM',
        category: 'VM',
        persistent: true,
        wizardTypeId: 'wizard/vm/v86',
        keyPrefix: 'vm/v86/',
        defaultNamePattern: 'VM',
      },
    ]
    renderCommands()

    const { subItems } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toEqual([
      'spacewave-chat/channel',
      'notes/docs',
      'notes/blog',
      'spacewave/forge/dashboard',
      'vm/v86',
    ])
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
    expect(
      items.find((item) => item.id === 'git/repo')?.experimental,
    ).toBeUndefined()
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
    expect(items.find((item) => item.id === 'forge/task')?.experimental).toBe(
      true,
    )
  })
})
