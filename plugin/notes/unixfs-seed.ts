import { FSHandle, type TreeUploadEntry } from '@s4wave/sdk/unixfs/handle.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

export interface SeedFile {
  path: string
  content: string
  mode?: number
}

// uploadSeedTree uploads a text-file tree into the UnixFS object root.
export async function uploadSeedTree(
  worldState: IWorldState,
  unixfsObjectKey: string,
  files: SeedFile[],
  directories?: string[],
  abortSignal?: AbortSignal,
): Promise<void> {
  const access = await worldState.accessTypedObject(unixfsObjectKey, abortSignal)
  if (!access.resourceId) {
    throw new Error('failed to access unixfs root')
  }

  const fsRef = worldState.getResourceRef().createRef(access.resourceId)
  using rootHandle = new FSHandle(fsRef)

  const encoder = new TextEncoder()
  const entries: TreeUploadEntry[] = []
  for (const dirPath of directories ?? []) {
    entries.push({ kind: 'directory', path: dirPath })
  }
  for (const file of files) {
    const data = encoder.encode(file.content)
    entries.push({
      kind: 'file',
      path: file.path,
      totalSize: BigInt(data.byteLength),
      stream: new Blob([data]).stream(),
      mode: file.mode ?? 0o644,
    })
  }
  await rootHandle.uploadTree(entries, undefined, abortSignal)
}
