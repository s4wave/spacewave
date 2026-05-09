type WebkitFileEntry = {
  isDirectory: false
  isFile: true
  file: (
    success: (file: File) => void,
    error?: (err: DOMException) => void,
  ) => void
  fullPath: string
  name: string
}

type WebkitDirectoryReader = {
  readEntries: (
    success: (entries: WebkitEntry[]) => void,
    error?: (err: DOMException) => void,
  ) => void
}

type WebkitDirectoryEntry = {
  isDirectory: true
  isFile: false
  createReader: () => WebkitDirectoryReader
  fullPath: string
  name: string
}

type WebkitEntry = WebkitFileEntry | WebkitDirectoryEntry

export interface NativeUploadSelection {
  files: File[]
  directories: string[]
}

// extractNativeUploadSelection collects dropped files and directories while
// preserving nested relative paths when the browser exposes the webkit entry API.
export async function extractNativeUploadSelection(
  dataTransfer: DataTransfer,
): Promise<NativeUploadSelection> {
  const roots = getNativeUploadRoots(dataTransfer)
  if (roots.length === 0) {
    return { files: Array.from(dataTransfer.files ?? []), directories: [] }
  }

  const selections = await Promise.all(roots.map(walkNativeUploadEntry))
  return mergeNativeUploadSelections(selections)
}

function getNativeUploadRoots(dataTransfer: DataTransfer): WebkitEntry[] {
  const roots: WebkitEntry[] = []
  for (const item of Array.from(dataTransfer.items ?? [])) {
    const entry = (item.webkitGetAsEntry?.() ?? null) as WebkitEntry | null
    if (entry !== null) {
      roots.push(entry)
    }
  }
  return roots
}

async function walkNativeUploadEntry(
  entry: WebkitEntry,
): Promise<NativeUploadSelection> {
  const relPath = trimNativeEntryPath(entry.fullPath || entry.name)
  if (entry.isDirectory) {
    const selection: NativeUploadSelection = {
      files: [],
      directories: relPath ? [relPath] : [],
    }
    const reader = entry.createReader()
    while (true) {
      // eslint-disable-next-line react-doctor/async-await-in-loop
      const entries = await readNativeDirectoryEntries(reader)
      if (entries.length === 0) {
        return selection
      }
      mergeNativeUploadSelection(
        selection,
        mergeNativeUploadSelections(
          await Promise.all(entries.map(walkNativeUploadEntry)),
        ),
      )
    }
  }

  const file = await readNativeFile(entry)
  return {
    files: [withRelativePath(file, relPath || file.name)],
    directories: [],
  }
}

function mergeNativeUploadSelections(
  selections: NativeUploadSelection[],
): NativeUploadSelection {
  const merged: NativeUploadSelection = { files: [], directories: [] }
  for (const selection of selections) {
    mergeNativeUploadSelection(merged, selection)
  }
  return merged
}

function mergeNativeUploadSelection(
  target: NativeUploadSelection,
  source: NativeUploadSelection,
): void {
  target.files.push(...source.files)
  target.directories.push(...source.directories)
}

function readNativeDirectoryEntries(
  reader: WebkitDirectoryReader,
): Promise<WebkitEntry[]> {
  return new Promise((resolve, reject) => {
    reader.readEntries(resolve, reject)
  })
}

function readNativeFile(entry: WebkitFileEntry): Promise<File> {
  return new Promise((resolve, reject) => {
    entry.file(resolve, reject)
  })
}

function withRelativePath(file: File, relativePath: string): File {
  const next = new File([file], file.name, {
    type: file.type,
    lastModified: file.lastModified,
  })
  Object.defineProperty(next, 'webkitRelativePath', {
    configurable: true,
    value: relativePath,
  })
  return next
}

function trimNativeEntryPath(path: string): string {
  return path.replace(/^\/+/, '').replace(/\/+$/, '')
}
