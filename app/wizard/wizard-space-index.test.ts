import { afterEach, describe, expect, it, vi } from 'vitest'

import { SetSpaceIndexPathOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'

import { deleteWizard, type WizardRef } from './wizard-space-index.js'

const applyWorldOp = vi.fn()
const deleteObject = vi.fn()

// wizardAt builds a wizard ref whose Space index the caller read as indexPath.
function wizardAt(indexPath: string): WizardRef {
  return {
    spaceWorld: { applyWorldOp, deleteObject } as never,
    objectKey: 'wizard/vm-1',
    sessionPeerId: 'peer-1',
    spaceSettings: { indexPath },
  }
}

// indexOp decodes the one Space index op deleteWizard applied.
function indexOp() {
  // Exactly one op was applied, by the wizard's session peer.
  expect(applyWorldOp).toHaveBeenCalledTimes(1)
  const [opId, opData, sender] = applyWorldOp.mock.calls[0] as [
    string,
    Uint8Array,
    string,
  ]
  expect(opId).toBe(SET_SPACE_INDEX_PATH_OP_ID)
  expect(sender).toBe('peer-1')
  return SetSpaceIndexPathOp.fromBinary(opData)
}

describe('deleteWizard', () => {
  afterEach(() => {
    applyWorldOp.mockReset()
    deleteObject.mockReset()
  })

  it('moves an index naming the wizard, guarded on the read path, before deleting', async () => {
    // Both writes succeed.
    applyWorldOp.mockResolvedValue({ seqno: 1n, sysErr: false })
    deleteObject.mockResolvedValue({ deleted: true })

    // The index the caller read names the wizard.
    await deleteWizard(wizardAt('wizard/vm-1'), 'vm/v86/one')

    // The op names the replacement and expects the index the caller read.
    const op = indexOp()
    expect(op.indexPath).toBe('vm/v86/one')
    expect(op.expectedIndexPath).toBe('wizard/vm-1')

    // The index write lands before the delete.
    expect(deleteObject).toHaveBeenCalledWith('wizard/vm-1')
    expect(applyWorldOp.mock.invocationCallOrder[0]).toBeLessThan(
      deleteObject.mock.invocationCallOrder[0] ?? 0,
    )
  })

  it('leaves an index naming another object alone and only deletes', async () => {
    deleteObject.mockResolvedValue({ deleted: true })

    await deleteWizard(wizardAt('canvas/main'), 'vm/v86/one')

    expect(applyWorldOp).not.toHaveBeenCalled()
    expect(deleteObject).toHaveBeenCalledWith('wizard/vm-1')
  })

  it('keeps the wizard when the index write throws', async () => {
    applyWorldOp.mockRejectedValue(new Error('world closed'))

    await expect(
      deleteWizard(wizardAt('wizard/vm-1'), 'vm/v86/one'),
    ).rejects.toThrow('world closed')

    // The index still names the wizard, so the wizard must still exist.
    expect(deleteObject).not.toHaveBeenCalled()
  })

  it('counts a wizard another participant deleted as deleted, and rethrows other failures', async () => {
    // Another participant already deleted the wizard.
    applyWorldOp.mockResolvedValue({ seqno: 1n, sysErr: false })
    deleteObject.mockRejectedValueOnce(new Error('object not found'))
    await deleteWizard(wizardAt('wizard/vm-1'), 'vm/v86/one')

    // Any other delete failure reaches the caller.
    deleteObject.mockRejectedValueOnce(new Error('disk full'))
    await expect(
      deleteWizard(wizardAt('wizard/vm-1'), 'vm/v86/one'),
    ).rejects.toThrow('disk full')
  })
})
