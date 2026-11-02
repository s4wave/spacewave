import { useCallback, useState } from 'react'
import { LuChevronDown, LuChevronRight } from '@s4wave/web/images/icons.js'
import { cn } from '@s4wave/web/style/utils.js'
import type { StatusEntry } from '@s4wave/sdk/git/worktree.pb.js'
import { FileStatusCode } from '@s4wave/sdk/git/worktree.pb.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import { colors as themeColors } from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 3%, transparent)`,
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
})

// statusCodeToLetter maps a FileStatusCode to a display letter.
function statusCodeToLetter(code: FileStatusCode): string {
  switch (code) {
    case FileStatusCode.ADDED:
      return 'A'
    case FileStatusCode.MODIFIED:
      return 'M'
    case FileStatusCode.DELETED:
      return 'D'
    case FileStatusCode.RENAMED:
      return 'R'
    case FileStatusCode.COPIED:
      return 'C'
    case FileStatusCode.UNTRACKED:
      return '?'
    case FileStatusCode.UPDATED_BUT_UNMERGED:
      return 'U'
    default:
      return ' '
  }
}

// statusCodeToColor maps a FileStatusCode to a tailwind text color class.
function statusCodeToColor(code: FileStatusCode): string {
  switch (code) {
    case FileStatusCode.ADDED:
      return 'text-green-400'
    case FileStatusCode.MODIFIED:
      return 'text-yellow-400'
    case FileStatusCode.DELETED:
      return 'text-red-400'
    case FileStatusCode.RENAMED:
      return 'text-blue-400'
    case FileStatusCode.COPIED:
      return 'text-blue-400'
    case FileStatusCode.UNTRACKED:
      return 'text-foreground-alt'
    case FileStatusCode.UPDATED_BUT_UNMERGED:
      return 'text-orange-400'
    default:
      return 'text-foreground-alt'
  }
}

// StatusSectionProps are props for the StatusSection component.
export interface StatusSectionProps {
  title: string
  entries: StatusEntry[]
  statusField: 'staging' | 'worktree'
  actionLabel: string
  onAction: (paths: string[]) => void
  onFileClick?: (path: string) => void
}

// StatusSection renders a collapsible section of file status entries.
export function StatusSection({
  title,
  entries,
  statusField,
  actionLabel,
  onAction,
  onFileClick,
}: StatusSectionProps) {
  const [collapsed, setCollapsed] = useState(false)

  const toggle = useCallback(() => {
    setCollapsed((v) => !v)
  }, [])

  if (entries.length === 0) return null

  return (
    <div className="border-foreground/8 border-b">
      <button
        type="button"
        className="text-foreground flex w-full items-center gap-1 px-3 py-1.5 text-xs font-medium select-none hover:bg-white/[0.03]"
        onClick={toggle}
      >
        {collapsed ? (
          <LuChevronRight sx={stylexStyles.rule1} />
        ) : (
          <LuChevronDown sx={stylexStyles.rule1} />
        )}
        <span>
          {title} ({entries.length})
        </span>
      </button>
      {!collapsed && (
        <div className="pb-1">
          {entries.map((entry) => {
            const path = entry.filePath ?? ''
            const code =
              statusField === 'staging'
                ? (entry.stagingStatus ?? FileStatusCode.UNMODIFIED)
                : (entry.worktreeStatus ?? FileStatusCode.UNMODIFIED)
            const letter = statusCodeToLetter(code)
            const color = statusCodeToColor(code)

            return (
              <div key={path} sx={[stylexStyles2.rule1, groupMarker]}>
                <span
                  className={cn('w-4 shrink-0 text-center font-mono', color)}
                >
                  {letter}
                </span>
                <button
                  type="button"
                  className="text-foreground min-w-0 flex-1 truncate text-left text-xs"
                  onClick={() => onFileClick?.(path)}
                >
                  {path}
                </button>
                <button
                  type="button"
                  className="text-brand hover:text-brand/80 shrink-0 text-xs opacity-0 transition-opacity group-hover:opacity-100"
                  onClick={() => onAction([path])}
                >
                  {actionLabel}
                </button>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

export { statusCodeToLetter, statusCodeToColor }
