import { describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'

import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'

import {
  cancelWizard,
  finishWizard,
  useWizardFinish,
  WizardGoneError,
  WizardInputError,
  type WizardRef,
} from './wizard-finish.js'
import { wizardTestWorld } from './wizard-test-world.js'

const createOpId = 'test/create'

// wizardIn builds a ref to wizard/one in a test World. The ref carries no
// read of the Space index: the World decides the index move.
function wizardIn(t: ReturnType<typeof wizardTestWorld>): WizardRef {
  return {
    spaceWorld: t.world,
    objectKey: 'wizard/one',
    sessionPeerId: 'peer-1',
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
    await expect(finishWizard(wizardIn(t), plan)).rejects.toThrow(
      WizardGoneError,
    )

    // No second object, and the other tab's index stands.
    expect(t.state.creates).toEqual([])
    expect(t.state.indexPath).toBe('obj/other')
  })

  it('moves an index that names the wizard though no settings were read', async () => {
    // The settings watch had not loaded, or lagged a write that pointed the
    // index at the wizard: the World still names the wizard when the list
    // applies, so the index moves to the target.
    const t = wizardTestWorld(['wizard/one'], 'canvas/main')
    t.state.indexPath = 'wizard/one'
    await finishWizard(wizardIn(t), plan)

    expect(t.state.indexPath).toBe('obj/one')
    expect(t.state.objects.has('wizard/one')).toBe(false)
  })

  it('reports a Finish whose commit was sent before an abort as done', async () => {
    // The caller aborts while the commit is in flight; the commit lands.
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    let open = () => {}
    t.commitGate = new Promise((resolve) => (open = resolve))
    const abort = new AbortController()
    const finishing = finishWizard(wizardIn(t), plan, abort.signal)
    await vi.waitFor(() => expect(t.calls).toContain('delete'))
    abort.abort()
    open()

    // The outcome is the commit's: it landed, so the Finish resolves.
    await expect(finishing).resolves.toBeUndefined()
    expect(t.calls).not.toContain('discard')
    expect(t.state.creates).toEqual([createOpId])
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

  it('refuses an Intro Finish after a Cancel removed the wizard', async () => {
    // Tab A cancels: the index naming the wizard is cleared, the wizard goes.
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    await cancelWizard(wizardIn(t), '')
    expect(t.state.indexPath).toBe('')

    // Tab B's Intro Finish creates nothing and is refused, not reported done.
    const intro = { creates: [], target: { openKey: 'obj/x', indexPath: 'x' } }
    await expect(finishWizard(wizardIn(t), intro)).rejects.toThrow(
      WizardGoneError,
    )
    expect(t.state.indexPath).toBe('')
  })

  it('refuses a Cancel after a Finish removed the wizard', async () => {
    // Another tab's Finish landed first: its object exists, the wizard is
    // gone, and the index names the object.
    const t = wizardTestWorld(['obj/other'], 'obj/other')
    await expect(cancelWizard(wizardIn(t), '')).rejects.toThrow(WizardGoneError)

    // Nothing committed, so Cancel cannot report success beside Finish.
    expect(t.calls).not.toContain('commit')
    expect(t.calls).toContain('discard')
    expect(t.state.indexPath).toBe('obj/other')
    expect(t.state.objects.has('obj/other')).toBe(true)
  })
})

describe('useWizardFinish', () => {
  it('returns to idle with the message when prepare refuses the input', async () => {
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    const { result } = renderHook(() => useWizardFinish(wizardIn(t)))

    // Nothing was attempted: the press offers Finish again, not Retry.
    await act(() =>
      result.current.finish(() => {
        throw new WizardInputError('Enter a name')
      }, vi.fn()),
    )
    expect(result.current.state).toEqual({
      status: 'idle',
      message: 'Enter a name',
    })
    expect(t.calls).toEqual([])
  })

  it('shows gone for a refused Finish, but done for a target made before it', async () => {
    const t = wizardTestWorld(['obj/other'], 'obj/other')
    const intro = { creates: [], target: { openKey: 'obj/x', indexPath: 'x' } }
    const { result } = renderHook(() => useWizardFinish(wizardIn(t)))

    const onDone = vi.fn()
    await act(() => result.current.finish(() => Promise.resolve(intro), onDone))
    expect(result.current.state).toEqual({ status: 'gone' })
    expect(onDone).not.toHaveBeenCalled()

    // A paired device exists apart from the list, so it opens.
    const paired = { ...intro, targetMade: true }
    await act(() =>
      result.current.finish(() => Promise.resolve(paired), onDone),
    )
    expect(result.current.state).toEqual({ status: 'done' })
    expect(onDone).toHaveBeenCalledWith(intro.target)
    expect(t.calls).not.toContain('commit')
  })

  it('commits but does not navigate once the wizard unmounted', async () => {
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    let open = () => {}
    t.commitGate = new Promise((resolve) => (open = resolve))
    const onDone = vi.fn()
    const { result, unmount } = renderHook(() => useWizardFinish(wizardIn(t)))

    // The view closes while the commit is in flight.
    const pressed = result.current.finish(() => Promise.resolve(plan), onDone)
    await vi.waitFor(() => expect(t.calls).toContain('delete'))
    unmount()
    open()

    // The Finish landed, and nothing navigates from the closed view.
    await expect(pressed).resolves.toEqual({ status: 'done' })
    expect(t.state.creates).toEqual([createOpId])
    expect(onDone).not.toHaveBeenCalled()
  })
})
