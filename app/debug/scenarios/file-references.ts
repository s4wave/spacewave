import { BlobType } from '@go/github.com/s4wave/spacewave/db/block/blob/blob.pb.js'
import {
  FSNode,
  NodeType,
} from '@go/github.com/s4wave/spacewave/db/unixfs/block/fstree.pb.js'
import { FSNodeTypeID } from '@go/github.com/s4wave/spacewave/db/unixfs/block/fstree_typeid.pb.js'
import { createQuickstartSetup } from '@s4wave/app/quickstart/create.js'
import type {
  AppSetupContext,
  AppSetupResult,
} from '@s4wave/app/SpacewaveApp.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { throwBatchError } from '@s4wave/sdk/world/errors.js'
import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import { accessObject } from '@s4wave/sdk/world/utils.js'

const fileKey = 'docking-arm.md'
const observationType = 'glados/observation'
const refersPred = '<glados/evidence>'

// putObject stores one block as a new World object of typeId, with any quads
// the object declares, in one batch.
async function putObject(
  world: EngineWorldState,
  key: string,
  typeId: string,
  data: Uint8Array,
  blockType: string,
  quads: { predicate: string; obj: string }[],
  signal: AbortSignal,
) {
  using stage = await world.stageWorldState(signal)
  using cursor = await stage.buildStorageCursor(signal)
  const rootRef = await accessObject(
    cursor,
    undefined,
    async (block) => {
      await block.setBlock({ data, blockType, markDirty: true }, signal)
    },
    signal,
  )
  throwBatchError(
    await world.getEngine().executeWorldBatch([
      {
        mutation: { case: 'createObject', value: { objectKey: key, rootRef } },
      },
      {
        mutation: { case: 'setObjectType', value: { objectKey: key, typeId } },
      },
      ...quads.map((quad) => ({
        mutation: {
          case: 'setGraphQuad' as const,
          value: { quad: { subject: keyToIRI(key), ...quad } },
        },
      })),
    ]),
  )
}

// fileReferences opens a design file stored as its own World object, as
// GLaDOS stores files, with nothing yet referring to it. Its debug context
// carries addObservation, which writes an Observation naming the file, so a
// check can watch the references strip take it in while the file is open.
export async function fileReferences(
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
  reportProgress('Writing the design file')
  const world = setup.spaceWorld
  const content = new TextEncoder().encode(
    Array.from(
      { length: 30 },
      (_, i) => `${i + 1}. Design note: the docking arm reaches bay ${i + 1}.`,
    ).join('\n') + '\n',
  )
  const size = BigInt(content.length)
  await putObject(
    world,
    fileKey,
    FSNodeTypeID,
    FSNode.toBinary({
      nodeType: NodeType.NodeType_FILE,
      permissions: 0o644,
      file: {
        totalSize: size,
        rootBlob: {
          blobType: BlobType.BlobType_RAW,
          totalSize: size,
          rawData: content,
        },
      },
    }),
    FSNodeTypeID,
    [],
    signal,
  )
  let observations = 0
  const addObservation = async () => {
    observations++
    const key = `glados/observations/${observations}`
    await putObject(
      world,
      key,
      observationType,
      new TextEncoder().encode(`The docking arm reaches bay ${observations}.`),
      '',
      [{ predicate: refersPred, obj: keyToIRI(fileKey) }],
      signal,
    )
    return key
  }
  const spaceId = setup.spaceResp.sharedObjectRef?.providerResourceRef?.id
  if (!spaceId || !setup.sessionIndex)
    throw new Error('Scenario setup did not return a Session and Space')
  return {
    path: `/u/${setup.sessionIndex}/so/${spaceId}/-/${fileKey}`,
    debug: { setup, spaceId, fileKey, addObservation },
  }
}
