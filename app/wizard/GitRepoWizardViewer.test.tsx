import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CreateGitRepoWizardOp } from '@s4wave/core/git/git.pb.js'
import {
  GitCloneProgressState,
  type GitCloneProgress,
} from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { keysWorld } from '@s4wave/web/test/world-query.js'

import { GitRepoWizardViewer } from './GitRepoWizardViewer.js'
import { wizardTestWorld, type WizardTestWorld } from './wizard-test-world.js'

const h = vi.hoisted(() => ({
  isDesktop: false,
  t: undefined as unknown as WizardTestWorld,
  navigateToObjects: vi.fn(),
  updateState: vi.fn().mockResolvedValue({}),
  startGitClone: vi.fn().mockResolvedValue({}),
  persistDraftState: vi.fn().mockResolvedValue(undefined),
  handleUpdateName: vi.fn(),
  handleBack: vi.fn(),
  handleCancel: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}))

let currentStep = 0
let localName = 'Repository'
let currentProgress: GitCloneProgress | null = null
let spaceSettingsIndexPath = 'wizard/git/repo/test'
let configValue: CreateGitRepoWizardOp = {
  clone: true,
  cloneOpts: { url: 'https://github.com/urfave/cli' },
}

vi.mock('@aptre/bldr', () => ({
  get isDesktop() {
    return h.isDesktop
  },
  cleanPath: (path: string) => '/' + path.split('/').filter(Boolean).join('/'),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({ value: currentProgress }),
}))

vi.mock('./useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/git/repo/test',
    state: {
      step: currentStep,
      targetTypeId: 'git/repo',
      targetKeyPrefix: 'git/repo/',
      name: localName,
    },
    localName,
    sessionPeerId: '12D3KooWGitPeer',
    spaceWorld: h.t.world,
    spaceSettings: {
      indexPath: spaceSettingsIndexPath,
      pluginIds: ['spacewave-web'],
    },
    navigateToObjects: h.navigateToObjects,
    wizardResource: {
      value: {
        updateState: h.updateState,
        startGitClone: h.startGitClone,
        watchGitCloneProgress: vi.fn(),
      },
    },
    configEditor: {
      element: <div>Git Repo Config</div>,
      value: configValue,
    },
    configData: undefined,
    persistDraftState: h.persistDraftState,
    handleConfigDataChange: vi.fn(),
    handleUpdateName: h.handleUpdateName,
    handleBack: h.handleBack,
    handleCancel: h.handleCancel,
  }),
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: h.toastSuccess,
    error: h.toastError,
  },
}))

describe('GitRepoWizardViewer', () => {
  beforeEach(() => {
    h.isDesktop = true
    currentStep = 0
    localName = 'Repository'
    currentProgress = null
    spaceSettingsIndexPath = 'wizard/git/repo/test'
    configValue = {
      clone: true,
      cloneOpts: { url: 'https://github.com/urfave/cli' },
    }
    h.t = wizardTestWorld(
      ['wizard/git/repo/test'],
      'wizard/git/repo/test',
      keysWorld(() => []),
    )
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('defaults the repository name from the clone URL', async () => {
    render(
      <GitRepoWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await waitFor(() => {
      expect(h.handleUpdateName).toHaveBeenCalledWith('cli')
    })
  })

  it('selects the prefilled repository name when the name step is focused', () => {
    currentStep = 1
    localName = 'cli'

    render(
      <GitRepoWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    const nameInput = screen.getByLabelText<HTMLInputElement>('Repository Name')
    expect(document.activeElement).toBe(nameInput)
    expect(nameInput.selectionStart).toBe(0)
    expect(nameInput.selectionEnd).toBe(nameInput.value.length)
  })

  it('starts an async clone and advances to the progress step', async () => {
    const user = userEvent.setup()
    currentStep = 1
    localName = 'cli'
    render(
      <GitRepoWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /clone/i }))

    expect(h.updateState).toHaveBeenCalledWith({ step: 2 })
    expect(h.t.calls).toEqual([])
    expect(h.startGitClone).toHaveBeenCalledTimes(1)
    const [req] = h.startGitClone.mock.calls[0] as [
      {
        objectKey: string
        name: string
        configData: Uint8Array
        opSender: string
      },
    ]
    expect(req.name).toBe('cli')
    expect(req.objectKey).toBe('cli-1')
    expect(req.opSender).toBe('12D3KooWGitPeer')
    const op = CreateGitRepoWizardOp.fromBinary(req.configData)
    expect(op.clone).toBe(true)
    expect(op.cloneOpts?.url).toBe('https://github.com/urfave/cli')
  })

  it('blocks GitHub clone URLs in the web browser before advancing', async () => {
    const user = userEvent.setup()
    h.isDesktop = false
    render(
      <GitRepoWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(
      screen.getByText(/browser cannot clone directly from GitHub/i),
    ).toBeTruthy()
    const next = screen.getByRole<HTMLButtonElement>('button', {
      name: /next/i,
    })
    expect(next.disabled).toBe(true)
    await user.click(next)

    expect(h.updateState).not.toHaveBeenCalled()
  })

  it('allows non-GitHub clone URLs in the web browser', async () => {
    const user = userEvent.setup()
    h.isDesktop = false
    configValue = {
      clone: true,
      cloneOpts: { url: 'https://gitlab.com/example/project.git' },
    }
    render(
      <GitRepoWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /next/i }))

    expect(h.updateState).toHaveBeenCalledWith({ step: 1 })
  })

  it('updates the space index before deleting the wizard for a created repo', async () => {
    const user = userEvent.setup()
    currentStep = 1
    localName = 'cli'
    configValue = { clone: false }

    render(
      <GitRepoWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /create/i }))

    // The repo, the index move and the wizard delete committed together.
    await waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith(['cli-1']),
    )
    expect(h.t.state).toEqual({
      objects: new Set(),
      indexPath: 'cli-1',
      creates: ['spacewave/git/repo/create'],
    })
  })

  it('does not promote a created repo when the wizard is not the indexed surface', async () => {
    const user = userEvent.setup()
    currentStep = 1
    localName = 'cli'
    spaceSettingsIndexPath = 'canvas/main'
    configValue = { clone: false }
    h.t = wizardTestWorld(
      ['wizard/git/repo/test'],
      'canvas/main',
      keysWorld(() => []),
    )

    render(
      <GitRepoWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith(['cli-1']),
    )
    expect(h.t.state).toEqual({
      objects: new Set(),
      indexPath: 'canvas/main',
      creates: ['spacewave/git/repo/create'],
    })
  })

  it('navigates to the repo when clone progress completes', async () => {
    currentStep = 2
    localName = 'cli'
    currentProgress = {
      state: GitCloneProgressState.DONE,
      message: 'Repository cloned.',
      objectKey: 'git/repo/cli',
    }

    render(
      <GitRepoWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await waitFor(() => {
      expect(h.navigateToObjects).toHaveBeenCalledWith(['git/repo/cli'])
    })
    expect(h.toastSuccess).toHaveBeenCalledWith('Cloned cli')
  })

  it('explains GitHub browser CORS failures from clone progress', () => {
    currentStep = 2
    currentProgress = {
      state: GitCloneProgressState.FAILED,
      message: 'Clone failed.',
      error:
        'clone: http transport: unexpected requesting "https://github.com/pkg/errors/info/refs?service=git-upload-pack" status code: 500: Failed to fetch',
    }

    render(
      <GitRepoWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(
      screen.getByText(
        /GitHub does not set the required cross-origin headers/i,
      ),
    ).toBeTruthy()
  })
})
