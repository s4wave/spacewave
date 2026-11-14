import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { IntroWizardConfig } from '@s4wave/sdk/world/wizard/wizard.pb.js'

import { IntroWizardViewer } from './IntroWizardViewer.js'
import { driveIntroConfig } from './intro.js'
import { wizardTestWorld, type WizardTestWorld } from './wizard-test-world.js'

const h = vi.hoisted(() => ({
  t: undefined as unknown as WizardTestWorld,
  hasState: true,
  navigateToObjects: vi.fn(),
  spaceSettingsIndexPath: 'wizard/welcome-1',
  handleCancel: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('./useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/welcome-1',
    state: h.hasState
      ? {
          step: 0,
          targetTypeId: 'unixfs/fs-node',
          targetKeyPrefix: 'files',
          name: 'Welcome',
          configData: IntroWizardConfig.toBinary(driveIntroConfig()),
        }
      : undefined,
    localName: 'Welcome',
    sessionPeerId: '12D3KooWIntroPeer',
    spaceWorld: h.t.world,
    spaceSettings: {
      indexPath: h.spaceSettingsIndexPath,
      pluginIds: ['spacewave-web'],
    },
    navigateToObjects: h.navigateToObjects,
    wizardResource: { value: {} },
    configEditor: { element: null, value: undefined },
    configData: undefined,
    persistDraftState: vi.fn().mockResolvedValue(undefined),
    handleConfigDataChange: vi.fn(),
    handleUpdateName: vi.fn(),
    handleBack: vi.fn(),
    cancel: { status: 'idle' },
    handleCancel: h.handleCancel,
  }),
}))

vi.mock('@s4wave/web/object/ObjectViewer.js', () => ({
  ObjectViewer: () => <div data-testid="object-viewer" />,
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    error: h.toastError,
  },
}))

const viewerProps = {
  objectInfo: {},
  worldState: {
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  },
} as unknown as ObjectViewerComponentProps

// advanceToFinish walks the ordered tour to its last step, where the finish
// control appears, then clicks it. driveIntroConfig supplies three callouts, so
// the finish action is two Next steps away from the opening callout.
async function advanceToFinish(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await user.click(
    screen.getByRole('button', { name: /got it, start exploring/i }),
  )
}

// expectFinished checks the index names the introduced object and the
// wizard is gone, with nothing created.
async function expectFinished() {
  await waitFor(() =>
    expect(h.navigateToObjects).toHaveBeenCalledWith(['files']),
  )
  expect(h.t.state).toEqual({
    objects: new Set(['files']),
    indexPath: 'files',
    creates: [],
  })
}

describe('IntroWizardViewer', () => {
  beforeEach(() => {
    h.hasState = true
    h.spaceSettingsIndexPath = 'wizard/welcome-1'
    h.t = wizardTestWorld(['wizard/welcome-1', 'files'], 'wizard/welcome-1')
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders the intro content around the contained object viewer', () => {
    render(<IntroWizardViewer {...viewerProps} />)

    expect(screen.getByTestId('object-viewer')).toBeTruthy()
    expect(screen.getByText('Welcome to your Drive')).toBeTruthy()
    // The tour opens on the first callout only; later callouts stay hidden.
    expect(screen.getByText('Add files')).toBeTruthy()
    expect(screen.queryByText('Upload progress')).toBeNull()
  })

  it('sets the index to the introduced object, deletes the wizard, and navigates', async () => {
    const user = userEvent.setup()

    render(<IntroWizardViewer {...viewerProps} />)

    await advanceToFinish(user)

    await expectFinished()
  })

  it('shows the gone screen when another tab already removed the wizard', async () => {
    const user = userEvent.setup()
    h.spaceSettingsIndexPath = 'files'
    h.t = wizardTestWorld(['files'], 'files')

    render(<IntroWizardViewer {...viewerProps} />)

    await advanceToFinish(user)

    // The list did nothing, so this tab neither reports done nor opens.
    expect(
      await screen.findByText('This wizard no longer exists.'),
    ).toBeTruthy()
    expect(h.navigateToObjects).not.toHaveBeenCalled()
    expect(h.toastError).not.toHaveBeenCalled()
    expect(h.t.calls).not.toContain('commit')
  })

  it('keeps the user on the wizard and surfaces real delete failures', async () => {
    const user = userEvent.setup()
    h.t.failOn = 'delete'

    render(<IntroWizardViewer {...viewerProps} />)

    await advanceToFinish(user)

    await waitFor(() =>
      expect(h.toastError).toHaveBeenCalledWith('delete failed'),
    )
    expect(h.navigateToObjects).not.toHaveBeenCalled()
    expect(h.t.state.indexPath).toBe('wizard/welcome-1')
    expect(h.t.state.objects.has('wizard/welcome-1')).toBe(true)
  })

  it('skips the introduction through the same guarded cleanup path as Finish', async () => {
    const user = userEvent.setup()

    render(<IntroWizardViewer {...viewerProps} />)

    await user.click(screen.getByRole('button', { name: /skip/i }))

    await expectFinished()
    expect(h.handleCancel).not.toHaveBeenCalled()
    expect(h.toastError).not.toHaveBeenCalled()
  })

  it('renders loading while wizard state is unavailable', () => {
    h.hasState = false

    render(<IntroWizardViewer {...viewerProps} />)

    expect(screen.getByText('Loading')).toBeTruthy()
  })
})
