import { useCallback, useEffect, useReducer, useRef } from 'react'

import { SetSpaceIndexPathOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// WizardRef names a wizard object in a Space World and the peer that writes
// as the session. A useWizardState result satisfies it, and creation code
// that runs outside React builds one from its request.
export interface WizardRef {
  spaceWorld: EngineWorldState
  objectKey: string
  sessionPeerId: string
}

// WizardMutation is one write a wizard's Finish or Cancel applies. op applies
// a World operation as the session peer; delete removes an object, and fails
// the whole list when the object is gone and mustExist is set. write runs a
// writer that needs a World to call, for seeders that read back what they
// wrote; it is the one entry that is not plain data.
export type WizardMutation =
  | { kind: 'op'; opId: string; opData: Uint8Array }
  | { kind: 'delete'; objectKey: string; mustExist: boolean }
  | { kind: 'write'; run: (world: IWorldState) => Promise<void> }

// applyWizardMutations applies the list in one write transaction and commits
// it, or discards it on a throw, so either every write lands or none does.
// The transaction holds the World's writer from open to commit, so the list
// carries everything prepared: no entry reads the network, a CDN or the
// person. It is the one place that touches the transaction.
//
// signal cancels the list until the commit starts, checked once more just
// before it, so an abort before the commit lands nothing. The commit itself
// does not take it: once it is sent, its result is the outcome, so an abort
// that races the commit can never report a Finish that landed as failed.
export async function applyWizardMutations(
  world: EngineWorldState,
  sender: string,
  mutations: WizardMutation[],
  signal?: AbortSignal,
) {
  const tx = await world.getEngine().newTransaction(true, signal)
  let committed = false
  try {
    for (const m of mutations) {
      // eslint-disable-next-line react-doctor/async-await-in-loop -- the list applies in order: the index move before the delete.
      await applyWizardMutation(tx, sender, m, signal)
    }
    signal?.throwIfAborted()
    await tx.commit()
    committed = true
  } finally {
    // A discard that fails leaves the release to drop the transaction.
    if (!committed) await tx.discard().catch(() => {})
    tx.release()
  }
}

// applyWizardMutation applies one entry of the list on the transaction.
async function applyWizardMutation(
  tx: IWorldState,
  sender: string,
  m: WizardMutation,
  signal?: AbortSignal,
) {
  switch (m.kind) {
    case 'op':
      await tx.applyWorldOp(m.opId, m.opData, sender, signal)
      return
    case 'delete': {
      const { deleted } = await tx.deleteObject(m.objectKey, signal)
      if (!deleted && m.mustExist) throw new WizardGoneError()
      return
    }
    case 'write':
      await m.run(tx)
      return
  }
}

// WizardGoneError refuses a Finish because the wizard is already gone:
// another tab finished or cancelled it. Nothing was written, and Retry can
// never succeed, so the Finish control shows the gone state.
export class WizardGoneError extends Error {
  constructor() {
    super('This wizard no longer exists')
    this.name = 'WizardGoneError'
  }
}

// WizardInputError refuses a press because of what the person entered, such
// as an empty name. prepare throws it before anything is written, so the
// Finish control returns to idle with the message beside Finish.
export class WizardInputError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'WizardInputError'
  }
}

// wizardRemoval lists the writes that remove a wizard: a Space index that
// names it moves to indexPath, or is cleared when indexPath is empty, then
// the wizard is deleted. The index op always goes in the list and expects the
// wizard's key, so the World decides inside the transaction: it moves an
// index that names the wizard and leaves any other index, without error. No
// client read of the index is involved, so a settings watch that has not
// loaded or lags cannot leave the index naming a deleted wizard. A Finish
// that creates sets mustExist, so a wizard another tab finished or cancelled
// refuses the whole list rather than creating a second object.
export function wizardRemoval(
  wizard: WizardRef,
  indexPath: string,
  mustExist: boolean,
): WizardMutation[] {
  return [
    {
      kind: 'op',
      opId: SET_SPACE_INDEX_PATH_OP_ID,
      opData: SetSpaceIndexPathOp.toBinary({
        indexPath,
        expectedIndexPath: wizard.objectKey,
        timestamp: new Date(),
      }),
    },
    { kind: 'delete', objectKey: wizard.objectKey, mustExist },
  ]
}

// WizardFinishPlan is a prepared Finish: the writes that create the target,
// and the target itself. Everything the writes need is read before the plan
// is built.
export interface WizardFinishPlan {
  creates: WizardMutation[]
  target: WizardFinishTarget
}

// WizardFinishTarget is what a Finish made: the object to open, the path a
// Space index naming the wizard moves to, and the name a success notice uses.
export interface WizardFinishTarget {
  openKey: string
  indexPath: string
  name?: string
}

// finishWizard applies a prepared Finish as one list: the creates, the index
// move and the wizard delete. The wizard exists until the list commits and
// its target exists only once it has, so a failure leaves the wizard, the
// index and the person's draft as they were, and Retry runs it all again.
// A Finish that creates nothing, such as an introduction, succeeds when the
// wizard is already gone, since a second press duplicates nothing.
export async function finishWizard(
  wizard: WizardRef,
  plan: WizardFinishPlan,
  signal?: AbortSignal,
) {
  await applyWizardMutations(
    wizard.spaceWorld,
    wizard.sessionPeerId,
    [
      ...plan.creates,
      ...wizardRemoval(
        wizard,
        plan.target.indexPath,
        plan.creates.length !== 0,
      ),
    ],
    signal,
  )
}

// cancelWizard removes a wizard without creating anything: a Space index
// that names it moves to indexPath, or is cleared. A wizard already deleted
// elsewhere counts as cancelled.
export async function cancelWizard(
  wizard: WizardRef,
  indexPath: string,
  signal?: AbortSignal,
) {
  await applyWizardMutations(
    wizard.spaceWorld,
    wizard.sessionPeerId,
    wizardRemoval(wizard, indexPath, false),
    signal,
  )
}

// WizardFinishState is the Finish control's state, from WizardFinish.tla.
// idle offers Finish, with the message of a press whose input was refused
// before anything was written; finishing applies the list; failed says what
// failed and offers Retry, which prepares and applies the whole list again;
// done means this press removed the wizard and its target opens; gone means
// another writer removed the wizard, so the only way out is to close.
export type WizardFinishState =
  | { status: 'idle'; message?: string }
  | { status: 'finishing' }
  | { status: 'failed'; error: string }
  | { status: 'done' }
  | { status: 'gone' }

// WizardFinishEvent moves the Finish control between its states.
type WizardFinishEvent =
  | { type: 'start' }
  | { type: 'refuse'; state: WizardFinishState }
  | { type: 'finish' }

// wizardFinishReducer applies an event to the Finish control's state.
export function wizardFinishReducer(
  _state: WizardFinishState,
  event: WizardFinishEvent,
): WizardFinishState {
  switch (event.type) {
    case 'start':
      return { status: 'finishing' }
    case 'refuse':
      return event.state
    case 'finish':
      return { status: 'done' }
  }
}

// wizardFinishView is the state a wizard shows: the control's own state, or
// gone once the watched wizard object is removed by another writer. A press
// that is finishing or done removes the wizard itself, so the watch seeing it
// gone then is not a deletion to show.
export function wizardFinishView(
  state: WizardFinishState,
  wizardGone: boolean,
): WizardFinishState {
  if (!wizardGone) return state
  if (state.status === 'finishing' || state.status === 'done') return state
  return { status: 'gone' }
}

// refusedFinishState is the state a press that threw ends in: idle with the
// message for an input the person can fix, gone for a wizard another writer
// removed, and failed with Retry for anything else.
function refusedFinishState(err: unknown): WizardFinishState {
  if (err instanceof WizardInputError) {
    return { status: 'idle', message: err.message }
  }
  if (err instanceof WizardGoneError) return { status: 'gone' }
  return {
    status: 'failed',
    error: err instanceof Error ? err.message : String(err),
  }
}

// UseWizardFinishResult is the Finish control a wizard renders.
export interface UseWizardFinishResult {
  state: WizardFinishState
  // finish runs a press, Finish from idle or Retry from failed: prepare
  // checks the input, reads what the writes need and builds the plan, then
  // the plan applies as one list. A WizardInputError from prepare returns the
  // press to idle with its message. onDone runs once the wizard is
  // gone, unless the wizard unmounted while the press ran. It resolves with
  // the state the press ended in, or undefined when a press was already
  // running.
  finish: (
    prepare: () => Promise<WizardFinishPlan>,
    onDone: (target: WizardFinishTarget) => void,
  ) => Promise<WizardFinishState | undefined>
}

// useWizardFinish holds a wizard's Finish control. wizardGone is the watch on
// the wizard object, which a useWizardState result carries.
export function useWizardFinish(
  wizard: WizardRef & { wizardGone?: boolean },
): UseWizardFinishResult {
  // The control's own state, and whether a press is running.
  const [state, dispatch] = useReducer(wizardFinishReducer, {
    status: 'idle',
  })
  const runningRef = useRef(false)

  // mountedRef is false once the wizard unmounts: a press still running
  // finishes its transaction but does not navigate from a closed view.
  const mountedRef = useRef(false)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  // finish runs one press through prepare, the list and onDone.
  const finish = useCallback(
    async (
      prepare: () => Promise<WizardFinishPlan>,
      onDone: (target: WizardFinishTarget) => void,
    ): Promise<WizardFinishState | undefined> => {
      // One press runs at a time.
      if (runningRef.current) return undefined
      runningRef.current = true
      dispatch({ type: 'start' })
      try {
        const plan = await prepare()
        await finishWizard(wizard, plan)
        dispatch({ type: 'finish' })
        if (mountedRef.current) onDone(plan.target)
        return { status: 'done' }
      } catch (err) {
        const next = refusedFinishState(err)
        dispatch({ type: 'refuse', state: next })
        return next
      } finally {
        runningRef.current = false
      }
    },
    [wizard],
  )

  return { state: wizardFinishView(state, !!wizard.wizardGone), finish }
}
