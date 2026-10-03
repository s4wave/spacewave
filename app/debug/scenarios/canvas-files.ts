import { createQuickstartSetup } from '@s4wave/app/quickstart/create.js'
import type {
  AppSetupContext,
  AppSetupResult,
} from '@s4wave/app/SpacewaveApp.js'
import { CanvasHandle } from '@s4wave/sdk/canvas/canvas.js'
import { NodeType } from '@s4wave/sdk/canvas/canvas.pb.js'
import { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { MknodType } from '@s4wave/sdk/unixfs/handle.pb.js'
import { accessObject } from '@s4wave/sdk/world/utils.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import {
  createSingleRowObjectLayout,
  ObjectLayoutTypeID,
  serializeObjectLayout,
} from '@s4wave/sdk/layout/world/object-layout.js'

const imageName = 'alpine-lake.svg'
const landscape = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="800" viewBox="0 0 1200 800">
<defs><linearGradient id="sky" x2="0" y2="1"><stop stop-color="#8193b6"/><stop offset="1" stop-color="#edc6b5"/></linearGradient><linearGradient id="water" x2="0" y2="1"><stop stop-color="#64889a"/><stop offset="1" stop-color="#192f45"/></linearGradient></defs>
<rect width="1200" height="800" fill="url(#sky)"/><circle cx="918" cy="191" r="55" fill="#f5dcc9"/>
<path d="M0 466 230 189 420 430 647 120 909 443 1092 228 1200 398V800H0Z" fill="#455769"/>
<path d="m125 316 105-127 115 146-91-40-42 31-38-34zm392-22 130-174 141 174-116-40-42 47-36-49z" fill="#d2dce0"/>
<path d="M0 492Q260 425 530 501T1200 485V800H0Z" fill="url(#water)"/>
<path d="M0 586 186 553 354 583 202 605 0 599zm762 21 282-31 156 10v20l-261 19z" fill="#a8aeb2" opacity=".35"/>
<path d="M0 699 84 659 169 691 263 656 399 718 637 751 844 720 1010 651 1200 684V800H0Z" fill="#172c31"/>
</svg>`

async function seedCanvasFiles(
  context: AppSetupContext,
  openImage: boolean,
): Promise<AppSetupResult> {
  const { root, cleanup, signal, reportProgress } = context
  const setup = await createQuickstartSetup(
    root,
    'canvas',
    signal,
    cleanup,
    ({ detail }) => reportProgress(detail),
  )
  reportProgress('Adding reference files')
  const world = setup.spaceWorld
  const filesAccess = await world.accessTypedObject('files', signal)
  const files = cleanup(
    new FSHandle(world.getResourceRef().createRef(filesAccess.resourceId)),
  )
  await files.mknod(
    ['Drawings', 'References', 'Exports'],
    MknodType.DIR,
    0o755,
    false,
    signal,
  )
  const upload = async (name: string, content: string) => {
    const data = new TextEncoder().encode(content)
    await files.uploadFile(
      name,
      BigInt(data.length),
      new ReadableStream({
        start(controller) {
          controller.enqueue(data)
          controller.close()
        },
      }),
      0o644,
      undefined,
      signal,
    )
  }
  await upload(imageName, landscape)
  await upload(
    'field-notes.md',
    '# Alpine study\n\nExplore the lake image, sketch on the Canvas, and keep reference files nearby.\n',
  )
  reportProgress('Arranging the Canvas')
  const canvasAccess = await world.accessTypedObject('canvas-1', signal)
  const canvas = cleanup(
    new CanvasHandle(world.getResourceRef().createRef(canvasAccess.resourceId)),
  )
  const state = await canvas.getState(signal)
  await canvas.update(
    {
      removeNodeIds: Object.keys(state.nodes ?? {}),
      setNodes: {
        title: {
          id: 'title',
          type: NodeType.TEXT,
          x: 70,
          y: 60,
          width: 310,
          height: 110,
          textContent: '# Alpine study\nA quiet place to collect ideas.',
          zIndex: 1,
        },
        image: {
          id: 'image',
          type: NodeType.WORLD_OBJECT,
          objectKey: 'files',
          viewPath: '/' + imageName,
          x: 70,
          y: 210,
          width: 430,
          height: 310,
          zIndex: 2,
        },
        note: {
          id: 'note',
          type: NodeType.TEXT,
          x: 560,
          y: 280,
          width: 230,
          height: 140,
          textContent: 'Soft light.\nCool water.\nRoom to think.',
          zIndex: 3,
        },
      },
    },
    signal,
  )

  const layout = createSingleRowObjectLayout([
    {
      id: 'canvas-pane',
      weight: 55,
      tabs: [
        {
          id: 'canvas',
          name: 'Canvas',
          objectKey: 'canvas-1',
          objectType: 'canvas',
        },
      ],
    },
    {
      id: 'files-pane',
      weight: 45,
      tabs: [
        {
          id: 'files',
          name: openImage ? 'Alpine lake' : 'Files',
          objectKey: 'files',
          objectType: 'unixfs/fs-node',
          path: openImage ? '/' + imageName : '/',
        },
      ],
    },
  ])
  reportProgress('Creating the workspace layout')
  using stage = await world.stageWorldState(signal)
  using cursor = await stage.buildStorageCursor(signal)
  const objectRef = await accessObject(
    cursor,
    undefined,
    async (block) => {
      await block.setBlock(
        {
          data: serializeObjectLayout(layout),
          blockType: ObjectLayoutTypeID,
          markDirty: true,
        },
        signal,
      )
    },
    signal,
  )
  // oxlint-disable-next-line react-doctor/server-sequential-independent-await -- Finish block storage before holding the World writer.
  using transaction = await world.getEngine().newTransaction(true, signal)
  try {
    using _object = await transaction.createObject(
      'reference-workspace',
      objectRef,
      signal,
    )
    await setObjectType(
      transaction,
      'reference-workspace',
      ObjectLayoutTypeID,
      signal,
    )
    await transaction.commit(signal)
  } finally {
    await transaction.discard()
  }
  const spaceId = setup.spaceResp.sharedObjectRef?.providerResourceRef?.id
  if (!spaceId || !setup.sessionIndex)
    throw new Error('Scenario setup did not return a Session and Space')
  return {
    path: `/u/${setup.sessionIndex}/so/${spaceId}/-/reference-workspace`,
    debug: { setup, files, canvas, spaceId, imageName },
  }
}

// canvasFiles opens a Canvas beside its reference files.
export function canvasFiles(context: AppSetupContext): Promise<AppSetupResult> {
  return seedCanvasFiles(context, false)
}

// imageContext opens the same workspace with its reference image selected.
export function imageContext(
  context: AppSetupContext,
): Promise<AppSetupResult> {
  return seedCanvasFiles(context, true)
}
