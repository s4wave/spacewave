import { useState } from 'react'
import { LuCircleAlert } from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { readKeyedSet, watchKeyedSet } from '@s4wave/sdk/world/entity-set.js'
import {
  WorldStateResource,
  type IWorldState,
} from '@s4wave/sdk/world/world-state.js'
import { ObjectLink } from '@s4wave/web/object/ObjectLink.js'
import { getObjectTypeLabel } from '@s4wave/web/space/object-tree.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'

import {
  fileReferencesKey,
  fileReferencesSource,
  fileReferencesView,
  type FileReferencesView,
} from './file-references.js'

// fileReferencesPage is how many referrers each press of More adds.
const fileReferencesPage = 8

// useFileReferences follows the first want + 1 objects with an edge into
// file. A live World shares one watch per file and limit among its viewers
// and wakes it only when an edge into the file or a referrer's type moves; a
// World snapshot is read once.
function useFileReferences(
  worldState: Resource<IWorldState>,
  file: string,
  want: number,
) {
  const key = fileReferencesKey(file, want)
  return useStreamingResource(
    worldState,
    async function* (world, signal) {
      const source = fileReferencesSource(file, want)
      const engine = world.getEngine?.()
      if (engine) {
        yield* engine.watchShared(
          key,
          (shared) => watchKeyedSet(engine, source, shared),
          signal,
        )
        return
      }
      if (!(world instanceof WorldStateResource)) {
        throw new Error('This World can not list references.')
      }
      yield await readKeyedSet(world, source, signal)
    },
    [key],
    { reconnect: true, keepWhileLoading: true },
  )
}

export interface FileReferencesProps {
  // worldState is the World holding the file.
  worldState: Resource<IWorldState>
  // fileKey is the object key of the file.
  fileKey: string
}

/**
 * FileReferences is the references strip under a UnixFS file: the objects
 * with an edge into the file, each opening its own viewer, in referrer key
 * order. More reads one page further; a reference written while the file is
 * open appears without a press.
 */
export function FileReferences({ worldState, fileKey }: FileReferencesProps) {
  // pages is how many pages the reader asked for; a new file remounts.
  const [pages, setPages] = useState(1)
  const want = pages * fileReferencesPage
  const set = useFileReferences(worldState, fileKey, want)
  return (
    <FileReferencesStrip
      view={fileReferencesView(set, want)}
      onMore={() => setPages((n) => n + 1)}
    />
  )
}

// stripClass is the strip's one line, a fact row tall in every state, so
// the file above it never moves when the read settles.
const stripClass =
  'viewer-inset-x border-foreground/8 h-viewer-fact-row flex shrink-0 items-center gap-2 border-t text-xs @pane-low:hidden'

/**
 * FileReferencesStrip draws one state of FileReferences.tla. Loading and an
 * empty page show the spec's hint; a failure shows Retry; a page shows its
 * referrers as links, an untyped one with no type label, and More while
 * more exist.
 */
export function FileReferencesStrip({
  view,
  onMore,
}: {
  view: FileReferencesView
  onMore: () => void
}) {
  if (view.load === 'loading') {
    return (
      <p
        role="status"
        aria-label="References"
        className={cn(stripClass, 'text-foreground-alt')}
      >
        Reading references
      </p>
    )
  }
  if (view.load === 'failed') {
    return (
      <div
        role="alert"
        aria-label="References"
        className={cn(stripClass, 'bg-destructive/6 text-foreground')}
      >
        <LuCircleAlert
          aria-hidden
          className="text-destructive size-3.5 shrink-0"
        />
        <span className="min-w-0 flex-1 truncate">{view.message}</span>
        <Button
          variant="outline"
          size="xs"
          className="shrink-0"
          onClick={view.retry}
        >
          Retry
        </Button>
      </div>
    )
  }
  if (view.refs.length === 0) {
    return (
      <p
        aria-label="References"
        className={cn(stripClass, 'text-foreground-alt/70')}
      >
        Nothing references this file
      </p>
    )
  }
  return (
    <section aria-label="References" className={stripClass}>
      <span className="text-foreground-alt shrink-0">Referenced by</span>
      <ul className="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto">
        {view.refs.map(([key, ref]) => (
          <li key={key} className="shrink-0">
            <ObjectLink
              objectKey={key}
              objectType={ref.typeId || undefined}
              kind={referrerKind(ref.typeId)}
              compact
            />
          </li>
        ))}
      </ul>
      {view.hasMore ? (
        <Button variant="ghost" size="xs" className="shrink-0" onClick={onMore}>
          More
        </Button>
      ) : null}
    </section>
  )
}

// referrerKind names a referrer's type in a few words: its registered label, or
// the last segment of a type ID no plugin registers, such as "observation" for
// "glados/observation". An untyped referrer has no kind.
function referrerKind(typeId: string): string {
  if (!typeId) return ''
  const label = getObjectTypeLabel(typeId)
  return label === typeId ? (typeId.split('/').at(-1) ?? typeId) : label
}
