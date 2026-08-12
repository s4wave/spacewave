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

// buildObjectKey constructs the next simple numbered world object key.
export function buildObjectKey(
  prefix: string,
  name: string,
  existingKeys?: Iterable<string | undefined>,
): string {
  const base = slugObjectKeySegment(name) || buildObjectKeyBase(prefix)
  const existing = new Set(existingKeys ?? [])
  const candidates = Array.from(
    { length: existing.size + 2 },
    (_, index) => `${base}-${index + 1}`,
  )
  return candidates.find((key) => !existing.has(key)) ?? `${base}-1`
}

// buildForgeObjectKey preserves a requested Forge key unless it collides.
export function buildForgeObjectKey(
  prefix: string,
  name: string,
  existingKeys?: Iterable<string | undefined>,
): string {
  const base = slugObjectKeySegment(name) || buildObjectKeyBase(prefix)
  const existing = new Set(existingKeys ?? [])
  if (!existing.has(base)) return base

  const candidates = Array.from(
    { length: existing.size + 2 },
    (_, index) => `${base}-${index + 2}`,
  )
  return candidates.find((key) => !existing.has(key)) ?? `${base}-2`
}

export function buildWizardObjectKey(
  name: string,
  existingKeys?: Iterable<string | undefined>,
): string {
  const base = slugObjectKeySegment(name) || 'wizard'
  const existing = new Set(existingKeys ?? [])
  const candidates = Array.from(
    { length: existing.size + 2 },
    (_, index) => `wizard/${base}-${index + 1}`,
  )
  return candidates.find((key) => !existing.has(key)) ?? `wizard/${base}-1`
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
