import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { getUnixFSDirEntryKind } from '@s4wave/sdk/unixfs/file-kind.js'
import { normalizeUnixFSLookupPath } from '@s4wave/sdk/unixfs/path.js'
import { getMimeType } from '@s4wave/web/hooks/useUnixFSHandle.js'
import { createSubItemQueryId } from '@s4wave/web/command/sub-item-navigation.js'
import type { SubItem } from '@s4wave/web/command/CommandContext.js'

function splitFindFileQuery(query: string): {
  directory: string
  fragment: string
} {
  const normalized = query.trim().replace(/^\/+/, '')
  const slash = normalized.lastIndexOf('/')
  return slash < 0
    ? { directory: '', fragment: normalized }
    : {
        directory: normalizeUnixFSLookupPath(`/${normalized.slice(0, slash)}`),
        fragment: normalized.slice(slash + 1),
      }
}

async function listDirectory(
  handle: FSHandle,
  directory: string,
  fragment: string,
  signal: AbortSignal,
): Promise<SubItem[]> {
  const entries = await handle.readdirAll(0n, signal)
  const normalizedFragment = fragment.toLowerCase()
  return entries.flatMap((entry) => {
    const name = entry.name ?? ''
    if (!name || !name.toLowerCase().includes(normalizedFragment)) return []
    const path = directory ? `${directory}/${name}` : name
    if (getUnixFSDirEntryKind(entry) === 'directory') {
      const nextQuery = `${path}/`
      return [
        {
          id: createSubItemQueryId(nextQuery),
          label: `/${nextQuery}`,
          description: 'Directory',
          iconName: 'LuFolder',
        },
      ]
    }
    if (!getMimeType(name).startsWith('image/')) return []
    return [
      {
        id: `/${path}`,
        label: `/${path}`,
        description: 'Image',
        iconName: 'LuImage',
      },
    ]
  })
}

// getUnixFSImageSubItems completes image paths against the queried directory.
export async function getUnixFSImageSubItems(
  root: FSHandle,
  query: string,
  signal: AbortSignal,
): Promise<SubItem[]> {
  const { directory, fragment } = splitFindFileQuery(query)
  if (!directory) return listDirectory(root, directory, fragment, signal)

  using directoryHandle = (await root.lookupPath(directory, signal)).handle
  return listDirectory(directoryHandle, directory, fragment, signal)
}
