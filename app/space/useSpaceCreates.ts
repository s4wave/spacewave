import { createContext, useContext, useEffect, useMemo, useRef } from 'react'

import { toast } from '@s4wave/web/ui/toaster.js'
import {
  PluginOpTimeoutError,
  WorldOperationRejection,
} from '@s4wave/sdk/world/errors.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// CreatePlan is one create command's object key and op, built once.
export interface CreatePlan {
  name: string
  objectKey: string
  // objectType is the type the op gives the object at objectKey.
  objectType: string
  opId: string
  opData: Uint8Array
  // persistent creates a wizard object rather than the target object.
  persistent: boolean
}

// PrepareCreate picks a free key in world and builds the op that creates
// the object there, or returns null when there is nothing to create.
export type PrepareCreate = (world: IWorldState) => Promise<CreatePlan | null>

// SpaceCreates runs the Create Object commands of one open Space.
export interface SpaceCreates {
  // create runs one create of the object named name. Its outcome is
  // reported for as long as the Space stays open, across world reloads.
  create(name: string, prepare: PrepareCreate): void
}

// SpaceCreatesContext carries the open Space's SpaceCreates.
export const SpaceCreatesContext = createContext<SpaceCreates | null>(null)

// useSpaceCreatesContext returns the open Space's SpaceCreates.
export function useSpaceCreatesContext(): SpaceCreates {
  const creates = useContext(SpaceCreatesContext)
  if (!creates) throw new Error('SpaceCreatesContext is not provided')
  return creates
}

// OpenSpace is one open Space's creates: the dispatch of each create still
// open, which closing the Space sends close; the runs waiting for the Space's
// world to return; and the create that holds each planned key while it is
// running, waiting or failed, which a later create of that key joins.
interface OpenSpace {
  jobs: Set<(event: CreateEvent) => void>
  waiting: Map<CreateJob, (world: IWorldState) => void>
  keys: Map<string, CreateJob>
}

// CreateState is one create's state. running sends the create, after a
// failure if failure is set; waiting holds a pressed Try again until the
// Space's world returns; failed offers Try again; refused means another
// object holds the planned key, so trying again cannot help; done and closed
// are final; joined is final for a create whose planned key another create
// in this Space already holds, which reports the one outcome. failure is the
// last failure's description, which a cancelled wait shows again.
export type CreateState =
  | { status: 'running'; failure?: string }
  | { status: 'waiting'; failure?: string }
  | { status: 'failed'; failure: string }
  | { status: 'refused'; failure: string }
  | { status: 'done' }
  | { status: 'joined' }
  | { status: 'closed' }

// CreateEvent moves one create between its states.
export type CreateEvent =
  | { type: 'retry' }
  | { type: 'wait' }
  | { type: 'resume' }
  | { type: 'cancel' }
  | { type: 'fail'; failure: string; retry: boolean }
  | { type: 'done' }
  | { type: 'join' }
  | { type: 'close' }

// createReducer returns the state after event. An event that does not apply
// to the state leaves it, so a Try again or Cancel pressed twice, or a run
// finishing after its Space closed, changes nothing.
export function createReducer(
  state: CreateState,
  event: CreateEvent,
): CreateState {
  switch (event.type) {
    case 'retry':
      return state.status === 'failed'
        ? { status: 'running', failure: state.failure }
        : state
    case 'wait':
      return state.status === 'running'
        ? { status: 'waiting', failure: state.failure }
        : state
    case 'resume':
      return state.status === 'waiting'
        ? { status: 'running', failure: state.failure }
        : state
    case 'cancel':
      return state.status === 'waiting'
        ? {
            status: 'failed',
            failure: state.failure ?? 'The Space was reconnecting.',
          }
        : state
    case 'fail':
      if (state.status !== 'running') return state
      return event.retry
        ? { status: 'failed', failure: event.failure }
        : { status: 'refused', failure: event.failure }
    case 'done':
      return state.status === 'running' ? { status: 'done' } : state
    case 'join':
      return state.status === 'running' ? { status: 'joined' } : state
    case 'close':
      return state.status === 'done' || state.status === 'joined'
        ? state
        : { status: 'closed' }
  }
}

// ToastPress is the click sonner passes a toast action. Preventing its
// default keeps sonner from deleting the toast id the action replaces.
type ToastPress = { preventDefault(): void } | undefined

// CreateJob is one create: its state, its plan once prepared, whether an op
// it sent may have been written, and the id of the one toast that shows its
// state, unique to this create.
interface CreateJob {
  name: string
  state: CreateState
  plan: CreatePlan | null
  sent: boolean
  toastId: string
}

// showCreateToast replaces job's toast with the one its state shows. Every
// toast passes every option, since sonner merges an update into the toast
// under the same id.
function showCreateToast(
  job: CreateJob,
  dispatch: (event: CreateEvent) => void,
) {
  const state = job.state
  const press = (event: CreateEvent) => (click: ToastPress) => {
    click?.preventDefault()
    dispatch(event)
  }
  // A failure the person closes, by its Dismiss or a swipe, ends its create.
  // sonner deletes the toast after a cancel press, which close does too.
  const onDismiss = () => dispatch({ type: 'close' })
  const dismiss = { label: 'Dismiss', onClick: onDismiss }
  switch (state.status) {
    case 'running':
      // The first run shows nothing until it fails or finishes.
      if (state.failure === undefined) return
      toast.loading(`Creating ${job.name}`, {
        id: job.toastId,
        description: undefined,
        duration: Infinity,
        action: undefined,
        cancel: undefined,
        onDismiss: undefined,
      })
      return
    case 'waiting':
      toast.loading(`Creating ${job.name}`, {
        id: job.toastId,
        description: 'Waiting for the Space to reconnect.',
        duration: Infinity,
        action: { label: 'Cancel', onClick: press({ type: 'cancel' }) },
        cancel: undefined,
        onDismiss: undefined,
      })
      return
    case 'failed':
      toast.error(`Could not create ${job.name}`, {
        id: job.toastId,
        description: state.failure,
        duration: Infinity,
        action: { label: 'Try again', onClick: press({ type: 'retry' }) },
        cancel: dismiss,
        onDismiss,
      })
      return
    case 'refused':
      toast.error(`Could not create ${job.name}`, {
        id: job.toastId,
        description: state.failure,
        duration: Infinity,
        action: undefined,
        cancel: dismiss,
        onDismiss,
      })
      return
    case 'done':
    case 'closed':
      toast.dismiss(job.toastId)
      return
    case 'joined':
      return
  }
}

// createFailure describes why a create's send failed.
function createFailure(err: unknown): string {
  if (err instanceof WorldOperationRejection) return err.message
  if (err instanceof PluginOpTimeoutError)
    return 'The plugin did not finish in time, so nothing was changed.'
  return 'The Space could not finish the change.'
}

// readObjectType returns the type of the object at key, '' for an object
// with no type, or null when no object exists there.
async function readObjectType(
  world: IWorldState,
  key: string,
): Promise<string | null> {
  const page = await world.listObjects({ prefix: key, limit: 1 })
  const object = page.objects?.[0]
  return object?.objectKey === key ? (object.typeId ?? '') : null
}

// useSpaceCreates owns the creates of the Space spaceId. The caller sits
// above the guard that unmounts the Space's body while its world reloads,
// so a create outlives that reload: world is the Space's current world, or
// null while it loads. A create that fails shows one toast whose Try again
// waits for the world, then reads the planned key and resends the same op.
// When the Space closes or spaceId changes, its toasts are dismissed and its
// creates report nothing more.
export function useSpaceCreates(
  spaceId: string,
  world: IWorldState | null,
  navigateToObjects: (objectKeys: string[]) => void,
): SpaceCreates {
  const liveWorld = useRef(world)
  const navigate = useRef(navigateToObjects)
  const open = useRef<OpenSpace | null>(null)

  useEffect(() => {
    navigate.current = navigateToObjects
  }, [navigateToObjects])

  useEffect(() => {
    const space: OpenSpace = {
      jobs: new Set(),
      waiting: new Map(),
      keys: new Map(),
    }
    open.current = space
    return () => {
      space.waiting.clear()
      for (const dispatch of space.jobs) dispatch({ type: 'close' })
      if (open.current === space) open.current = null
    }
  }, [spaceId])

  useEffect(() => {
    liveWorld.current = world
    const space = open.current
    if (!world || !space) return
    const waiting = [...space.waiting.values()]
    space.waiting.clear()
    for (const resume of waiting) resume(world)
  }, [world])

  return useMemo<SpaceCreates>(
    () => ({
      create(name, prepare) {
        const space = open.current
        if (!space) return
        const job: CreateJob = {
          name,
          state: { status: 'running' },
          plan: null,
          sent: false,
          toastId: `create-object:${crypto.randomUUID()}`,
        }

        // dispatch moves job to its next state, shows that state's toast and
        // starts the run a Try again or a returned world asks for. Only
        // running sends, and only retry and resume enter it, from states
        // with no run in flight, so one create never runs twice at once.
        const dispatch = (event: CreateEvent) => {
          const prev = job.state
          const next = createReducer(prev, event)
          if (next === prev) return
          job.state = next
          const final =
            next.status === 'done' ||
            next.status === 'joined' ||
            next.status === 'closed'
          if (final) space.jobs.delete(dispatch)
          if (final || next.status === 'refused') releaseKey()
          if (prev.status === 'waiting') space.waiting.delete(job)
          showCreateToast(job, dispatch)
          if (event.type === 'retry') start()
        }
        // run sends the create on the Space's world, waiting for it while it
        // reloads.
        const run = async () => {
          let world = liveWorld.current
          if (!world) {
            dispatch({ type: 'wait' })
            world = await new Promise<IWorldState>((resume) =>
              space.waiting.set(job, resume),
            )
            dispatch({ type: 'resume' })
          }
          if (job.state.status !== 'running') return
          if (!job.plan) {
            const plan = await prepare(world)
            // Nothing to create, or the Space closed while this planned.
            if (!plan || job.state.status !== 'running') {
              dispatch({ type: 'close' })
              return
            }
            // A create whose key another create here holds joins it, so one
            // object is sent and reported once.
            if (space.keys.has(plan.objectKey)) {
              dispatch({ type: 'join' })
              return
            }
            job.plan = plan
            space.keys.set(plan.objectKey, job)
          }
          const plan = job.plan
          // Contract: a create whose reply was lost may still have committed,
          // and the op does not yet treat an identical repeat as a no-op, so
          // a repeat first reads its key. The object there is either this
          // create landing, or another writer's object that this create must
          // not touch.
          if (job.sent) {
            const found = await readObjectType(world, plan.objectKey)
            if (found !== null && found !== plan.objectType) {
              dispatch({
                type: 'fail',
                failure: 'The Space could not finish the change.',
                retry: false,
              })
              return
            }
            if (found !== null) return finish(plan)
          }
          // Only a create still running sends: its Space may have closed
          // while it planned or read its key.
          if (job.state.status !== 'running') return
          job.sent = true
          await world.applyWorldOp(plan.opId, plan.opData, '')
          finish(plan)
        }
        const finish = (plan: CreatePlan) => {
          if (job.state.status !== 'running') return
          dispatch({ type: 'done' })
          if (!plan.persistent) toast.success(`Created ${plan.name}`)
          navigate.current([plan.objectKey])
        }
        const releaseKey = () => {
          const key = job.plan?.objectKey
          if (key !== undefined && space.keys.get(key) === job)
            space.keys.delete(key)
        }
        const start = () => {
          run().catch((err: unknown) => {
            console.error('failed to create object', err)
            // The host discarded a timed out op, so it wrote nothing, and
            // the read before this send already settled any earlier one: a
            // retry sends without reading the key first.
            if (err instanceof PluginOpTimeoutError) job.sent = false
            dispatch({
              type: 'fail',
              failure: createFailure(err),
              retry: true,
            })
          })
        }
        space.jobs.add(dispatch)
        start()
      },
    }),
    [],
  )
}
