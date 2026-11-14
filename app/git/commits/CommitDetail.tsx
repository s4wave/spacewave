import { LuArrowRight } from 'react-icons/lu'

import type { GitRepoHandle } from '@s4wave/sdk/git/repo.js'
import type { CommitInfo } from '@s4wave/sdk/git/repo.pb.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { FactList, FactRow } from '@s4wave/web/ui/FactList.js'

import { formatRelativeTime } from '../util.js'
import { GitDiffPatchFiles } from './GitDiffPatch.js'

// CommitDetailProps are props for the CommitDetail component.
export interface CommitDetailProps {
  handle: GitRepoHandle
  commitHash: string
  onNavigateCommit?: (hash: string) => void
}

interface CommitHeaderProps {
  commit: CommitInfo
  onNavigateCommit?: (hash: string) => void
}

/** CommitHeader renders the commit message and its hash, parent, author, and date. */
function CommitHeader({ commit, onNavigateCommit }: CommitHeaderProps) {
  // Split the message into its subject and body, and gather the facts.
  const message = commit.message ?? ''
  const subject = message.split('\n')[0]
  const body = message.split('\n').slice(1).join('\n').trim()
  const parentHashes = commit.parentHashes ?? []
  const authorTimestamp = commit.authorTimestamp
  const author = [
    commit.authorName,
    commit.authorEmail ? `<${commit.authorEmail}>` : '',
  ]
    .filter(Boolean)
    .join(' ')
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
      <FactList>
        <FactRow label="Commit" value={commit.hash ?? ''} />
        {parentHashes.map((ph, i) => (
          <FactRow
            key={ph}
            label={parentHashes.length > 1 ? `Parent ${i + 1}` : 'Parent'}
            value={ph}
            control={
              onNavigateCommit ? (
                <button
                  type="button"
                  aria-label={`Open parent ${ph.slice(0, 7)}`}
                  title="Open parent"
                  className="focus-ring text-foreground-alt hover:bg-foreground/5 hover:text-foreground flex size-6 items-center justify-center rounded [&_svg]:size-3.5"
                  onClick={() => onNavigateCommit(ph)}
                >
                  <LuArrowRight aria-hidden />
                </button>
              ) : undefined
            }
          />
        ))}
        <FactRow label="Author" value={author} kind="text" />
        {authorDate && (
          <FactRow
            label="Date"
            value={authorDate.toLocaleString()}
            sub={formatRelativeTime(authorTimestamp)}
            kind="text"
            control={null}
          />
        )}
      </FactList>
    </div>
  )
}

// CommitDetail displays a full commit detail page.
export function CommitDetail({
  handle,
  commitHash,
  onNavigateCommit,
}: CommitDetailProps) {
  // Read the commit.
  const commitResource = useResource(
    { value: handle, loading: false, error: null, retry: () => {} },
    async (h) => {
      if (!h) return null
      return (await h.getCommit(commitHash)) ?? null
    },
    [commitHash],
  )

  // Read the changed files and their line counts.
  const diffStatResource = useResource(
    { value: handle, loading: false, error: null, retry: () => {} },
    async (h) => {
      if (!h) return null
      return h.getDiffStat(commitHash)
    },
    [commitHash],
  )

  // Read the patch.
  const diffPatchResource = useResource(
    { value: handle, loading: false, error: null, retry: () => {} },
    async (h) => {
      if (!h) return null
      return h.getDiffPatch(commitHash)
    },
    [commitHash],
  )

  // Show the commit once it loads.
  const commit = commitResource.value

  // Show the load while it runs.
  if (commitResource.loading) {
    return (
      <div className="px-3 py-4">
        <div className="text-foreground-alt text-xs">Loading commit…</div>
      </div>
    )
  }

  // Show a failed load.
  if (commitResource.error) {
    return (
      <div className="px-3 py-4">
        <div className="text-destructive text-xs">
          Failed to load commit: {commitResource.error.message}
        </div>
      </div>
    )
  }

  // Show a hash that names no commit.
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
