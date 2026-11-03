import { useCallback, useMemo } from 'react'
import type { StatusEntry } from '@s4wave/sdk/git/worktree.pb.js'
import { FileStatusCode } from '@s4wave/sdk/git/worktree.pb.js'
import type { GitWorktreeHandle } from '@s4wave/sdk/git/worktree.js'
import { StatusSection } from './StatusSection.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: { overflow: 'auto' },
})

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
      <div sx={stylexStyles.rule1}>
        <span sx={stylexStyles.rule2}>No changes in working directory</span>
      </div>
    )
  }

  return (
    <div sx={stylexStyles.rule3}>
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
