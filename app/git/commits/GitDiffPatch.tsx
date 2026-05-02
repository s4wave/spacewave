import { useCallback, useMemo, useState } from 'react'
import { LuChevronDown, LuChevronRight } from 'react-icons/lu'

import { PatchDiff, type PatchDiffProps } from '@pierre/diffs/react'

import type { DiffFileStat } from '@s4wave/sdk/git/repo.pb.js'
import { cn } from '@s4wave/web/style/utils.js'

// GitDiffPatchProps are props for the GitDiffPatch component.
export interface GitDiffPatchProps {
  patch: string | undefined
  className?: string
}

// GitDiffPatch renders a unified git patch with Spacewave styling.
export function GitDiffPatch({ patch, className }: GitDiffPatchProps) {
  const options = useMemo<NonNullable<PatchDiffProps<undefined>['options']>>(
    () => ({
      diffStyle: 'unified',
      diffIndicators: 'classic',
      hunkSeparators: 'line-info-basic',
      overflow: 'wrap',
      themeType: 'dark',
      disableFileHeader: true,
    }),
    [],
  )

  if (!patch) return null

  return (
    <div className={cn('overflow-hidden', className)}>
      <PatchDiff
        patch={patch}
        disableWorkerPool
        className="git-diff-patch"
        options={options}
      />
    </div>
  )
}

// GitDiffPatchFilesProps are props for the GitDiffPatchFiles component.
export interface GitDiffPatchFilesProps {
  files: DiffFileStat[] | undefined
  patch: string | undefined
  loading: boolean
  error?: Error | null
}

// GitDiffPatchFiles renders collapsible per-file patch sections.
export function GitDiffPatchFiles({
  files,
  patch,
  loading,
  error,
}: GitDiffPatchFilesProps) {
  const sections = useMemo(() => splitPatchFiles(patch, files), [patch, files])
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set())

  const toggle = useCallback((path: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(path)) {
        next.delete(path)
        return next
      }
      next.add(path)
      return next
    })
  }, [])

  if (loading) {
    return (
      <div className="text-foreground-alt px-3 py-2 text-xs">
        Loading diff...
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-destructive px-3 py-2 text-xs">
        Failed to load diff: {error.message}
      </div>
    )
  }

  if (sections.length === 0) {
    return (
      <div className="text-foreground-alt/70 px-3 py-2 text-xs">
        No diff to display
      </div>
    )
  }

  return (
    <div className="space-y-2">
      {sections.map((section) => {
        const isCollapsed = collapsed.has(section.path)
        return (
          <div
            key={section.path}
            className="border-foreground/6 bg-background-card/30 overflow-hidden rounded-lg border"
          >
            <button
              className="hover:bg-background-card/50 flex h-10 w-full items-center gap-2 px-3 text-left transition-colors"
              onClick={() => toggle(section.path)}
            >
              {isCollapsed ?
                <LuChevronRight className="text-foreground-alt/50 h-3.5 w-3.5 shrink-0" />
              : <LuChevronDown className="text-foreground-alt/50 h-3.5 w-3.5 shrink-0" />
              }
              <span className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
                {section.path}
              </span>
              <span className="flex shrink-0 items-center gap-2 font-mono text-xs">
                {section.deletions > 0 && (
                  <span className="text-error">-{section.deletions}</span>
                )}
                {section.additions > 0 && (
                  <span className="text-success">+{section.additions}</span>
                )}
              </span>
            </button>
            {!isCollapsed && (
              <GitDiffPatch
                patch={section.patch}
                className="border-foreground/6 border-t"
              />
            )}
          </div>
        )
      })}
    </div>
  )
}

interface PatchSection {
  path: string
  additions: number
  deletions: number
  patch?: string
}

function splitPatchFiles(
  patch: string | undefined,
  files: DiffFileStat[] | undefined,
): PatchSection[] {
  if (!patch) return []

  const chunks = splitPatchChunks(patch)
  const byPath = new Map<string, string>()
  for (const chunk of chunks) {
    const path = getPatchPath(chunk)
    if (path) byPath.set(path, chunk)
  }

  if (files?.length) {
    return files.map((file) => ({
      path: file.path ?? '',
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      patch: byPath.get(file.path ?? ''),
    }))
  }

  return chunks.map((chunk) => ({
    path: getPatchPath(chunk) ?? 'diff',
    additions: 0,
    deletions: 0,
    patch: chunk,
  }))
}

function splitPatchChunks(patch: string): string[] {
  if (patch.includes('\ndiff --git ') || patch.startsWith('diff --git ')) {
    return splitPatchChunksByHeader(patch, (line) =>
      line.startsWith('diff --git '),
    )
  }
  return splitPatchChunksByHeader(patch, (line) => line.startsWith('--- '))
}

function splitPatchChunksByHeader(
  patch: string,
  isHeader: (line: string) => boolean,
): string[] {
  const chunks: string[] = []
  const lines = patch.split('\n')
  const current: string[] = []
  for (const line of lines) {
    if (isHeader(line) && current.length > 0) {
      chunks.push(current.join('\n'))
      current.length = 0
    }
    current.push(line)
  }
  if (current.length > 0) chunks.push(current.join('\n'))
  return chunks.filter((chunk) => chunk.trim().length > 0)
}

function getPatchPath(patch: string): string | undefined {
  const first = patch.split('\n')[0]
  if (first.startsWith('diff --git ')) {
    const parts = first.slice('diff --git '.length).split(' ')
    return stripPatchPathPrefix(parts[1]) ?? stripPatchPathPrefix(parts[0])
  }
  if (first.startsWith('--- ')) {
    return stripPatchPathPrefix(patch.split('\n')[1]?.slice(4) ?? '')
  }
  return undefined
}

function stripPatchPathPrefix(path: string | undefined): string | undefined {
  if (!path) return undefined
  if (path === '/dev/null') return undefined
  return path.replace(/^"?[ab]\//, '').replace(/"$/, '')
}
