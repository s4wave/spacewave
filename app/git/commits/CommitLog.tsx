import { useCallback, useMemo, useState } from 'react'

import type {
  CommitInfo,
  DiffFileStat,
  LogResponse,
} from '@s4wave/sdk/git/repo.pb.js'
import type { GitRepoHandle } from '@s4wave/sdk/git/repo.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { CommitRow } from './CommitRow.js'

// CommitLogProps are props for the CommitLog component.
export interface CommitLogProps {
  handle: GitRepoHandle
  refName: string
  onCommitClick?: (hash: string) => void
}

// CommitLog displays a paginated list of recent commits for the selected branch.
export function CommitLog({ handle, refName, onCommitClick }: CommitLogProps) {
  return (
    <CommitLogInner
      key={refName}
      handle={handle}
      refName={refName}
      onCommitClick={onCommitClick}
    />
  )
}

function CommitLogInner({ handle, refName, onCommitClick }: CommitLogProps) {
  const pageSize = 20
  const [extraCommits, setExtraCommits] = useState<CommitInfo[]>([])
  const [hasMore, setHasMore] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [expandedHashes, setExpandedHashes] = useState<Set<string>>(
    () => new Set(),
  )
  const [diffStats, setDiffStats] = useState<Map<string, DiffFileStat[]>>(
    () => new Map(),
  )
  const [diffStatsLoading, setDiffStatsLoading] = useState<Set<string>>(
    () => new Set(),
  )

  const initialResource = useResource(
    { value: handle, loading: false, error: null, retry: () => {} },
    async (h) => {
      if (!h) return null
      return h.log(refName, 0, pageSize)
    },
    [refName],
  )

  const commits = useMemo(
    () => [...(initialResource.value?.commits ?? []), ...extraCommits],
    [initialResource.value?.commits, extraCommits],
  )
  const loading = initialResource.loading
  const error = initialResource.error
  const initialHasMore = initialResource.value?.hasMore ?? false

  const handleShowMore = useCallback(() => {
    setLoadingMore(true)
    handle
      .log(refName, commits.length, pageSize)
      .then((resp: LogResponse) => {
        setExtraCommits((prev) => [...prev, ...(resp.commits ?? [])])
        setHasMore(resp.hasMore ?? false)
        setLoadingMore(false)
      })
      .catch(() => {
        setLoadingMore(false)
      })
  }, [handle, refName, commits.length])

  const handleToggle = useCallback((hash: string) => {
    setExpandedHashes((prev) => {
      const next = new Set(prev)
      if (next.has(hash)) {
        next.delete(hash)
      } else {
        next.add(hash)
      }
      return next
    })
  }, [])

  const handleLoadDiffStat = useCallback(
    (hash: string) => {
      if (diffStats.has(hash) || diffStatsLoading.has(hash)) return
      setDiffStatsLoading((prev) => {
        const next = new Set(prev)
        next.add(hash)
        return next
      })
      handle
        .getDiffStat(hash)
        .then((resp) => {
          setDiffStats((prev) => {
            const next = new Map(prev)
            next.set(hash, resp.files ?? [])
            return next
          })
          setDiffStatsLoading((prev) => {
            const next = new Set(prev)
            next.delete(hash)
            return next
          })
        })
        .catch(() => {
          setDiffStatsLoading((prev) => {
            const next = new Set(prev)
            next.delete(hash)
            return next
          })
        })
    },
    [handle, diffStats, diffStatsLoading],
  )

  if (loading) {
    return (
      <div className="border-foreground/8 border-t px-3 py-2">
        <LoadingInline label="Loading commits" tone="muted" size="sm" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="border-foreground/8 border-t">
        <div className="text-destructive px-3 py-2 text-xs">
          Failed to load commits: {error.message}
        </div>
      </div>
    )
  }

  if (commits.length === 0) return null

  return (
    <div className="border-foreground/8 border-t">
      <div>
        {commits.map((commit) => {
          const hash = commit.hash ?? ''
          return (
            <CommitRow
              key={hash}
              commit={commit}
              expanded={expandedHashes.has(hash)}
              onToggle={handleToggle}
              onCommitClick={onCommitClick}
              diffStat={diffStats.get(hash)}
              diffStatLoading={diffStatsLoading.has(hash)}
              onLoadDiffStat={handleLoadDiffStat}
            />
          )
        })}
      </div>
      {(hasMore || (initialHasMore && extraCommits.length === 0)) && (
        <button
          className="flex w-full items-center px-3 py-1.5 text-left text-xs hover:underline"
          onClick={handleShowMore}
          disabled={loadingMore}
        >
          {loadingMore ?
            <LoadingInline label="Loading" tone="muted" size="sm" />
          : <span className="text-brand">Show more</span>}
        </button>
      )}
    </div>
  )
}
