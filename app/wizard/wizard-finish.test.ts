import { describe, expect, it } from 'vitest'

import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'

import {
  cancelWizard,
  finishWizard,
  wizardGoneMessage,
  type WizardRef,
} from './wizard-finish.js'
import { wizardTestWorld } from './wizard-test-world.js'

const createOpId = 'test/create'

// wizardIn builds a ref to wizard/one in a test World whose index names it.
function wizardIn(t: ReturnType<typeof wizardTestWorld>): WizardRef {
  return {
    spaceWorld: t.world,
    objectKey: 'wizard/one',
    sessionPeerId: 'peer-1',
    spaceSettings: { indexPath: 'wizard/one' },
  }
}

// plan is a Finish that creates one object, obj/one.
const plan = {
  creates: [
    { kind: 'op' as const, opId: createOpId, opData: new Uint8Array() },
  ],
  target: { openKey: 'obj/one', indexPath: 'obj/one' },
}

describe('finishWizard', () => {
  it('creates, moves the index and deletes the wizard in one commit', async () => {
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    await finishWizard(wizardIn(t), plan)

    // Every write ran inside one transaction that committed.
    expect(t.calls).toEqual([
      'open',
      createOpId,
      SET_SPACE_INDEX_PATH_OP_ID,
      'delete',
      'commit',
      'release',
    ])
    expect(t.state).toEqual({
      objects: new Set(),
      indexPath: 'obj/one',
      creates: [createOpId],
    })
  })

  it.each([createOpId, SET_SPACE_INDEX_PATH_OP_ID, 'delete', 'commit'])(
    'leaves no object, the index and the wizard when %s throws',
    async (failOn) => {
      // The chosen call throws and the Finish rejects with it.
      const t = wizardTestWorld(['wizard/one'], 'wizard/one')
      t.failOn = failOn
      await expect(finishWizard(wizardIn(t), plan)).rejects.toThrow(failOn)

      // The transaction was discarded, so nothing it staged was published.
      expect(t.calls).toContain('discard')
      expect(t.calls.at(-1)).toBe('release')
      expect(t.state).toEqual({
        objects: new Set(['wizard/one']),
        indexPath: 'wizard/one',
        creates: [],
      })
    },
  )

  it('refuses the whole Finish when another tab already removed the wizard', async () => {
    // Another tab finished: its object exists and the wizard is gone.
    const t = wizardTestWorld(['obj/other'], 'obj/other')
    const stale = { ...wizardIn(t), spaceSettings: { indexPath: 'wizard/one' } }
    await expect(finishWizard(stale, plan)).rejects.toThrow(wizardGoneMessage)

    // No second object, and the other tab's index stands.
    expect(t.state.creates).toEqual([])
    expect(t.state.indexPath).toBe('obj/other')
  })

  it('leaves an index another writer moved', async () => {
    // The person pointed the index elsewhere after the wizard read it.
    const t = wizardTestWorld(['wizard/one'], 'canvas/main')
    await finishWizard(wizardIn(t), plan)

    expect(t.state.indexPath).toBe('canvas/main')
    expect(t.state.objects.has('wizard/one')).toBe(false)
  })
})

describe('cancelWizard', () => {
  it('clears an index naming the wizard and deletes it', async () => {
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    await cancelWizard(wizardIn(t), '')

    expect(t.state).toEqual({ objects: new Set(), indexPath: '', creates: [] })
  })

  it('counts a wizard already deleted as cancelled', async () => {
    const t = wizardTestWorld([], '')
    await cancelWizard({ ...wizardIn(t), spaceSettings: {} }, '')

    expect(t.calls).toContain('commit')
  })
})
