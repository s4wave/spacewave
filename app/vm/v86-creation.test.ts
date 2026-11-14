import { describe, expect, it } from 'vitest'

import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { CREATE_VM_V86_OP_ID } from '@s4wave/sdk/vm/vmv86.js'

import { wizardTestWorld } from '../wizard/wizard-test-world.js'
import { runVmCreation, type VmCreationRequest } from './v86-creation.js'

const request: VmCreationRequest = {
  linkCdnImage: false,
  cdnId: '',
  cdnSourceObjectKey: '',
  imageObjectKey: 'vm/image/debian',
  vmKey: 'vm/v86/debian',
  vmName: 'Debian',
  memoryMb: 256,
  vgaMemoryMb: 8,
  networking: false,
  sessionPeerId: 'peer-1',
  wizardObjectKey: 'wizard/vm-1',
}

// drain runs the creation stream to its end and returns the stages.
async function drain(t: ReturnType<typeof wizardTestWorld>) {
  const stages: string[] = []
  const signal = new AbortController().signal
  for await (const p of runVmCreation(t.world, request, signal)) {
    stages.push(p.stage)
  }
  return stages
}

describe('runVmCreation', () => {
  it('creates the VM, moves the index and deletes the wizard in one transaction', async () => {
    const t = wizardTestWorld(['wizard/vm-1'], 'wizard/vm-1')
    expect(await drain(t)).toEqual(['creating', 'ready'])

    // The VM create, the index move and the delete commit together.
    expect(t.calls).toEqual([
      'open',
      CREATE_VM_V86_OP_ID,
      SET_SPACE_INDEX_PATH_OP_ID,
      'delete',
      'commit',
      'release',
    ])
    expect(t.state).toEqual({
      objects: new Set(),
      indexPath: 'vm/v86/debian',
      creates: [CREATE_VM_V86_OP_ID],
    })
  })

  it('writes nothing when the wizard delete fails, so a retry with the same key creates once', async () => {
    // The delete throws after the create staged.
    const t = wizardTestWorld(['wizard/vm-1'], 'wizard/vm-1')
    t.failOn = 'delete'
    await expect(drain(t)).rejects.toThrow('delete failed')

    // No VM, the index still names the wizard, and the wizard is there.
    expect(t.state).toEqual({
      objects: new Set(['wizard/vm-1']),
      indexPath: 'wizard/vm-1',
      creates: [],
    })

    // The retry reruns the stream with the same request and creates one VM.
    t.failOn = undefined
    expect(await drain(t)).toEqual(['creating', 'ready'])
    expect(t.state.creates).toEqual([CREATE_VM_V86_OP_ID])
  })
})
