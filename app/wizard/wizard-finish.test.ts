import { describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'

import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { RecordWizardEndingOpTypeID } from '@s4wave/sdk/world/wizard/wizard_typeid.pb.js'
import { WorldOperationRejection } from '@s4wave/sdk/world/errors.js'
import { WorldErrorCode } from '@s4wave/sdk/world/world.pb.js'

import {
  applyWizardMutations,
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

// madePlan is a Finish that creates the object at key.
function madePlan(key: string) {
  return {
    creates: [
      {
        kind: 'op' as const,
        opId: createOpId,
        opData: new TextEncoder().encode(key),
      },
    ],
    target: { openKey: key, indexPath: key },
  }
}

describe('finishWizard', () => {
  it('creates, records the ending, moves the index and deletes the wizard in one commit', async () => {
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    await finishWizard(wizardIn(t), madePlan('obj/one'))

    // Every write ran inside one batch that committed.
    expect(t.calls).toEqual([
      'batch',
      createOpId,
      RecordWizardEndingOpTypeID,
      SET_SPACE_INDEX_PATH_OP_ID,
      'delete',
      'commit',
    ])
    expect(t.state).toEqual({
      objects: new Set(['obj/one']),
      indexPath: 'obj/one',
      creates: [createOpId],
      endings: new Map([['wizard/one', 'obj/one']]),
    })
  })

  it.each([
    createOpId,
    RecordWizardEndingOpTypeID,
    SET_SPACE_INDEX_PATH_OP_ID,
    'delete',
    'commit',
  ])(
    'leaves no object, the index and the wizard when %s throws',
    async (failOn) => {
      // The chosen call throws and the Finish rejects with it.
      const t = wizardTestWorld(['wizard/one'], 'wizard/one')
      t.failOn = failOn
      await expect(finishWizard(wizardIn(t), plan)).rejects.toThrow(failOn)

      // The batch failed, so nothing it staged was published.
      expect(t.state).toEqual({
        objects: new Set(['wizard/one']),
        indexPath: 'wizard/one',
        creates: [],
        endings: new Map(),
      })
    },
  )

  it('refuses the whole Finish when another tab already removed the wizard', async () => {
    // Another tab finished: its object exists and the wizard is gone.
    const t = wizardTestWorld(['obj/other'], 'obj/other')
    await expect(finishWizard(wizardIn(t), plan)).rejects.toThrow(
      WizardGoneError,
    )

    // No second object is created, and the other tab's index stands.
    expect(t.state.creates).toEqual([])
    expect(t.state.indexPath).toBe('obj/other')
  })

  it('refuses a create linked from a wizard another tab removed', async () => {
    // The wizard is gone, so the link from the wizard and the create both fail.
    const t = wizardTestWorld(['obj/other'], 'obj/other')
    const created = [
      { kind: 'op' as const, opId: createOpId, opData: new Uint8Array() },
      {
        kind: 'link' as const,
        subjectKey: 'wizard/one',
        predicate: '<wizard/made>',
        objectKey: 'obj/other',
      },
    ]
    await expect(
      applyWizardMutations(t.world, 'peer-1', created),
    ).rejects.toThrow(WizardGoneError)
    expect(t.state.creates).toEqual([])
  })

  it('moves an index that names the wizard though no settings were read', async () => {
    // The settings watch had not loaded, or lagged a write that pointed the
    // index at the wizard: the World's index still points to the wizard when
    // the list applies, so the index moves to the target.
    const t = wizardTestWorld(['wizard/one'], 'canvas/main')
    t.state.indexPath = 'wizard/one'
    await finishWizard(wizardIn(t), plan)

    expect(t.state.indexPath).toBe('obj/one')
    expect(t.state.objects.has('wizard/one')).toBe(false)
  })

  it('reports a Finish whose commit was sent before an abort as done', async () => {
    // The caller aborts while the commit is in flight, but the commit lands.
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    let open = () => {}
    t.commitGate = new Promise((resolve) => (open = resolve))
    const abort = new AbortController()
    const finishing = finishWizard(wizardIn(t), plan, abort.signal)
    await vi.waitFor(() => expect(t.calls).toContain('delete'))
    abort.abort()
    open()

    // The outcome is the batch's: it landed, so the Finish resolves.
    await expect(finishing).resolves.toBeUndefined()
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

    expect(t.state).toEqual({
      objects: new Set(),
      indexPath: '',
      creates: [],
      endings: new Map(),
    })
  })

  it('refuses an Intro Finish after a Cancel removed the wizard', async () => {
    // Tab A cancels: the index pointing to the wizard is cleared, and the
    // wizard goes.
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
    // gone, and the index points to the object.
    const t = wizardTestWorld(['obj/other'], 'obj/other')
    await expect(cancelWizard(wizardIn(t), '')).rejects.toThrow(WizardGoneError)

    // Nothing committed, so Cancel cannot report success beside Finish.
    expect(t.calls).not.toContain('commit')
    expect(t.state.indexPath).toBe('obj/other')
    expect(t.state.objects.has('obj/other')).toBe(true)
  })
})

describe('useWizardFinish', () => {
  it('returns to idle with the message when prepare refuses the input', async () => {
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    const { result } = renderHook(() => useWizardFinish(wizardIn(t)))

    // Nothing was tried: the press offers Finish again, not Retry.
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

  it('returns to idle with the reason when the operation rejects the input', async () => {
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    const { result } = renderHook(() => useWizardFinish(wizardIn(t)))

    // The same list fails the same way, so the press offers Finish, not Retry.
    await act(() =>
      result.current.finish(() => {
        throw new WorldOperationRejection('test/taken', 'The name is taken')
      }, vi.fn()),
    )
    expect(result.current.state).toEqual({
      status: 'idle',
      message: 'The name is taken',
    })
  })

  it('ends unsupported when the plugin has no build for this device', async () => {
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    t.refuseOn = {
      opId: createOpId,
      code: WorldErrorCode.PLUGIN_EXECUTABLE_MISSING,
    }
    const onDone = vi.fn()
    const { result } = renderHook(() => useWizardFinish(wizardIn(t)))
    await act(() =>
      result.current.finish(() => Promise.resolve(madePlan('obj/one')), onDone),
    )
    expect(result.current.state).toEqual({ status: 'unsupported' })
    expect(onDone).not.toHaveBeenCalled()
    expect(t.state.objects).toEqual(new Set(['wizard/one']))
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

  it('ends done when Retry finds the wizard removed by its own lost reply', async () => {
    // The list commits but its reply is lost, so the press shows failed.
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    const made = madePlan('obj/one')
    t.loseReply = true
    const onDone = vi.fn()
    const { result } = renderHook(() => useWizardFinish(wizardIn(t)))
    await act(() => result.current.finish(() => Promise.resolve(made), onDone))
    expect(result.current.state).toEqual({ status: 'done' })
    expect(onDone).toHaveBeenCalledWith(made.target)
    expect(t.state.objects).toEqual(new Set(['obj/one']))
  })

  it('ends done when an Intro Finish that creates nothing lost its reply', async () => {
    // Intro opens an object that already exists, so only its ending says the
    // Finish landed rather than another tab's Cancel.
    const t = wizardTestWorld(['wizard/one', 'obj/x'], 'wizard/one')
    const intro = { creates: [], target: { openKey: 'obj/x', indexPath: 'x' } }
    t.loseReply = true
    const onDone = vi.fn()
    const { result } = renderHook(() => useWizardFinish(wizardIn(t)))
    await act(() => result.current.finish(() => Promise.resolve(intro), onDone))
    expect(result.current.state).toEqual({ status: 'done' })
    expect(onDone).toHaveBeenCalledWith(intro.target)
  })

  it('shows the gone screen when the ending could not be read', async () => {
    // The reply is lost and so is the ending read, so the press shows failed.
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    const world = t.world as unknown as { lookupGraphQuads: unknown }
    world.lookupGraphQuads = vi.fn().mockRejectedValue(new Error('offline'))
    t.loseReply = true
    const onDone = vi.fn()
    const { result, rerender } = renderHook(
      ({ gone }) => useWizardFinish({ ...wizardIn(t), wizardGone: gone }),
      { initialProps: { gone: false } },
    )
    await act(() =>
      result.current.finish(() => Promise.resolve(madePlan('obj/one')), onDone),
    )
    expect(result.current.state.status).toBe('failed')

    // The watch sees the wizard gone: the gone screen reads the ending.
    rerender({ gone: true })
    expect(result.current.state).toEqual({ status: 'gone' })
    expect(onDone).not.toHaveBeenCalled()
  })

  it('shows gone when another tab removed the wizard after a failed press', async () => {
    const t = wizardTestWorld(['wizard/one'], 'wizard/one')
    t.failOn = 'commit'
    const onDone = vi.fn()
    const { result, rerender } = renderHook(
      ({ gone }) => useWizardFinish({ ...wizardIn(t), wizardGone: gone }),
      { initialProps: { gone: false } },
    )
    await act(() =>
      result.current.finish(() => Promise.resolve(madePlan('obj/one')), onDone),
    )

    // Another tab cancels: no target, so the wizard was removed elsewhere.
    t.state.objects.delete('wizard/one')
    rerender({ gone: true })
    await vi.waitFor(() =>
      expect(result.current.state).toEqual({ status: 'gone' }),
    )
    expect(onDone).not.toHaveBeenCalled()
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
