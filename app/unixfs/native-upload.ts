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
  const items = Array.from(dataTransfer.items ?? [])
  const roots = items
    .map((item) => (item.webkitGetAsEntry?.() ?? null) as WebkitEntry | null)
    .filter((entry): entry is WebkitEntry => entry !== null)
  if (roots.length === 0) {
    return { files: Array.from(dataTransfer.files ?? []), directories: [] }
  }

  const selection: NativeUploadSelection = { files: [], directories: [] }
  for (const root of roots) {
    await walkNativeUploadEntry(root, selection)
  }
  return selection
}

async function walkNativeUploadEntry(
  entry: WebkitEntry,
  selection: NativeUploadSelection,
): Promise<void> {
  const relPath = trimNativeEntryPath(entry.fullPath || entry.name)
  if (entry.isDirectory) {
    if (relPath) {
      selection.directories.push(relPath)
    }
    const reader = entry.createReader()
    while (true) {
      const entries = await readNativeDirectoryEntries(reader)
      if (entries.length === 0) {
        return
      }
      for (const child of entries) {
        await walkNativeUploadEntry(child, selection)
      }
    }
  }

  const file = await readNativeFile(entry)
  selection.files.push(withRelativePath(file, relPath || file.name))
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
