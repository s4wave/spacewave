import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { CREATE_WASIVM_OP_ID } from '@s4wave/sdk/vm/wasivm.js'
import {
  wizardTestWorld,
  type WizardTestWorld,
} from '@s4wave/app/wizard/wizard-test-world.js'

const h = vi.hoisted(() => ({
  t: undefined as unknown as WizardTestWorld,
  navigateToObjects: vi.fn(),
  rootResource: { value: undefined },
}))

vi.mock('@s4wave/app/wizard/useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/wasivm-1',
    state: { step: 0 },
    localName: 'Shell',
    sessionPeerId: 'peer-1',
    spaceWorld: h.t.world,
    spaceSettings: { indexPath: 'wizard/wasivm-1' },
    navigateToObjects: h.navigateToObjects,
    persistDraftState: vi.fn().mockResolvedValue(undefined),
    handleUpdateName: vi.fn(),
    handleBack: vi.fn(),
    handleCancel: vi.fn(),
  }),
}))

// The CDN lists no images; the Space has one filesystem to boot.
vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => h.rootResource,
}))
vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: (resource: unknown) => ({
    value: resource === h.rootResource ? [] : ['fs/root'],
  }),
}))

vi.mock('@s4wave/app/space/create-op-builders.js', () => ({
  buildObjectKey: () => Promise.resolve('vm/wasivm/shell'),
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import WasiVmWizard from './WasiVmWizard.js'

describe('WasiVmWizard', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('replaces the wizard with the created VM in the Space index', async () => {
    // The index names the wizard.
    h.t = wizardTestWorld(['wizard/wasivm-1'], 'wizard/wasivm-1')

    // The wizard opens on its only step.
    render(
      <WasiVmWizard
        {...({ objectInfo: {}, worldState: {} } as ObjectViewerComponentProps)}
      />,
    )

    // Finish creates the VM.
    fireEvent.click(screen.getByRole('button', { name: /create/i }))

    // The VM, the index move and the wizard delete committed together.
    await waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith(['vm/wasivm/shell']),
    )
    expect(h.t.state).toEqual({
      objects: new Set(),
      indexPath: 'vm/wasivm/shell',
      creates: [CREATE_WASIVM_OP_ID],
    })
  })
})
