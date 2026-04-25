import type { GitRepoHandle } from '@s4wave/sdk/git/repo.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { formatRelativeTime } from '../util.js'
import { DiffStatSection } from './DiffStatSection.js'

// CommitDetailProps are props for the CommitDetail component.
export interface CommitDetailProps {
  handle: GitRepoHandle
  commitHash: string
  onNavigateCommit?: (hash: string) => void
}

// CommitDetail displays a full commit detail page.
export function CommitDetail({
  handle,
  commitHash,
  onNavigateCommit,
}: CommitDetailProps) {
  const commitResource = useResource(
    { value: handle, loading: false, error: null, retry: () => {} },
    async (h) => {
      if (!h) return null
      return (await h.getCommit(commitHash)) ?? null
    },
    [commitHash],
  )

  const diffStatResource = useResource(
    { value: handle, loading: false, error: null, retry: () => {} },
    async (h) => {
      if (!h) return null
      return h.getDiffStat(commitHash)
    },
    [commitHash],
  )

  const commit = commitResource.value

  if (commitResource.loading) {
    return (
      <div className="px-3 py-4">
        <div className="text-foreground-alt text-xs">Loading commit...</div>
      </div>
    )
  }

  if (commitResource.error) {
    return (
      <div className="px-3 py-4">
        <div className="text-destructive text-xs">
          Failed to load commit: {commitResource.error.message}
        </div>
      </div>
    )
  }

  if (!commit) {
    return (
      <div className="px-3 py-4">
        <div className="text-foreground-alt text-xs">Commit not found</div>
      </div>
    )
  }

  const message = commit.message ?? ''
  const subject = message.split('\n')[0]
  const body = message.split('\n').slice(1).join('\n').trim()
  const fullHash = commit.hash ?? ''
  const authorDate =
    commit.authorTimestamp ?
      new Date(Number(commit.authorTimestamp) * 1000)
    : null

  return (
    <div className="min-h-0 flex-1 overflow-auto">
      <div className="border-foreground/8 border-b px-3 py-3">
        <div className="text-foreground mb-2 text-xs font-medium">
          {subject}
        </div>
        {body && (
          <pre className="text-foreground mb-3 font-mono text-xs whitespace-pre-wrap">
            {body}
          </pre>
        )}
        <div className="flex flex-col gap-1 text-xs">
          <div className="flex items-center gap-2">
            <span className="text-foreground-alt w-16 shrink-0">Commit</span>
            <span className="text-foreground font-mono">{fullHash}</span>
          </div>
          {(commit.parentHashes?.length ?? 0) > 0 && (
            <div className="flex items-center gap-2">
              <span className="text-foreground-alt w-16 shrink-0">
                {(commit.parentHashes?.length ?? 0) > 1 ? 'Parents' : 'Parent'}
              </span>
              <span className="flex gap-1.5">
                {commit.parentHashes?.map((ph) => (
                  <button
                    key={ph}
                    className="text-brand font-mono hover:underline"
                    onClick={() => onNavigateCommit?.(ph)}
                  >
                    {ph.slice(0, 7)}
                  </button>
                ))}
              </span>
            </div>
          )}
          <div className="flex items-center gap-2">
            <span className="text-foreground-alt w-16 shrink-0">Author</span>
            <span className="text-foreground">
              {commit.authorName}
              {commit.authorEmail && (
                <span className="text-foreground-alt ml-1">
                  {'<'}
                  {commit.authorEmail}
                  {'>'}
                </span>
              )}
            </span>
          </div>
          {authorDate && (
            <div className="flex items-center gap-2">
              <span className="text-foreground-alt w-16 shrink-0">Date</span>
              <span className="text-foreground">
                {authorDate.toLocaleString()}
              </span>
              <span className="text-foreground-alt/70">
                (
                {formatRelativeTime(
                  commit.authorTimestamp ?
                    BigInt(commit.authorTimestamp)
                  : undefined,
                )}
                )
              </span>
            </div>
          )}
        </div>
      </div>
      <div className="px-3 py-2">
        <DiffStatSection
          files={diffStatResource.value?.files}
          loading={diffStatResource.loading}
        />
      </div>
    </div>
  )
}
