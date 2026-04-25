import { LuFolder, LuHistory } from 'react-icons/lu'

import type { CommitInfo } from '@s4wave/sdk/git/repo.pb.js'

import { formatRelativeTime } from '../util.js'

// SelectedRefBarProps are props for the SelectedRefBar component.
export interface SelectedRefBarProps {
  lastCommit: CommitInfo | undefined
  loading: boolean
  error: Error | null
  onClickCommit?: () => void
  onClickTree?: () => void
  onClickLog?: () => void
}

// SelectedRefBar displays the commit info for the selected ref.
export function SelectedRefBar({
  lastCommit,
  loading,
  error,
  onClickCommit,
  onClickTree,
  onClickLog,
}: SelectedRefBarProps) {
  if (loading) {
    return (
      <div className="border-foreground/8 text-foreground-alt flex items-center border-b px-3 py-1 text-xs">
        Loading commit info...
      </div>
    )
  }

  if (error) {
    return (
      <div className="border-foreground/8 text-destructive flex items-center border-b px-3 py-1 text-xs">
        Failed to load commit info
      </div>
    )
  }

  if (!lastCommit) return null

  const subject = lastCommit.message?.split('\n')[0] ?? ''
  const shortHash = (lastCommit.hash ?? '').slice(0, 7)
  const author = lastCommit.authorName ?? ''
  const timeAgo = formatRelativeTime(lastCommit.authorTimestamp)

  return (
    <div className="border-foreground/8 flex items-center gap-2 border-b px-1.5 py-1 text-xs select-none">
      <button
        className="flex min-w-0 flex-1 items-center gap-2 rounded px-1.5 hover:bg-white/[0.03]"
        onClick={onClickCommit}
      >
        <span className="text-brand shrink-0 font-mono">{shortHash}</span>
        <span className="text-foreground truncate font-medium">{subject}</span>
        {author && (
          <span className="text-foreground-alt shrink-0">by {author}</span>
        )}
        {timeAgo && (
          <span className="text-foreground-alt/70 shrink-0">{timeAgo}</span>
        )}
      </button>
      {onClickTree && (
        <button
          className="text-foreground-alt hover:text-foreground shrink-0 rounded p-0.5 hover:bg-white/[0.05]"
          onClick={onClickTree}
          title="Browse files"
        >
          <LuFolder className="h-3 w-3" />
        </button>
      )}
      {onClickLog && (
        <button
          className="text-foreground-alt hover:text-foreground shrink-0 rounded p-0.5 hover:bg-white/[0.05]"
          onClick={onClickLog}
          title="View commit log"
        >
          <LuHistory className="h-3 w-3" />
        </button>
      )}
    </div>
  )
}
