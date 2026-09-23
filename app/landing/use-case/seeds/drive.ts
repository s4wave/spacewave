import type {
  AppSetupContext,
  AppSetupResult,
} from '@s4wave/app/SpacewaveApp.js'
import { UNIXFS_OBJECT_KEY } from '@s4wave/core/space/world/ops/init-unixfs.js'
import { FSHandle, type TreeUploadEntry } from '@s4wave/sdk/unixfs/handle.js'

import { DRIVE_DEMO_FILES, DRIVE_DEMO_FOLDERS } from './drive-content.js'
import { quickstartDemoPath, runQuickstart } from './quickstart.js'

// driveDemo seeds a Drive Quickstart Space with sample folders and files and
// opens its file browser.
export async function driveDemo(
  context: AppSetupContext,
): Promise<AppSetupResult> {
  const { cleanup, signal, reportProgress } = context

  // Create the Space through the same Quickstart a new user runs.
  const setup = await runQuickstart(context, 'drive')

  // Write the sample tree in one upload.
  reportProgress('Adding sample files')
  const world = setup.spaceWorld
  const access = await world.accessTypedObject(UNIXFS_OBJECT_KEY, signal)
  const files = cleanup(
    new FSHandle(world.getResourceRef().createRef(access.resourceId)),
  )
  const encoder = new TextEncoder()
  const entries: TreeUploadEntry[] = [
    ...DRIVE_DEMO_FOLDERS.map((path) => ({
      kind: 'directory' as const,
      path,
    })),
    ...DRIVE_DEMO_FILES.map(({ path, content }) => {
      const data = encoder.encode(content)
      return {
        kind: 'file' as const,
        path,
        totalSize: BigInt(data.length),
        stream: new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(data)
            controller.close()
          },
        }),
      }
    }),
  ]
  await files.uploadTree(entries, undefined, signal)

  return {
    path: quickstartDemoPath(setup, 'drive', UNIXFS_OBJECT_KEY),
    debug: { setup, files },
  }
}
