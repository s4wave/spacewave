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

// OpenSpace is one open Space's creates: their failure toasts, and the
// presses of Try again waiting for the Space's world to return.
interface OpenSpace {
  closed: boolean
  toasts: Set<string>
  waiting: Array<(world: IWorldState) => void>
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
    const space: OpenSpace = { closed: false, toasts: new Set(), waiting: [] }
    open.current = space
    return () => {
      space.closed = true
      space.waiting = []
      for (const id of space.toasts) toast.dismiss(id)
      space.toasts.clear()
      if (open.current === space) open.current = null
    }
  }, [spaceId])

  useEffect(() => {
    liveWorld.current = world
    const space = open.current
    if (!world || !space) return
    const waiting = space.waiting
    space.waiting = []
    for (const resume of waiting) resume(world)
  }, [world])

  return useMemo<SpaceCreates>(
    () => ({
      create(name, prepare) {
        const space = open.current
        if (!space) return
        let plan: CreatePlan | null = null
        let sent = false
        let toastId: string | undefined

        const show = () => {
          toastId ??= `create-object:${plan?.objectKey ?? crypto.randomUUID()}`
          space.toasts.add(toastId)
          return toastId
        }
        const nextWorld = () =>
          liveWorld.current ??
          new Promise<IWorldState>((resume) => {
            toast.loading(`Creating ${name}`, {
              id: show(),
              description: 'Waiting for the Space to reconnect.',
              duration: Infinity,
            })
            space.waiting.push(resume)
          })
        const done = (created: CreatePlan) => {
          if (space.closed) return
          if (toastId) {
            toast.dismiss(toastId)
            space.toasts.delete(toastId)
          }
          if (!created.persistent) toast.success(`Created ${created.name}`)
          navigate.current([created.objectKey])
        }
        const fail = (err: unknown, retry: boolean) => {
          if (space.closed) return
          toast.error(`Could not create ${name}`, {
            id: show(),
            description:
              err instanceof WorldOperationRejection
                ? err.message
                : 'The Space could not finish the change.',
            ...(retry && {
              duration: Infinity,
              action: { label: 'Try again', onClick: attempt },
            }),
          })
        }
        // run waits for the Space's world; a closed Space drops its waiting
        // presses, so a run that resumes acts while the Space is open.
        const run = async () => {
          if (space.closed) return
          const world = await nextWorld()
          plan ??= await prepare(world)
          if (!plan) return
          // Contract: a create whose reply was lost may still have committed,
          // and the op does not yet treat an identical repeat as a no-op, so
          // a repeat first reads its key. The object there is either this
          // create landing, or another writer's object that this create must
          // not touch.
          if (sent) {
            const found = await readObjectType(world, plan.objectKey)
            if (found === plan.objectType) {
              done(plan)
              return
            }
            if (found !== null) {
              fail(null, false)
              return
            }
          }
          sent = true
          await world.applyWorldOp(plan.opId, plan.opData, '')
          done(plan)
        }
        const attempt = () => {
          run().catch((err: unknown) => {
            console.error('failed to create object', err)
            fail(err, true)
          })
        }
        attempt()
      },
    }),
    [],
  )
}
