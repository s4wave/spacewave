import { useCallback, useReducer, useRef } from 'react'

import { SetSpaceIndexPathOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// WizardRef names a wizard object in a Space World with the Space index the
// caller last read. A useWizardState result satisfies it, and creation code
// that runs outside React builds one from its request.
export interface WizardRef {
  spaceWorld: EngineWorldState
  objectKey: string
  sessionPeerId: string
  spaceSettings?: { indexPath?: string }
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
    await tx.commit(signal)
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
      if (!deleted && m.mustExist) throw new Error(wizardGoneMessage)
      return
    }
    case 'write':
      await m.run(tx)
      return
  }
}

// wizardGoneMessage explains a Finish refused because the wizard is gone.
export const wizardGoneMessage =
  'This wizard was already finished or deleted elsewhere'

// wizardRemoval lists the writes that remove a wizard: a Space index that
// names it moves to indexPath, or is cleared when indexPath is empty, then
// the wizard is deleted. The index op expects the index the caller read, so
// the World refuses it, without error, when another writer changed it since.
// A Finish that creates sets mustExist, so a wizard another tab finished or
// cancelled refuses the whole list rather than creating a second object.
export function wizardRemoval(
  wizard: WizardRef,
  indexPath: string,
  mustExist: boolean,
): WizardMutation[] {
  // Move the index only when the index read names this wizard.
  const current = wizard.spaceSettings?.indexPath ?? ''
  const removal: WizardMutation[] = []
  if (parseObjectUri(current).objectKey === wizard.objectKey) {
    removal.push({
      kind: 'op',
      opId: SET_SPACE_INDEX_PATH_OP_ID,
      opData: SetSpaceIndexPathOp.toBinary({
        indexPath,
        expectedIndexPath: current,
        timestamp: new Date(),
      }),
    })
  }
  removal.push({ kind: 'delete', objectKey: wizard.objectKey, mustExist })
  return removal
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
// idle offers Finish; finishing applies the list; failed says what failed
// and offers Retry, which prepares and applies the whole list again; done
// means the wizard is gone and its target opens.
export type WizardFinishState =
  | { status: 'idle' }
  | { status: 'finishing' }
  | { status: 'failed'; error: string }
  | { status: 'done' }

// WizardFinishEvent moves the Finish control between its states.
type WizardFinishEvent =
  | { type: 'start' }
  | { type: 'fail'; error: string }
  | { type: 'finish' }

// wizardFinishReducer applies an event to the Finish control's state.
export function wizardFinishReducer(
  _state: WizardFinishState,
  event: WizardFinishEvent,
): WizardFinishState {
  switch (event.type) {
    case 'start':
      return { status: 'finishing' }
    case 'fail':
      return { status: 'failed', error: event.error }
    case 'finish':
      return { status: 'done' }
  }
}

// UseWizardFinishResult is the Finish control a wizard renders.
export interface UseWizardFinishResult {
  state: WizardFinishState
  // finish runs a press, Finish from idle or Retry from failed: prepare
  // reads what the writes need and builds the plan, then the plan applies
  // as one list. onDone runs once the wizard is gone. It resolves with the
  // state the press ended in, or undefined when a press was already running.
  finish: (
    prepare: () => Promise<WizardFinishPlan>,
    onDone: (target: WizardFinishTarget) => void,
  ) => Promise<WizardFinishState | undefined>
}

// useWizardFinish holds a wizard's Finish control.
export function useWizardFinish(wizard: WizardRef): UseWizardFinishResult {
  const [state, dispatch] = useReducer(wizardFinishReducer, {
    status: 'idle',
  })
  const runningRef = useRef(false)

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
        onDone(plan.target)
        return { status: 'done' }
      } catch (err) {
        const error = err instanceof Error ? err.message : String(err)
        dispatch({ type: 'fail', error })
        return { status: 'failed', error }
      } finally {
        runningRef.current = false
      }
    },
    [wizard],
  )

  return { state, finish }
}
