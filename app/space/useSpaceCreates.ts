import { createContext, useContext, useEffect, useMemo, useRef } from 'react'

import { toast } from '@s4wave/web/ui/toaster.js'
import { WorldOperationRejection } from '@s4wave/sdk/world/errors.js'
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
// open, which closing the Space sends close, and the runs waiting for the
// Space's world to return.
interface OpenSpace {
  jobs: Set<(event: CreateEvent) => void>
  waiting: Map<CreateJob, (world: IWorldState) => void>
}

// CreateState is one create's state. running sends the create, after a
// failure if failure is set; waiting holds a pressed Try again until the
// Space's world returns; failed offers Try again; refused means another
// object holds the planned key, so trying again cannot help; done and closed
// are final. failure is the last failure's description, which a cancelled
// wait shows again.
export type CreateState =
  | { status: 'running'; failure?: string }
  | { status: 'waiting'; failure?: string }
  | { status: 'failed'; failure: string }
  | { status: 'refused'; failure: string }
  | { status: 'done' }
  | { status: 'closed' }

// CreateEvent moves one create between its states.
export type CreateEvent =
  | { type: 'retry' }
  | { type: 'wait' }
  | { type: 'resume' }
  | { type: 'cancel' }
  | { type: 'fail'; failure: string; retry: boolean }
  | { type: 'done' }
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
    case 'close':
      return state.status === 'done' ? state : { status: 'closed' }
  }
}

// ToastPress is the click sonner passes a toast action. Preventing its
// default keeps sonner from deleting the toast id the action replaces.
type ToastPress = { preventDefault(): void } | undefined

// CreateJob is one create: its state, its plan once prepared, whether its op
// was ever sent, and the one toast that shows its state.
interface CreateJob {
  name: string
  state: CreateState
  plan: CreatePlan | null
  sent: boolean
  toastId: string | null
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
  switch (state.status) {
    case 'running':
      // The first run shows nothing until it fails or finishes.
      if (state.failure === undefined) return
      toast.loading(`Creating ${job.name}`, {
        id: createToastId(job),
        description: undefined,
        duration: Infinity,
        action: undefined,
      })
      return
    case 'waiting':
      toast.loading(`Creating ${job.name}`, {
        id: createToastId(job),
        description: 'Waiting for the Space to reconnect.',
        duration: Infinity,
        action: { label: 'Cancel', onClick: press({ type: 'cancel' }) },
      })
      return
    case 'failed':
      toast.error(`Could not create ${job.name}`, {
        id: createToastId(job),
        description: state.failure,
        duration: Infinity,
        action: { label: 'Try again', onClick: press({ type: 'retry' }) },
      })
      return
    case 'refused':
      toast.error(`Could not create ${job.name}`, {
        id: createToastId(job),
        description: state.failure,
        duration: undefined,
        action: undefined,
      })
      return
    case 'done':
    case 'closed':
      if (job.toastId) toast.dismiss(job.toastId)
      return
  }
}

// createToastId names job's one toast by its planned key, fixed at the first
// toast it shows.
function createToastId(job: CreateJob): string {
  job.toastId ??= `create-object:${job.plan?.objectKey ?? crypto.randomUUID()}`
  return job.toastId
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
    const space: OpenSpace = { jobs: new Set(), waiting: new Map() }
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
          toastId: null,
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
          if (next.status === 'done' || next.status === 'closed')
            space.jobs.delete(dispatch)
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
          job.plan ??= await prepare(world)
          const plan = job.plan
          if (!plan) {
            dispatch({ type: 'close' })
            return
          }
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
        const start = () => {
          run().catch((err: unknown) => {
            console.error('failed to create object', err)
            dispatch({
              type: 'fail',
              failure:
                err instanceof WorldOperationRejection
                  ? err.message
                  : 'The Space could not finish the change.',
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
