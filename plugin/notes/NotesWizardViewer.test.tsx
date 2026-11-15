import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { keysWorld } from '@s4wave/web/test/world-query.js'

const h = vi.hoisted(() => ({
  t: undefined as unknown as WizardTestWorld,
  navigateToObjects: vi.fn(),
  persistDraftState: vi.fn().mockResolvedValue(undefined),
  handleUpdateName: vi.fn(),
  handleBack: vi.fn(),
  handleCancel: vi.fn().mockResolvedValue(undefined),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  state: {
    step: 0,
    targetTypeId: 'notes/notebook',
    targetKeyPrefix: 'notebook/',
    name: 'Notebook',
  },
  localName: 'Notebook',
  worldKeys: [] as string[],
  stageCreate: vi.fn(),
}))

// The stage builders return a staged op naming their arguments, so a test
// sees which content each Finish staged and which op it sent.
vi.mock('./staged-seed.js', () => {
  const staged =
    (opTypeId: string) =>
    (_stage: unknown, ...args: unknown[]) => {
      h.stageCreate(opTypeId, ...args)
      return Promise.resolve({
        opTypeId,
        opData: new TextEncoder().encode(String(args[0])),
      })
    }
  return {
    stageNotebookCreate: staged('notes/notebook/init-staged'),
    stageDocsCreate: staged('notes/docs/create-staged'),
    stageBlogCreate: staged('notes/blog/create-staged'),
  }
})

vi.mock('@s4wave/app/wizard/useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/notes/test',
    state: h.state,
    localName: h.localName,
    sessionPeerId: 'peer-1',
    spaceSettings: { indexPath: 'wizard/notes/test' },
    spaceWorld: h.t.world,
    navigateToObjects: h.navigateToObjects,
    persistDraftState: h.persistDraftState,
    handleUpdateName: h.handleUpdateName,
    handleBack: h.handleBack,
    cancel: { status: 'idle' },
    handleCancel: h.handleCancel,
  }),
}))

vi.mock('@s4wave/app/wizard/WizardShell.js', () => ({
  WizardShell: ({
    title,
    canFinalize,
    onFinalize,
    finish,
  }: {
    title: React.ReactNode
    canFinalize?: boolean
    onFinalize: () => void
    finish: WizardFinishState
  }) => (
    <>
      <button
        type="button"
        aria-label="create"
        disabled={!canFinalize}
        onClick={onFinalize}
      >
        {title}
      </button>
      {finish.status === 'failed' && <div role="alert">{finish.error}</div>}
    </>
  ),
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: h.toastSuccess,
    error: h.toastError,
  },
}))

vi.mock('@s4wave/web/ui/loading/LoadingCard.js', () => ({
  LoadingCard: ({ view }: { view: { title?: string } }) => (
    <div>{view.title}</div>
  ),
}))

import type { WizardFinishState } from '@s4wave/app/wizard/wizard-finish.js'
import {
  wizardTestWorld,
  type WizardTestWorld,
} from '@s4wave/app/wizard/wizard-test-world.js'
import { NotesWizardViewer } from './NotesWizardViewer.js'
import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { CREATE_BLOG_STAGED_OP_ID } from './proto/create-blog.js'
import { CREATE_DOCS_STAGED_OP_ID } from './proto/create-docs.js'
import { INIT_NOTEBOOK_STAGED_OP_ID } from './proto/init-notebook.js'

describe('NotesWizardViewer', () => {
  beforeEach(() => {
    h.state = {
      step: 0,
      targetTypeId: 'notes/notebook',
      targetKeyPrefix: 'notebook/',
      name: 'Notebook',
    }
    h.localName = 'Notebook'
    h.worldKeys = []
    h.t = wizardTestWorld(['wizard/notes/test'], 'wizard/notes/test', {
      ...keysWorld(() => h.worldKeys),
      // Each Finish opens a stage; releasing it records 'release stage'.
      stageWorldState: () => {
        const stage = {
          release: () => h.t.calls.push('release stage'),
          [Symbol.dispose]: () => stage.release(),
        }
        return Promise.resolve(stage as never)
      },
    })
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  // opKey returns the key the staged op the Finish applied names, by op id.
  function opKey(opId: string) {
    const data = h.t.opData.get(opId)
    if (!data) throw new Error('expected op ' + opId)
    return new TextDecoder().decode(data)
  }

  function renderViewer() {
    return render(
      <NotesWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )
  }

  it('finalizes a notebook through the plugin-owned staged create op', async () => {
    const user = userEvent.setup()
    renderViewer()

    await user.click(screen.getByRole('button', { name: 'create' }))

    await waitFor(() => {
      expect(h.navigateToObjects).toHaveBeenCalledWith(['notebook-1'])
    })
    expect(h.stageCreate).toHaveBeenCalledWith(
      INIT_NOTEBOOK_STAGED_OP_ID,
      'notebook-1',
      'notebook-1-fs',
      'Notebook',
    )
    expect(opKey(INIT_NOTEBOOK_STAGED_OP_ID)).toBe('notebook-1')
    // The create, the index move and the wizard delete commit together, and
    // the stage holding the content is released only once they have.
    expect(h.t.state).toEqual({
      objects: new Set(),
      indexPath: 'notebook-1',
      creates: [INIT_NOTEBOOK_STAGED_OP_ID],
    })
    expect(h.t.calls).toEqual([
      'batch',
      INIT_NOTEBOOK_STAGED_OP_ID,
      SET_SPACE_INDEX_PATH_OP_ID,
      'delete',
      'commit',
      'release stage',
    ])
    expect(h.navigateToObjects).toHaveBeenCalledWith(['notebook-1'])
    expect(h.toastSuccess).toHaveBeenCalledWith('Created Notebook')
  })

  it('finalizes documentation through the plugin-owned create op', async () => {
    const user = userEvent.setup()
    h.state = {
      step: 0,
      targetTypeId: 'notes/docs',
      targetKeyPrefix: 'docs/',
      name: 'Documentation',
    }
    h.localName = 'Documentation'
    h.worldKeys = ['documentation-1']
    renderViewer()

    await user.click(screen.getByRole('button', { name: 'create' }))

    await waitFor(() => {
      expect(h.navigateToObjects).toHaveBeenCalledWith(['documentation-2'])
    })
    expect(h.stageCreate).toHaveBeenCalledWith(
      CREATE_DOCS_STAGED_OP_ID,
      'documentation-2',
      'Documentation',
      expect.any(Date),
    )
    expect(opKey(CREATE_DOCS_STAGED_OP_ID)).toBe('documentation-2')
    expect(h.t.state.indexPath).toBe('documentation-2')
  })

  it('finalizes a blog through the plugin-owned create op', async () => {
    const user = userEvent.setup()
    h.state = {
      step: 0,
      targetTypeId: 'notes/blog',
      targetKeyPrefix: 'blog/',
      name: 'Blog',
    }
    h.localName = 'Blog'
    renderViewer()

    await user.click(screen.getByRole('button', { name: 'create' }))

    await waitFor(() => {
      expect(h.navigateToObjects).toHaveBeenCalledWith(['blog-1'])
    })
    expect(h.stageCreate).toHaveBeenCalledWith(
      CREATE_BLOG_STAGED_OP_ID,
      'blog-1',
      'Blog',
      expect.any(Date),
    )
    expect(opKey(CREATE_BLOG_STAGED_OP_ID)).toBe('blog-1')
    expect(h.t.state.indexPath).toBe('blog-1')
  })

  it('keeps the wizard and writes nothing when the wizard delete fails, then Retry finishes once', async () => {
    const user = userEvent.setup()
    h.t.failOn = 'delete'
    renderViewer()

    // The create applied, but the failed delete discards it with the rest.
    await user.click(screen.getByRole('button', { name: 'create' }))
    expect((await screen.findByRole('alert')).textContent).toBe('delete failed')
    expect(h.t.state).toEqual({
      objects: new Set(['wizard/notes/test']),
      indexPath: 'wizard/notes/test',
      creates: [],
    })
    expect(h.navigateToObjects).not.toHaveBeenCalled()
    // The failed list released its stage; Retry stages again.
    expect(h.t.calls.at(-1)).toBe('release stage')

    // Retry runs the whole Finish again and creates one notebook.
    h.t.failOn = undefined
    await user.click(screen.getByRole('button', { name: 'create' }))
    await waitFor(() => {
      expect(h.navigateToObjects).toHaveBeenCalledWith(['notebook-1'])
    })
    expect(h.t.state.creates).toEqual([INIT_NOTEBOOK_STAGED_OP_ID])
    expect(h.stageCreate).toHaveBeenCalledTimes(2)
  })

  it('does not finalize unknown notes wizard targets', async () => {
    const user = userEvent.setup()
    h.state = {
      step: 0,
      targetTypeId: 'notes/unknown',
      targetKeyPrefix: 'notes/',
      name: 'Unknown',
    }
    h.localName = 'Unknown'
    renderViewer()

    const button = screen.getByRole<HTMLButtonElement>('button', {
      name: 'create',
    })
    expect(button.disabled).toBe(true)
    await user.click(button)

    expect(h.t.calls).toEqual([])
  })
})
