import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, waitFor } from '@testing-library/react'

import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { InitObjectLayoutOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { INIT_OBJECT_LAYOUT_OP_ID } from '@s4wave/core/space/world/ops/init-object-layout.js'
import type { SubItemsCallback } from '@s4wave/web/command/CommandContext.js'
import {
  SharedObjectContext,
  SpaceContext,
} from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { SpaceCommands } from './SpaceCommands.js'

interface RegisteredCommand {
  commandId?: string
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
  createNotebookClientSide: vi.fn().mockResolvedValue(undefined),
  createDocsClientSide: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (opts: RegisteredCommand) => {
    registeredCommands.push(opts)
  },
}))

vi.mock('@s4wave/web/command/CommandContext.js', () => ({
  useOpenCommand: () => h.openCommand,
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

vi.mock('../../plugin/notes/content-seed.js', () => ({
  buildNotebookUnixfsObjectKey: (objectKey: string) => objectKey + '-fs',
  createNotebookClientSide: h.createNotebookClientSide,
  createDocsClientSide: h.createDocsClientSide,
}))

describe('SpaceCommands', () => {
  const mockSpace = {
    listWizards: vi.fn().mockResolvedValue([
      {
        typeId: 'notebook',
        displayName: 'Notebook',
        category: 'Content',
        createOpId: 'spacewave-notes/notes/init-notebook',
        keyPrefix: 'notebook/',
        defaultNamePattern: 'Notebook',
      },
      {
        typeId: 'docs',
        displayName: 'Documentation',
        category: 'Content',
        createOpId: 'spacewave-notes/docs/create',
        keyPrefix: 'docs/',
        defaultNamePattern: 'Documentation',
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
    ]),
  }

  function renderCommands() {
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
            <SpaceCommands canRename={true} onRenameSpace={vi.fn()} />
          </SpaceContainerContext.Provider>
        </SpaceContext.Provider>
      </SharedObjectContext.Provider>,
    )
  }

  function getCreateObjectCommandHandlers() {
    const createObjectCommand = registeredCommands.find(
      (cmd) => cmd.commandId === 'spacewave.create-object',
    )
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
    vi.clearAllMocks()
  })

  it('creates a notebook through the merged spacewave-app client-side path', async () => {
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('notebook')

    handler({ subItemId: 'notebook' })

    await waitFor(() => {
      expect(h.createNotebookClientSide).toHaveBeenCalledTimes(1)
    })

    expect(h.applyWorldOp).not.toHaveBeenCalled()

    expect(h.createNotebookClientSide).toHaveBeenCalledWith(
      expect.anything(),
      'notebook/notebook-1',
      'notebook/notebook-1-fs',
      'Notebook',
      expect.any(Date),
    )
    expect(h.navigateToObjects).toHaveBeenCalledWith(['notebook/notebook-1'])
  })

  it('creates docs through the merged spacewave-app client-side path', async () => {
    renderCommands()

    const { subItems, handler } = getCreateObjectCommandHandlers()
    const items = await subItems('', new AbortController().signal)
    expect(items.map((item) => item.id)).toContain('docs')

    handler({ subItemId: 'docs' })

    await waitFor(() => {
      expect(h.createDocsClientSide).toHaveBeenCalledTimes(1)
    })

    expect(h.applyWorldOp).not.toHaveBeenCalled()

    expect(h.createDocsClientSide).toHaveBeenCalledWith(
      expect.anything(),
      'docs/docs-1',
      'Documentation',
      '',
      expect.any(Date),
    )
    expect(h.navigateToObjects).toHaveBeenCalledWith(['docs/docs-1'])
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
    expect(decoded.objectKey).toMatch(/^wizard\/forge\/job\/[a-z0-9]+$/)
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
    expect(decoded.objectKey).toBe('object-layout/object-layout-1')
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
    expect(decoded.objectKey).toMatch(/^wizard\/forge\/task\/[a-z0-9]+$/)
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
    expect(decoded.objectKey).toMatch(/^wizard\/git\/repo\/[a-z0-9]+$/)
    expect(decoded.wizardTypeId).toBe('wizard/git/repo')
    expect(decoded.targetTypeId).toBe('git/repo')
    expect(decoded.targetKeyPrefix).toBe('git/repo/')
    expect(decoded.name).toBe('Repository')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })
})
