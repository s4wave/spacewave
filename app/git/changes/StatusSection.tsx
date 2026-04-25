import { useCallback, useState } from 'react'
import { LuChevronDown, LuChevronRight } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import type { StatusEntry } from '@s4wave/sdk/git/worktree.pb.js'
import { FileStatusCode } from '@s4wave/sdk/git/worktree.pb.js'

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
        className="text-foreground flex w-full items-center gap-1 px-3 py-1.5 text-xs font-medium select-none hover:bg-white/[0.03]"
        onClick={toggle}
      >
        {collapsed ?
          <LuChevronRight className="h-3.5 w-3.5 shrink-0" />
        : <LuChevronDown className="h-3.5 w-3.5 shrink-0" />}
        <span>
          {title} ({entries.length})
        </span>
      </button>
      {!collapsed && (
        <div className="pb-1">
          {entries.map((entry) => {
            const path = entry.filePath ?? ''
            const code =
              statusField === 'staging' ?
                (entry.stagingStatus ?? FileStatusCode.UNMODIFIED)
              : (entry.worktreeStatus ?? FileStatusCode.UNMODIFIED)
            const letter = statusCodeToLetter(code)
            const color = statusCodeToColor(code)

            return (
              <div
                key={path}
                className="group flex items-center gap-2 px-3 py-0.5 hover:bg-white/[0.03]"
              >
                <span
                  className={cn('w-4 shrink-0 text-center font-mono', color)}
                >
                  {letter}
                </span>
                <button
                  className="text-foreground min-w-0 flex-1 truncate text-left text-xs"
                  onClick={() => onFileClick?.(path)}
                >
                  {path}
                </button>
                <button
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
