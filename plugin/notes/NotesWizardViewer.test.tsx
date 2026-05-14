import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const h = vi.hoisted(() => ({
  createNotebookClientSide: vi.fn().mockResolvedValue(undefined),
  createDocsClientSide: vi.fn().mockResolvedValue(undefined),
  createBlogClientSide: vi.fn().mockResolvedValue(undefined),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
  navigateToObjects: vi.fn(),
  persistDraftState: vi.fn().mockResolvedValue(undefined),
  setCreating: vi.fn(),
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
  existingObjectKeys: [] as string[],
}))

vi.mock('@s4wave/app/wizard/useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/notes/test',
    state: h.state,
    localName: h.localName,
    creating: false,
    setCreating: h.setCreating,
    spaceWorld: {
      deleteObject: h.deleteObject,
    },
    existingObjectKeys: h.existingObjectKeys,
    navigateToObjects: h.navigateToObjects,
    persistDraftState: h.persistDraftState,
    handleUpdateName: h.handleUpdateName,
    handleBack: h.handleBack,
    handleCancel: h.handleCancel,
  }),
}))

vi.mock('@s4wave/app/wizard/WizardShell.js', () => ({
  WizardShell: ({
    title,
    canFinalize,
    onFinalize,
  }: {
    title: React.ReactNode
    canFinalize?: boolean
    onFinalize: () => void
  }) => (
    <button
      type="button"
      aria-label="create"
      disabled={!canFinalize}
      onClick={onFinalize}
    >
      {title}
    </button>
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

vi.mock('./content-seed.js', () => ({
  buildNotebookUnixfsObjectKey: (objectKey: string) => objectKey + '-fs',
  createDocsClientSide: h.createDocsClientSide,
  createNotebookClientSide: h.createNotebookClientSide,
}))

vi.mock('./blog-seed.js', () => ({
  createBlogClientSide: h.createBlogClientSide,
}))

import { NotesWizardViewer } from './NotesWizardViewer.js'

describe('NotesWizardViewer', () => {
  beforeEach(() => {
    h.state = {
      step: 0,
      targetTypeId: 'notes/notebook',
      targetKeyPrefix: 'notebook/',
      name: 'Notebook',
    }
    h.localName = 'Notebook'
    h.existingObjectKeys = []
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

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

  it('finalizes a notebook through plugin-owned seed creation', async () => {
    const user = userEvent.setup()
    renderViewer()

    await user.click(screen.getByRole('button', { name: 'create' }))

    await waitFor(() => {
      expect(h.createNotebookClientSide).toHaveBeenCalledTimes(1)
    })
    expect(h.createNotebookClientSide).toHaveBeenCalledWith(
      expect.objectContaining({ deleteObject: h.deleteObject }),
      'notebook-1',
      'notebook-1-fs',
      'Notebook',
      expect.any(Date),
    )
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/notes/test')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['notebook-1'])
    expect(h.toastSuccess).toHaveBeenCalledWith('Created Notebook')
  })

  it('finalizes documentation through plugin-owned seed creation', async () => {
    const user = userEvent.setup()
    h.state = {
      step: 0,
      targetTypeId: 'notes/docs',
      targetKeyPrefix: 'docs/',
      name: 'Documentation',
    }
    h.localName = 'Documentation'
    h.existingObjectKeys = ['documentation-1']
    renderViewer()

    await user.click(screen.getByRole('button', { name: 'create' }))

    await waitFor(() => {
      expect(h.createDocsClientSide).toHaveBeenCalledTimes(1)
    })
    expect(h.createDocsClientSide).toHaveBeenCalledWith(
      expect.objectContaining({ deleteObject: h.deleteObject }),
      'documentation-2',
      'Documentation',
      '',
      expect.any(Date),
    )
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/notes/test')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['documentation-2'])
  })

  it('finalizes a blog through plugin-owned seed creation', async () => {
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
      expect(h.createBlogClientSide).toHaveBeenCalledTimes(1)
    })
    expect(h.createBlogClientSide).toHaveBeenCalledWith(
      expect.objectContaining({ deleteObject: h.deleteObject }),
      'blog-1',
      'Blog',
      '',
      '',
      expect.any(Date),
    )
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/notes/test')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['blog-1'])
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

    expect(h.createNotebookClientSide).not.toHaveBeenCalled()
    expect(h.createDocsClientSide).not.toHaveBeenCalled()
    expect(h.createBlogClientSide).not.toHaveBeenCalled()
  })
})
