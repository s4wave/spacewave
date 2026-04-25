import { useCallback, useMemo } from 'react'
import type { StatusEntry } from '@s4wave/sdk/git/worktree.pb.js'
import { FileStatusCode } from '@s4wave/sdk/git/worktree.pb.js'
import type { GitWorktreeHandle } from '@s4wave/sdk/git/worktree.js'
import { StatusSection } from './StatusSection.js'

// ChangesViewProps are props for the ChangesView component.
export interface ChangesViewProps {
  entries: StatusEntry[]
  handle: GitWorktreeHandle
  onFileClick?: (path: string) => void
  onRefresh?: () => void
}

// ChangesView renders a magit-style view of staged, unstaged, and untracked changes.
export function ChangesView({
  entries,
  handle,
  onFileClick,
  onRefresh,
}: ChangesViewProps) {
  const staged = useMemo(
    () =>
      entries.filter(
        (e) =>
          e.stagingStatus !== undefined &&
          e.stagingStatus !== FileStatusCode.UNMODIFIED &&
          e.stagingStatus !== FileStatusCode.UNTRACKED,
      ),
    [entries],
  )

  const unstaged = useMemo(
    () =>
      entries.filter(
        (e) =>
          e.worktreeStatus !== undefined &&
          e.worktreeStatus !== FileStatusCode.UNMODIFIED &&
          e.worktreeStatus !== FileStatusCode.UNTRACKED,
      ),
    [entries],
  )

  const untracked = useMemo(
    () =>
      entries.filter(
        (e) =>
          e.worktreeStatus === FileStatusCode.UNTRACKED ||
          e.stagingStatus === FileStatusCode.UNTRACKED,
      ),
    [entries],
  )

  const handleStage = useCallback(
    (paths: string[]) => {
      void handle.stageFiles(paths).then(() => onRefresh?.())
    },
    [handle, onRefresh],
  )

  const handleUnstage = useCallback(
    (paths: string[]) => {
      void handle.unstageFiles(paths).then(() => onRefresh?.())
    },
    [handle, onRefresh],
  )

  if (entries.length === 0) {
    return (
      <div className="flex h-full items-center justify-center">
        <span className="text-foreground-alt text-xs">
          No changes in working directory
        </span>
      </div>
    )
  }

  return (
    <div className="overflow-auto">
      <StatusSection
        title="Staged Changes"
        entries={staged}
        statusField="staging"
        actionLabel="Unstage"
        onAction={handleUnstage}
        onFileClick={onFileClick}
      />
      <StatusSection
        title="Changes"
        entries={unstaged}
        statusField="worktree"
        actionLabel="Stage"
        onAction={handleStage}
        onFileClick={onFileClick}
      />
      <StatusSection
        title="Untracked Files"
        entries={untracked}
        statusField="worktree"
        actionLabel="Stage"
        onAction={handleStage}
        onFileClick={onFileClick}
      />
    </div>
  )
}
