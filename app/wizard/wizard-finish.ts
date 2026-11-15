import { useCallback, useEffect, useReducer, useRef } from 'react'

import { SetSpaceIndexPathOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import {
  PluginExecutableMissingError,
  throwBatchError,
  WorldOperationRejection,
} from '@s4wave/sdk/world/errors.js'
import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import { RecordWizardEndingOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { lookupWizardEnding } from '@s4wave/sdk/world/wizard/wizard-ending.js'
import { RecordWizardEndingOpTypeID } from '@s4wave/sdk/world/wizard/wizard_typeid.pb.js'
import {
  type WorldMutation,
  WorldErrorCode,
} from '@s4wave/sdk/world/world.pb.js'

// WizardRef names a wizard object in a Space World and the peer that writes
// as the session. A useWizardState result satisfies it, and creation code
// that runs outside React builds one from its request.
export interface WizardRef {
  spaceWorld: EngineWorldState
  objectKey: string
  sessionPeerId: string
}

// WizardMutation is one write a wizard's Finish or Cancel applies. op applies
// a World operation as the session peer; link sets the graph quad from one
// object to another under predicate, an IRI; delete removes an object and its
// quads, and fails the whole list with WizardGoneError when the object is
// gone. Each is plain data, so the list crosses to the World as one request.
export type WizardMutation =
  | { kind: 'op'; opId: string; opData: Uint8Array }
  | { kind: 'link'; subjectKey: string; predicate: string; objectKey: string }
  | { kind: 'delete'; objectKey: string }

// applyWizardMutations applies the list as one World batch: the World opens
// one write transaction, applies each entry and commits, or writes nothing
// when an entry is refused or fails. The World's writer is held only inside
// that call, never across a round trip to this tab.
//
// A link from a wizard commits with the create it records, so a reload or
// another tab finds what the wizard made, and a wizard already gone refuses
// the create with WizardGoneError.
//
// signal cancels the list before it is sent, so an abort before then lands
// nothing. The batch itself does not take it: once it is sent, its result is
// the outcome, so an abort can never report a Finish that landed as failed.
export async function applyWizardMutations(
  world: EngineWorldState,
  sender: string,
  mutations: WizardMutation[],
  signal?: AbortSignal,
) {
  signal?.throwIfAborted()
  const response = await world
    .getEngine()
    .executeWorldBatch(mutations.map((m) => wizardWorldMutation(sender, m)))
  // A delete or link refused for a missing object names the wizard: a list
  // links only from the wizard to an object it creates earlier in the list.
  const refused = mutations[response.failedIndex ?? 0]
  if (
    response.errorCode === WorldErrorCode.OBJECT_NOT_FOUND &&
    (refused?.kind === 'delete' || refused?.kind === 'link')
  ) {
    throw new WizardGoneError()
  }
  throwBatchError(response)
}

// wizardWorldMutation is the World mutation for one entry of the list.
function wizardWorldMutation(sender: string, m: WizardMutation): WorldMutation {
  switch (m.kind) {
    case 'op':
      return {
        mutation: {
          case: 'op',
          value: { opTypeId: m.opId, opData: m.opData, opSender: sender },
        },
      }
    case 'link':
      return {
        mutation: {
          case: 'setGraphQuad',
          value: {
            quad: {
              subject: keyToIRI(m.subjectKey),
              predicate: m.predicate,
              obj: keyToIRI(m.objectKey),
            },
          },
        },
      }
    case 'delete':
      return {
        mutation: { case: 'deleteObject', value: { objectKey: m.objectKey } },
      }
  }
}

// WizardGoneError refuses a Finish or a Cancel because the wizard is already
// gone: another tab finished or cancelled it. Nothing was written, and a new
// press can never succeed, so the control shows the gone state.
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
// loaded or lags cannot leave the index naming a deleted wizard. The delete
// expects the wizard, so a wizard another tab finished or cancelled refuses
// the whole list rather than creating a second object or reporting a second
// ending, even for a Finish that creates nothing.
function wizardRemoval(wizard: WizardRef, indexPath: string): WizardMutation[] {
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
    { kind: 'delete', objectKey: wizard.objectKey },
  ]
}

// WizardFinishPlan is a prepared Finish: the writes that create the target,
// and the target itself. Everything the writes need is read before the plan
// is built. Non-empty creates make the object at target.openKey, a key that
// was free when the plan was prepared. targetMade says the target exists
// apart from the list, such as a device paired before Finish: a list refused
// because the wizard is gone then still ends done and opens the target, since
// the target is real.
export interface WizardFinishPlan {
  creates: WizardMutation[]
  target: WizardFinishTarget
  targetMade?: boolean
}

// WizardFinishTarget is what a Finish made: the object to open, the path a
// Space index naming the wizard moves to, and the name a success notice uses.
export interface WizardFinishTarget {
  openKey: string
  indexPath: string
  name?: string
}

// finishWizard applies a prepared Finish as one list: the creates, the
// ending, the index move and the wizard delete. The wizard exists until the
// list commits and its target exists only once it has, so a failure leaves
// the wizard, the index and the person's draft as they were, and Retry runs it
// all again. Every Finish, including one that creates nothing, refuses with
// WizardGoneError once the wizard is gone.
//
// The ending records target.openKey in the World with the wizard's removal,
// so whoever sees the wizard gone, this tab after a lost reply or a reload,
// or any other tab, reads whether it finished and as what.
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
      wizardEnding(wizard, plan.target.openKey),
      ...wizardRemoval(wizard, plan.target.indexPath),
    ],
    signal,
  )
}

// wizardEnding is the op that records the object a Finish opens as the
// wizard's ending.
function wizardEnding(wizard: WizardRef, openKey: string): WizardMutation {
  return {
    kind: 'op',
    opId: RecordWizardEndingOpTypeID,
    opData: RecordWizardEndingOp.toBinary({
      wizardKey: wizard.objectKey,
      targetKey: openKey,
    }),
  }
}

// finishLanded reports whether a Finish of plan landed though its reply was
// lost or its Retry found the wizard gone: the World records the wizard as
// finished as plan's target. An unreadable ending reads as not landed.
export async function finishLanded(
  wizard: WizardRef,
  plan: WizardFinishPlan,
  signal?: AbortSignal,
): Promise<boolean> {
  const ending = await lookupWizardEnding(
    wizard.spaceWorld,
    wizard.objectKey,
    signal,
  ).catch(() => undefined)
  return ending === plan.target.openKey
}

// cancelWizard removes a wizard without creating anything: a Space index
// that names it moves to indexPath, or is cleared. A wizard already gone,
// because another tab finished or cancelled it, refuses with
// WizardGoneError, so a Cancel never succeeds beside a Finish.
export async function cancelWizard(
  wizard: WizardRef,
  indexPath: string,
  signal?: AbortSignal,
) {
  await applyWizardMutations(
    wizard.spaceWorld,
    wizard.sessionPeerId,
    wizardRemoval(wizard, indexPath),
    signal,
  )
}

// WizardFinishState is the Finish control's state, from WizardFinish.tla.
// idle offers Finish, with the message of a press whose input was refused,
// before anything was written or by the operation itself; finishing applies
// the list; failed says what failed and offers Retry, which prepares and
// applies the whole list again; unsupported means the plugin behind the list
// has no build for this device, so no press here can finish and the wizard
// offers only its delete and Back; done means this press removed the wizard
// and its target opens; gone means the wizard was removed outside this press,
// and WizardGone reads from the World whether it finished, and as what, or
// was deleted.
//
// A failed list may have committed with its reply lost. The press checks the
// recorded ending before it shows failed, and a later Retry is refused as gone
// and checks it again; a failed press the watch then sees gone shows gone,
// where the ending names the object it made.
export type WizardFinishState =
  | { status: 'idle'; message?: string }
  | { status: 'finishing' }
  | { status: 'failed'; error: string }
  | { status: 'unsupported' }
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
// gone once the watched wizard object is removed outside a running or landed
// press. A press that is finishing or done removes the wizard itself, so the
// watch seeing it gone then is not an ending to show.
export function wizardFinishView(
  state: WizardFinishState,
  wizardGone: boolean,
): WizardFinishState {
  if (!wizardGone) return state
  if (state.status === 'finishing' || state.status === 'done') return state
  return { status: 'gone' }
}

// refusedFinishState is the state a press that threw ends in: idle with the
// message for an input the person can fix, which the operation's own
// rejection is too, since the same list fails the same way; gone for a wizard
// removed outside this press; unsupported for a plugin with no build for this
// device; and failed with Retry for anything else.
function refusedFinishState(err: unknown): WizardFinishState {
  if (
    err instanceof WizardInputError ||
    err instanceof WorldOperationRejection
  ) {
    return { status: 'idle', message: err.message }
  }
  if (err instanceof WizardGoneError) return { status: 'gone' }
  if (err instanceof PluginExecutableMissingError) {
    return { status: 'unsupported' }
  }
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
      let plan: WizardFinishPlan | undefined
      const done = (plan: WizardFinishPlan): WizardFinishState => {
        dispatch({ type: 'finish' })
        if (mountedRef.current) onDone(plan.target)
        return { status: 'done' }
      }
      try {
        plan = await prepare()
        await finishWizard(wizard, plan)
        return done(plan)
      } catch (err) {
        // A target made apart from the list, such as a paired device, opens
        // though another tab removed the wizard. A list whose reply was lost,
        // or this Retry of one, landed when the World records this plan's
        // target as the wizard's ending.
        if (
          plan &&
          !(err instanceof WizardInputError) &&
          ((err instanceof WizardGoneError && plan.targetMade) ||
            (await finishLanded(wizard, plan)))
        ) {
          return done(plan)
        }
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
