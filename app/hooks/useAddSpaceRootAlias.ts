import { useCallback, useState } from 'react'
import { isDesktop, openElectronDirectory } from '@aptre/bldr'

import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import {
  SpaceRootKind,
  SpaceRootOpenMode,
} from '@s4wave/sdk/root/root.pb.js'

// useAddSpaceRootAlias opens the native directory picker and persists a root alias.
export function useAddSpaceRootAlias() {
  const rootResource = useRootResource()
  const root = rootResource.value
  const [adding, setAdding] = useState(false)
  const canAdd = isDesktop && !!root && !adding

  const add = useCallback(async () => {
    if (!isDesktop) {
      toast.error('Desktop app required', {
        description: 'State root loading is available in the desktop app.',
      })
      return
    }
    if (!root || adding) return
    setAdding(true)
    try {
      const path = await openElectronDirectory()
      if (!path) return
      await root.upsertSpaceRootAlias({
        record: {
          aliasId: aliasIdFromPath(path),
          displayName: pathBaseName(path),
          kind: SpaceRootKind.SpaceRootKind_NATIVE_DIRECTORY,
          openMode: SpaceRootOpenMode.SpaceRootOpenMode_OPEN_EXISTING,
          native: { path },
        },
      })
      toast.success('State root added', { description: path })
    } catch (err) {
      toast.error('Could not add state root', { description: String(err) })
    } finally {
      setAdding(false)
    }
  }, [adding, root])

  return { add, adding, canAdd }
}

function pathBaseName(path: string): string {
  const parts = path.split(/[\\/]/).filter(Boolean)
  return parts[parts.length - 1] ?? 'State root'
}

function aliasIdFromPath(path: string): string {
  const base = pathBaseName(path)
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return base || 'state-root'
}
