import { Cluster } from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import { ClusterTypeID } from '@go/github.com/s4wave/spacewave/forge/cluster/cluster_typeid.pb.js'
import {
  Job,
  State as JobState,
} from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import { JobTypeID } from '@go/github.com/s4wave/spacewave/forge/job/job_typeid.pb.js'
import {
  Task,
  State as TaskState,
} from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { TaskTypeID } from '@go/github.com/s4wave/spacewave/forge/task/task_typeid.pb.js'
import { Worker } from '@go/github.com/s4wave/spacewave/forge/worker/worker.pb.js'
import { WorkerTypeID } from '@go/github.com/s4wave/spacewave/forge/worker/worker_typeid.pb.js'
import { createQuickstartSetup } from '@s4wave/app/quickstart/create.js'
import type {
  AppSetupContext,
  AppSetupResult,
} from '@s4wave/app/SpacewaveApp.js'
import { ChatChannel } from '@s4wave/sdk/chat/chat.pb.js'
import { ChatChannelTypeID } from '@s4wave/sdk/chat/chat_typeid.pb.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { throwBatchError } from '@s4wave/sdk/world/errors.js'
import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'

import { putObject } from './file-references.js'

// hangarGoal is the object the scenario's relations center on.
const hangarGoal = 'goals/dock'

// receiptCount is how many receipts point at the goal, enough to fold into one
// counted family.
const receiptCount = 40

// HangarObject is one object the scenario writes.
interface HangarObject {
  key: string
  typeId: string
  root?: { data: Uint8Array; blockType: string }
  links?: [predicate: string, target: string][]
}

// hangarObjects are titled objects of four types, forge jobs and a task that
// declare their states and times, untitled objects of three plugin types, and
// relations both ways around the goal: eight outgoing groups, more than the
// graph draws, and four incoming groups, two of them families: jobs in mixed
// states and receipts.
function hangarObjects(): HangarObject[] {
  const hoursAgo = (h: number) => new Date(Date.now() - h * 3600_000)
  const job = (
    key: string,
    jobState: JobState,
    result?: { success: boolean; failError?: string },
  ): HangarObject => ({
    key,
    typeId: JobTypeID,
    root: {
      blockType: JobTypeID,
      data: Job.toBinary({ jobState, result, timestamp: hoursAgo(3) }),
    },
    links: [['<glados/pursues>', hangarGoal]],
  })
  const channel = (key: string, name: string): HangarObject => ({
    key,
    typeId: ChatChannelTypeID,
    root: {
      blockType: ChatChannelTypeID,
      data: ChatChannel.toBinary({ name }),
    },
  })
  const worker = (key: string, name: string): HangarObject => ({
    key,
    typeId: WorkerTypeID,
    root: { blockType: WorkerTypeID, data: Worker.toBinary({ name }) },
  })
  const goal = (key: string, links: HangarObject['links'] = []) => ({
    key,
    typeId: 'glados/goal',
    links,
  })
  return [
    channel('channels/general', 'General'),
    channel('channels/ops', 'Docking operations'),
    {
      key: 'clusters/bay',
      typeId: ClusterTypeID,
      root: {
        blockType: ClusterTypeID,
        data: Cluster.toBinary({ name: 'Docking bay cluster' }),
      },
    },
    worker('workers/arm', 'Arm controller'),
    worker('workers/clamp', 'Clamp controller'),
    worker('workers/camera', 'Camera relay'),
    goal('goals/power'),
    goal('goals/survey'),
    goal('goals/beacon'),
    goal('notes/approach'),
    {
      key: 'tasks/align',
      typeId: TaskTypeID,
      root: {
        blockType: TaskTypeID,
        data: Task.toBinary({
          name: 'Align the docking clamp',
          taskState: TaskState.TaskState_RUNNING,
          timestamp: hoursAgo(2),
        }),
      },
    },
    job('jobs/dock-build', JobState.JobState_RUNNING),
    job('jobs/dock-test', JobState.JobState_COMPLETE, { success: true }),
    job('jobs/clamp-calibrate', JobState.JobState_COMPLETE, {
      success: false,
      failError: 'clamp out of range',
    }),
    goal(hangarGoal, [
      ['<glados/owner>', 'clusters/bay'],
      ['<glados/step>', 'tasks/align'],
      ['<glados/channel>', 'channels/general'],
      ['<glados/depends-on>', 'goals/power'],
      ['<glados/depends-on>', 'goals/survey'],
      ['<glados/worker>', 'workers/arm'],
      ['<glados/worker>', 'workers/clamp'],
      ['<glados/worker>', 'workers/camera'],
      ['<glados/signals>', 'goals/beacon'],
      ['<glados/note>', 'notes/approach'],
      ['<glados/reviewed-in>', 'channels/ops'],
    ]),
    goal('goals/launch', [['<glados/depends-on>', hangarGoal]]),
    {
      key: 'threads/dock-review',
      typeId: 'glados/thread',
      links: [['<glados/topic>', hangarGoal]],
    },
    ...Array.from({ length: receiptCount }, (_, i) => ({
      key: `receipts/${String(i + 1).padStart(2, '0')}`,
      typeId: 'glados/receipt',
      links: [['<glados/goal>', hangarGoal]] as HangarObject['links'],
    })),
  ]
}

// deleteObject deletes an object as another writer would.
async function deleteObject(world: EngineWorldState, objectKey: string) {
  throwBatchError(
    await world
      .getEngine()
      .executeWorldBatch([
        { mutation: { case: 'deleteObject', value: { objectKey } } },
      ]),
  )
}

// objectBrowser opens a Space with no index object, so its root shows the
// object browser over the hangar objects. Its debug context carries
// deleteObject and addReceipt, so a check can delete the open object or grow
// a count as another writer.
export async function objectBrowser(
  context: AppSetupContext,
): Promise<AppSetupResult> {
  const { root, cleanup, signal, reportProgress } = context
  const setup = await createQuickstartSetup(
    root,
    'space',
    signal,
    cleanup,
    ({ detail }) => reportProgress(detail),
  )
  reportProgress('Writing the hangar objects')
  const world = setup.spaceWorld
  const objects = hangarObjects()
  await Promise.all(
    objects.map((object) =>
      putObject(
        world,
        object.key,
        object.typeId,
        object.root ?? null,
        [],
        signal,
      ),
    ),
  )
  // The relations go in once every end exists, in one batch.
  throwBatchError(
    await world.getEngine().executeWorldBatch(
      objects.flatMap((object) =>
        (object.links ?? []).map(([predicate, target]) => ({
          mutation: {
            case: 'setGraphQuad' as const,
            value: {
              quad: {
                subject: keyToIRI(object.key),
                predicate,
                obj: keyToIRI(target),
              },
            },
          },
        })),
      ),
    ),
  )
  let receipts = receiptCount
  const addReceipt = async () => {
    receipts++
    const key = `receipts/${receipts}`
    await putObject(
      world,
      key,
      'glados/receipt',
      null,
      [{ predicate: '<glados/goal>', obj: keyToIRI(hangarGoal) }],
      signal,
    )
    return key
  }
  const spaceId = setup.spaceResp.sharedObjectRef?.providerResourceRef?.id
  if (!spaceId || !setup.sessionIndex)
    throw new Error('Scenario setup did not return a Session and Space')
  return {
    path: `/u/${setup.sessionIndex}/so/${spaceId}`,
    debug: {
      setup,
      spaceId,
      goalKey: hangarGoal,
      deleteObject: (objectKey: string) => deleteObject(world, objectKey),
      addReceipt,
    },
  }
}
