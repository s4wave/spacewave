import { changePattern } from '@s4wave/sdk/world/change-feed.js'
import type {
  KeyedSet,
  KeyedSetReader,
  KeyedSetSource,
} from '@s4wave/sdk/world/entity-set.js'
import { iriToKey, isIRI, keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import { TypePred, typeIDFromIRI } from '@s4wave/sdk/world/types/types.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'

// FileReference is one object with an edge into a file. typeId is the
// object's type, or empty for an untyped object, such as a placeholder.
export interface FileReference {
  readonly typeId: string
}

// fileReferencesKey returns the key for the watch of the first limit
// referrers of file.
export function fileReferencesKey(file: string, limit: number): string {
  return JSON.stringify(['file-references', file, limit])
}

/**
 * listReferrers lists the distinct objects with an edge into file, at most
 * want + 1 of them, the extra one only showing that more exist.
 *
 * The listing selects no predicate, since every declared reference has its
 * own predicate, so the listing can not resume after a key: the World pages
 * edges past a key only within one predicate. The listing grows instead.
 * The World counts quads, and one object may point at the file by several
 * predicates, so a truncated listing with too few distinct objects is read
 * again at twice the limit.
 *
 * Rows come in referrer key order. Every Space World is cayley_kv, which
 * sorts an incoming bucket by quad text, subject first, so the order is
 * stable and a page never reshuffles. Keys carry no time, so the order is
 * not by when the reference was written.
 */
export async function listReferrers(
  tx: KeyedSetReader,
  file: string,
  want: number,
  signal: AbortSignal,
): Promise<string[]> {
  for (let limit = want + 1; ; limit *= 2) {
    const resp = await tx.listGraphEdgeBuckets([file], limit, {
      direction: GraphEdgeBucketDirection.IN,
      abortSignal: signal,
    })
    const bucket = resp.buckets?.[0]
    const keys = new Set<string>()
    for (const quad of bucket?.incoming ?? []) {
      if (!isIRI(quad.subject)) continue
      const key = iriToKey(quad.subject)
      if (key !== file) keys.add(key)
    }
    if (keys.size > want || !bucket?.incomingTruncated) {
      return [...keys].slice(0, want + 1)
    }
  }
}

/**
 * fileReferencesSource is the keyed set of the first want + 1 objects with
 * an edge into file, each with its type. A wake costs one listing when an
 * edge into the file moved, and one batched type lookup for the referrers it
 * reads. A retype of a referrer reads only that type.
 */
export function fileReferencesSource(
  file: string,
  want: number,
): KeyedSetSource<FileReference> {
  const into = changePattern({ obj: keyToIRI(file) })
  // A retype commits only the type quad, so each referrer is followed by its
  // type quads as well as its key.
  const typePred = changePattern({ predicate: TypePred }).predicate
  return {
    members: { patterns: [into] },
    list: (tx, signal) => listReferrers(tx, file, want, signal),
    names: (keys) => ({
      keys,
      patterns: keys.map((key) =>
        changePattern({ subject: keyToIRI(key), predicate: TypePred }),
      ),
    }),
    route: (change, members) => {
      const quads = change.quads ?? []
      const retyped = quads.flatMap((quad) =>
        quad.predicate === typePred &&
        isIRI(quad.subject) &&
        members.has(iriToKey(quad.subject))
          ? [iriToKey(quad.subject)]
          : [],
      )
      return {
        list: quads.some((quad) => quad.obj === into.obj),
        keys: [...(change.keys ?? []), ...retyped],
      }
    },
    read: async function* (tx, keys, signal) {
      if (keys.length === 0) return
      const types = await tx.listGraphEdgeBuckets([...keys], 1, {
        predicate: TypePred,
        direction: GraphEdgeBucketDirection.OUT,
        abortSignal: signal,
      })
      const typeIds = new Map(
        (types.buckets ?? []).map((bucket) => [
          bucket.originObjectKey ?? '',
          typeIDFromIRI(bucket.outgoing?.[0]?.obj),
        ]),
      )
      yield {
        rows: keys.map(
          (key) => [key, { typeId: typeIds.get(key) ?? '' }] as const,
        ),
      }
    },
  }
}

// FileReferencesView is what the references strip shows, one state of
// FileReferences.tla: the read under way, its failure with a retry, or the
// page as read. Read authority is per Space, so every referrer in a World
// the reader can open is one the reader can read. An empty page is the
// spec's "none" and any other page is its "found".
export type FileReferencesView =
  | { readonly load: 'loading' }
  | {
      readonly load: 'failed'
      readonly message: string
      readonly retry: () => void
    }
  | {
      readonly load: 'ready'
      // refs are the page's referrers, in list order.
      readonly refs: readonly (readonly [string, FileReference])[]
      // hasMore is set when more referrers exist past the page.
      readonly hasMore: boolean
    }

// fileReferencesView is what the strip shows of set, a read of want + 1
// referrers. The extra one is read only to learn that more exist and is
// never drawn. A referrer whose type has not been read yet is left out.
export function fileReferencesView(
  set: {
    value?: KeyedSet<FileReference> | null
    error?: Error | null
    retry: () => void
  },
  want: number,
): FileReferencesView {
  if (set.error) {
    return {
      load: 'failed',
      message: `Could not read references: ${set.error.message}`,
      retry: set.retry,
    }
  }
  const value = set.value
  if (!value) return { load: 'loading' }
  return {
    load: 'ready',
    refs: value.keys.slice(0, want).flatMap((key) => {
      const row = value.rows.get(key)
      return row ? [[key, row] as const] : []
    }),
    hasMore: value.keys.length > want,
  }
}
