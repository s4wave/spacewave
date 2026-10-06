import type { ReactNode } from 'react'

import type { GitRepoHandle } from '@s4wave/sdk/git/repo.js'
import type { CommitInfo } from '@s4wave/sdk/git/repo.pb.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { formatRelativeTime } from '../util.js'
import { GitDiffPatchFiles } from './GitDiffPatch.js'

// CommitDetailProps are props for the CommitDetail component.
export interface CommitDetailProps {
  handle: GitRepoHandle
  commitHash: string
  onNavigateCommit?: (hash: string) => void
}

interface CommitFieldProps {
  label: string
  children: ReactNode
}

/** CommitField renders one labeled row of commit metadata. */
function CommitField({ label, children }: CommitFieldProps) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-foreground-alt w-16 shrink-0">{label}</span>
      {children}
    </div>
  )
}

interface CommitHeaderProps {
  commit: CommitInfo
  onNavigateCommit?: (hash: string) => void
}

/** CommitHeader renders the commit message and its hash, parent, author, and date. */
function CommitHeader({ commit, onNavigateCommit }: CommitHeaderProps) {
  const message = commit.message ?? ''
  const subject = message.split('\n')[0]
  const body = message.split('\n').slice(1).join('\n').trim()
  const parentHashes = commit.parentHashes ?? []
  const authorTimestamp = commit.authorTimestamp
  const authorDate = authorTimestamp
    ? new Date(Number(authorTimestamp) * 1000)
    : null

  return (
    <div className="border-foreground/8 border-b p-3">
      <div className="text-foreground mb-2 text-xs font-medium">{subject}</div>
      {body && (
        <pre className="text-foreground mb-3 font-mono text-xs whitespace-pre-wrap">
          {body}
        </pre>
      )}
      <div className="flex flex-col gap-1 text-xs">
        <CommitField label="Commit">
          <span className="text-foreground font-mono">{commit.hash ?? ''}</span>
        </CommitField>
        {parentHashes.length > 0 && (
          <CommitField label={parentHashes.length > 1 ? 'Parents' : 'Parent'}>
            <span className="flex gap-1.5">
              {parentHashes.map((ph) => (
                <button
                  type="button"
                  key={ph}
                  className="text-brand font-mono hover:underline"
                  onClick={() => onNavigateCommit?.(ph)}
                >
                  {ph.slice(0, 7)}
                </button>
              ))}
            </span>
          </CommitField>
        )}
        <CommitField label="Author">
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
        </CommitField>
        {authorDate && (
          <CommitField label="Date">
            <span className="text-foreground">
              {authorDate.toLocaleString()}
            </span>
            <span className="text-foreground-alt/70">
              ({formatRelativeTime(authorTimestamp)})
            </span>
          </CommitField>
        )}
      </div>
    </div>
  )
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

  const diffPatchResource = useResource(
    { value: handle, loading: false, error: null, retry: () => {} },
    async (h) => {
      if (!h) return null
      return h.getDiffPatch(commitHash)
    },
    [commitHash],
  )

  const commit = commitResource.value

  if (commitResource.loading) {
    return (
      <div className="px-3 py-4">
        <div className="text-foreground-alt text-xs">Loading commit…</div>
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

  return (
    <div className="min-h-0 flex-1 overflow-auto">
      <CommitHeader commit={commit} onNavigateCommit={onNavigateCommit} />
      <div className="p-3">
        <GitDiffPatchFiles
          files={diffStatResource.value?.files}
          patch={diffPatchResource.value?.patch}
          loading={diffStatResource.loading || diffPatchResource.loading}
          truncated={diffPatchResource.value?.truncated}
          totalBytes={diffPatchResource.value?.totalBytes}
          limitBytes={diffPatchResource.value?.limitBytes}
          error={diffPatchResource.error}
        />
      </div>
    </div>
  )
}
