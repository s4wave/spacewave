import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CreateGitRepoWizardOp } from '@s4wave/core/git/git.pb.js'
import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import {
  GitCloneProgressState,
  type GitCloneProgress,
} from '@s4wave/sdk/world/wizard/wizard.pb.js'

import { GitRepoWizardViewer } from './GitRepoWizardViewer.js'

const h = vi.hoisted(() => ({
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
  navigateToObjects: vi.fn(),
  setCreating: vi.fn(),
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
let configValue: CreateGitRepoWizardOp = {
  clone: true,
  cloneOpts: { url: 'https://github.com/urfave/cli' },
}

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
    creating: false,
    setCreating: h.setCreating,
    sessionPeerId: '12D3KooWGitPeer',
    spaceWorld: {
      applyWorldOp: h.applyWorldOp,
      deleteObject: h.deleteObject,
    },
    spaceSettings: {
      indexPath: 'wizard/git/repo/test',
      pluginIds: ['spacewave-web'],
    },
    existingObjectKeys: [],
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
    currentStep = 0
    localName = 'Repository'
    currentProgress = null
    configValue = {
      clone: true,
      cloneOpts: { url: 'https://github.com/urfave/cli' },
    }
  })

  afterEach(() => {
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
    expect(h.applyWorldOp).not.toHaveBeenCalled()
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

    expect(h.applyWorldOp).toHaveBeenCalledTimes(2)
    expect(h.applyWorldOp.mock.calls[1]?.[0]).toBe(SET_SPACE_SETTINGS_OP_ID)
    const settingsOp = SetSpaceSettingsOp.fromBinary(
      h.applyWorldOp.mock.calls[1]?.[1] as Uint8Array,
    )
    expect(settingsOp.settings?.indexPath).toBe('cli-1')
    expect(settingsOp.settings?.pluginIds).toEqual(['spacewave-web'])
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/git/repo/test')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['cli-1'])
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
})
