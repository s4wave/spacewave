import {
  CanvasInitOp,
  InitObjectLayoutOp,
  InitUnixFSOp,
} from '@s4wave/core/space/world/ops/ops.pb.js'
import { INIT_OBJECT_LAYOUT_OP_ID } from '@s4wave/core/space/world/ops/init-object-layout.js'
import { INIT_UNIXFS_OP_ID } from '@s4wave/core/space/world/ops/init-unixfs.js'
import { CREATE_CHAT_CHANNEL_OP_ID } from '@s4wave/sdk/chat/create-channel.js'
import { CREATE_COMPUTERS_DASHBOARD_OP_ID } from '@s4wave/sdk/device/computers/create-computers-dashboard.js'
import { CREATE_FORGE_DASHBOARD_OP_ID } from '@s4wave/sdk/forge/dashboard/create-forge-dashboard.js'
import { CreateChatChannelOp } from '@s4wave/sdk/chat/chat.pb.js'
import { CreateComputersDashboardOp } from '@s4wave/sdk/device/device.pb.js'
import { CreateForgeDashboardOp } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import { ClusterCreateOp } from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { CreateGitRepoWizardOp } from '@s4wave/core/git/git.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

const CANVAS_INIT_OP_ID = 'space/world/init-canvas'

export type BuildCreateOpFn = (
  objectKey: string,
  name: string,
  configData?: Uint8Array,
) => Uint8Array

const createOpBuilders = new Map<string, BuildCreateOpFn>([
  [
    CANVAS_INIT_OP_ID,
    (objectKey) => CanvasInitOp.toBinary({ objectKey, timestamp: new Date() }),
  ],
  [
    INIT_OBJECT_LAYOUT_OP_ID,
    (objectKey) =>
      InitObjectLayoutOp.toBinary({ objectKey, timestamp: new Date() }),
  ],
  [
    INIT_UNIXFS_OP_ID,
    (objectKey) => InitUnixFSOp.toBinary({ objectKey, timestamp: new Date() }),
  ],
  [
    CREATE_CHAT_CHANNEL_OP_ID,
    (objectKey, name) =>
      CreateChatChannelOp.toBinary({
        objectKey,
        name,
        topic: '',
        timestamp: new Date(),
      }),
  ],
  [
    CREATE_FORGE_DASHBOARD_OP_ID,
    (objectKey, name) =>
      CreateForgeDashboardOp.toBinary({
        objectKey,
        name,
        timestamp: new Date(),
      }),
  ],
  [
    CREATE_COMPUTERS_DASHBOARD_OP_ID,
    (objectKey, name) =>
      CreateComputersDashboardOp.toBinary({
        objectKey,
        name,
        timestamp: new Date(),
      }),
  ],
  [
    'forge/cluster/create',
    (objectKey, name) =>
      ClusterCreateOp.toBinary({
        clusterKey: objectKey,
        name,
        peerId: '',
      }),
  ],
  [
    'spacewave/forge/job/create',
    (objectKey, name, configData) => {
      if (configData?.length) {
        const config = ForgeJobCreateOp.fromBinary(configData)
        return ForgeJobCreateOp.toBinary({
          ...config,
          jobKey: objectKey,
          timestamp: new Date(),
        })
      }
      return ForgeJobCreateOp.toBinary({
        jobKey: objectKey,
        clusterKey: '',
        taskDefs: [{ name }],
        timestamp: new Date(),
      })
    },
  ],
  [
    'spacewave/forge/task/create',
    (objectKey, name, configData) => {
      if (configData?.length) {
        const config = ForgeTaskCreateOp.fromBinary(configData)
        return ForgeTaskCreateOp.toBinary({
          ...config,
          taskKey: objectKey,
          name,
          timestamp: new Date(),
        })
      }
      return ForgeTaskCreateOp.toBinary({
        taskKey: objectKey,
        name,
        jobKey: '',
        timestamp: new Date(),
      })
    },
  ],
  [
    'spacewave/git/repo/create',
    (objectKey, name, configData) => {
      if (configData?.length) {
        const config = CreateGitRepoWizardOp.fromBinary(configData)
        return CreateGitRepoWizardOp.toBinary({
          ...config,
          objectKey,
          name,
          timestamp: new Date(),
        })
      }
      return CreateGitRepoWizardOp.toBinary({
        objectKey,
        name,
        timestamp: new Date(),
      })
    },
  ],
])

export function lookupCreateOpBuilder(
  createOpId: string,
): BuildCreateOpFn | undefined {
  return createOpBuilders.get(createOpId)
}

// The key builders read the taken keys under their candidate prefix from the
// World when an object is created, so naming never needs the whole listing.

// buildObjectKey returns the first free numbered key for name.
export async function buildObjectKey(
  world: IWorldState,
  prefix: string,
  name: string,
  signal?: AbortSignal,
): Promise<string> {
  const base = slugObjectKeySegment(name) || buildObjectKeyBase(prefix)
  const taken = await listObjectKeys(world, `${base}-`, signal)
  return firstFreeKey(taken, (n) => `${base}-${n}`, 1)
}

// buildForgeObjectKey preserves a requested Forge key unless it collides.
export async function buildForgeObjectKey(
  world: IWorldState,
  prefix: string,
  name: string,
  signal?: AbortSignal,
): Promise<string> {
  const base = slugObjectKeySegment(name) || buildObjectKeyBase(prefix)
  const taken = await listObjectKeys(world, base, signal)
  if (!taken.has(base)) return base
  return firstFreeKey(taken, (n) => `${base}-${n}`, 2)
}

// buildWizardObjectKey returns the first free numbered wizard key for name.
export async function buildWizardObjectKey(
  world: IWorldState,
  name: string,
  signal?: AbortSignal,
): Promise<string> {
  const prefix = wizardObjectKey(name, 0).slice(0, -1)
  const taken = await listObjectKeys(world, prefix, signal)
  return firstFreeKey(taken, (n) => wizardObjectKey(name, n), 1)
}

// wizardObjectKey returns the nth numbered wizard key for name. A caller
// whose name is already unique, such as a timestamped quickstart, uses the
// first key without reading the World.
export function wizardObjectKey(name: string, n = 1): string {
  return `wizard/${slugObjectKeySegment(name) || 'wizard'}-${n}`
}

// listObjectKeys returns every key under prefix, one listing page at a time.
async function listObjectKeys(
  world: IWorldState,
  prefix: string,
  signal?: AbortSignal,
): Promise<Set<string>> {
  const keys = new Set<string>()
  let startAfter = ''
  for (;;) {
    const page = await world.listObjects(
      { prefix, startAfter, limit: 1000 },
      signal,
    )
    for (const object of page.objects ?? []) {
      if (object.objectKey) keys.add(object.objectKey)
    }
    const last = page.objects?.at(-1)?.objectKey
    if (!page.more || !last) return keys
    startAfter = last
  }
}

// firstFreeKey returns the first candidate from first upward that is not taken.
function firstFreeKey(
  taken: Set<string>,
  candidate: (n: number) => string,
  first: number,
): string {
  let n = first
  while (taken.has(candidate(n))) n++
  return candidate(n)
}

function buildObjectKeyBase(prefix: string): string {
  const segments = prefix.split('/').filter(Boolean)
  const prefixBase = segments[segments.length - 1]
  return slugObjectKeySegment(prefixBase || 'object')
}

function slugObjectKeySegment(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}
