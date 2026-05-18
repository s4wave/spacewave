import { MknodType } from '@s4wave/sdk/unixfs/handle.pb.js'
import { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

export interface SeedFile {
  path: string
  content: string
  mode?: number
}

function splitSeedPath(path: string): string[] {
  const parts = path.split('/').filter((part) => part !== '' && part !== '.')
  if (parts.length === 0 || parts.some((part) => part === '..')) {
    throw new Error('invalid seed path: ' + path)
  }
  return parts
}

async function ensureSeedDirectory(
  rootHandle: FSHandle,
  ensuredDirs: Set<string>,
  parts: string[],
  abortSignal?: AbortSignal,
): Promise<void> {
  if (parts.length === 0) return

  const key = parts.join('/')
  if (ensuredDirs.has(key)) return

  await rootHandle.mkdirAll(parts, 0o755, abortSignal)
  ensuredDirs.add(key)
}

async function writeSeedFile(
  rootHandle: FSHandle,
  ensuredDirs: Set<string>,
  file: SeedFile,
  data: Uint8Array,
  abortSignal?: AbortSignal,
): Promise<void> {
  const parts = splitSeedPath(file.path)
  const parentParts = parts.slice(0, -1)
  const name = parts[parts.length - 1]

  await ensureSeedDirectory(rootHandle, ensuredDirs, parentParts, abortSignal)

  let parentHandle: FSHandle | undefined
  let fileHandle: FSHandle | undefined
  try {
    const dirHandle =
      parentParts.length === 0
        ? rootHandle
        : (parentHandle = (
            await rootHandle.lookupPath(parentParts.join('/'), abortSignal)
          ).handle)

    await dirHandle.mknod(
      [name],
      MknodType.FILE,
      file.mode ?? 0o644,
      false,
      abortSignal,
    )
    fileHandle = await dirHandle.lookup(name, abortSignal)
    await fileHandle.writeAt(0n, data, abortSignal)
  } finally {
    fileHandle?.release()
    parentHandle?.release()
  }
}

async function ensureSeedDirectories(
  rootHandle: FSHandle,
  ensuredDirs: Set<string>,
  directories: string[],
  idx: number,
  abortSignal?: AbortSignal,
): Promise<void> {
  if (idx >= directories.length) return
  await ensureSeedDirectory(
    rootHandle,
    ensuredDirs,
    splitSeedPath(directories[idx]),
    abortSignal,
  )
  await ensureSeedDirectories(
    rootHandle,
    ensuredDirs,
    directories,
    idx + 1,
    abortSignal,
  )
}

async function writeSeedFiles(
  rootHandle: FSHandle,
  ensuredDirs: Set<string>,
  files: SeedFile[],
  idx: number,
  encoder: TextEncoder,
  abortSignal?: AbortSignal,
): Promise<void> {
  if (idx >= files.length) return
  const file = files[idx]
  await writeSeedFile(
    rootHandle,
    ensuredDirs,
    file,
    encoder.encode(file.content),
    abortSignal,
  )
  await writeSeedFiles(
    rootHandle,
    ensuredDirs,
    files,
    idx + 1,
    encoder,
    abortSignal,
  )
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
  const ensuredDirs = new Set<string>()
  await ensureSeedDirectories(
    rootHandle,
    ensuredDirs,
    directories ?? [],
    0,
    abortSignal,
  )
  await writeSeedFiles(rootHandle, ensuredDirs, files, 0, encoder, abortSignal)
}
