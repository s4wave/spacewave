import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'

const h = vi.hoisted(() => ({
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
  navigateToObjects: vi.fn(),
  rootResource: { value: undefined },
}))

vi.mock('@s4wave/app/wizard/useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/wasivm-1',
    state: { step: 0 },
    localName: 'Shell',
    creating: false,
    setCreating: vi.fn(),
    sessionPeerId: 'peer-1',
    spaceWorld: { applyWorldOp: h.applyWorldOp, deleteObject: h.deleteObject },
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

// deleteWizard is spied on, passing through, to check Finish hands it the
// wizard and the VM that replaces it in the Space index.
vi.mock('@s4wave/app/wizard/wizard-space-index.js', async (importOriginal) => {
  const mod =
    await importOriginal<
      typeof import('@s4wave/app/wizard/wizard-space-index.js')
    >()
  return { ...mod, deleteWizard: vi.fn(mod.deleteWizard) }
})

import { deleteWizard } from '@s4wave/app/wizard/wizard-space-index.js'
import WasiVmWizard from './WasiVmWizard.js'

describe('WasiVmWizard', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('replaces the wizard with the created VM in the Space index', async () => {
    // The wizard opens on its only step.
    render(
      <WasiVmWizard
        {...({ objectInfo: {}, worldState: {} } as ObjectViewerComponentProps)}
      />,
    )

    // Finish creates the VM.
    fireEvent.click(screen.getByRole('button', { name: /create/i }))

    // The wizard is deleted through deleteWizard, naming the VM as the index.
    await waitFor(() =>
      expect(h.deleteObject).toHaveBeenCalledWith('wizard/wasivm-1'),
    )
    expect(deleteWizard).toHaveBeenCalledWith(
      expect.objectContaining({ objectKey: 'wizard/wasivm-1' }),
      'vm/wasivm/shell',
    )
    expect(h.navigateToObjects).toHaveBeenCalledWith(['vm/wasivm/shell'])
  })
})
