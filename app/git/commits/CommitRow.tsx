import { useCallback } from 'react'
import { LuChevronDown, LuChevronRight } from 'react-icons/lu'

import type { CommitInfo, DiffFileStat } from '@s4wave/sdk/git/repo.pb.js'

import { formatRelativeTime } from '../util.js'
import { DiffStatSection } from './DiffStatSection.js'

// CommitRowProps are props for the CommitRow component.
export interface CommitRowProps {
  commit: CommitInfo
  expanded: boolean
  onToggle: (hash: string) => void
  onCommitClick?: (hash: string) => void
  diffStat: DiffFileStat[] | undefined
  diffStatLoading: boolean
  onLoadDiffStat: (hash: string) => void
}

// CommitRow displays a single commit with expand/collapse for details.
export function CommitRow({
  commit,
  expanded,
  onToggle,
  onCommitClick,
  diffStat,
  diffStatLoading,
  onLoadDiffStat,
}: CommitRowProps) {
  const hash = commit.hash ?? ''
  const shortHash = hash.slice(0, 7)
  const message = commit.message ?? ''
  const subject = message.split('\n')[0]
  const body = message.split('\n').slice(1).join('\n').trim()
  const author = commit.authorName ?? ''
  const timeAgo = formatRelativeTime(commit.authorTimestamp)

  const handleClick = useCallback(() => {
    onToggle(hash)
    if (!expanded) {
      onLoadDiffStat(hash)
    }
  }, [hash, expanded, onToggle, onLoadDiffStat])

  const handleHashClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation()
      onCommitClick?.(hash)
    },
    [hash, onCommitClick],
  )

  return (
    <div className="border-foreground/8 border-b last:border-b-0">
      <button
        className="flex w-full items-center gap-2 px-3 py-1 text-left text-xs select-none hover:bg-white/[0.03]"
        onClick={handleClick}
      >
        {expanded ?
          <LuChevronDown className="text-foreground-alt h-3 w-3 shrink-0" />
        : <LuChevronRight className="text-foreground-alt h-3 w-3 shrink-0" />}
        <span
          className="text-brand shrink-0 cursor-pointer font-mono hover:underline"
          onClick={handleHashClick}
        >
          {shortHash}
        </span>
        <span className="text-foreground min-w-0 flex-1 truncate">
          {subject}
        </span>
        {author && (
          <span className="text-foreground-alt shrink-0">{author}</span>
        )}
        {timeAgo && (
          <span className="text-foreground-alt/70 shrink-0">{timeAgo}</span>
        )}
      </button>
      {expanded && (
        <div className="bg-black/[0.02] px-3 py-2">
          {body && (
            <pre className="text-foreground mb-2 font-mono text-xs whitespace-pre-wrap">
              {body}
            </pre>
          )}
          {(commit.parentHashes?.length ?? 0) > 0 && (
            <div className="text-foreground-alt mb-2 text-xs">
              {(commit.parentHashes?.length ?? 0) > 1 ?
                'Parents: '
              : 'Parent: '}
              {commit.parentHashes?.map((ph) => ph.slice(0, 7)).join(', ')}
            </div>
          )}
          <DiffStatSection files={diffStat} loading={diffStatLoading} />
        </div>
      )}
    </div>
  )
}
