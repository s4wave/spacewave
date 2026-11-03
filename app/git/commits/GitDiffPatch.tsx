import { useCallback, useMemo, useState } from 'react'
import { LuChevronDown, LuChevronRight } from '@s4wave/web/images/icons.js'

import { PatchDiff, type PatchDiffProps } from '@pierre/diffs/react'

import type { DiffFileStat } from '@s4wave/sdk/git/repo.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    color: themeColors['--color-warning'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
  },
  rule7: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule8: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule9: { color: themeColors['--color-error'] },
  rule10: { color: themeColors['--color-success'] },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// GitDiffPatchProps are props for the GitDiffPatch component.
export interface GitDiffPatchProps {
  patch: string | undefined
  sx?: stylex.StyleXStyles
  className?: never
}

// GitDiffPatch renders a unified git patch with Spacewave styling.
export function GitDiffPatch({ patch, sx }: GitDiffPatchProps) {
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
    <div sx={[patchContainerStyles.root, sx]}>
      <PatchDiff
        patch={patch}
        disableWorkerPool
        className={'git-diff-patch'}
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
  truncated?: boolean
  totalBytes?: bigint | number
  limitBytes?: number
  error?: Error | null
}

// GitDiffPatchFiles renders collapsible per-file patch sections.
export function GitDiffPatchFiles({
  files,
  patch,
  loading,
  truncated,
  totalBytes,
  limitBytes,
  error,
}: GitDiffPatchFilesProps) {
  const sections = useMemo(
    () => splitPatchFiles(patch, files, truncated ?? false),
    [patch, files, truncated],
  )
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
    return <div sx={stylexStyles2.rule1}>Loading diff…</div>
  }

  if (error) {
    return (
      <div sx={stylexStyles2.rule2}>Failed to load diff: {error.message}</div>
    )
  }

  if (sections.length === 0) {
    return <div sx={stylexStyles2.rule3}>No diff to display</div>
  }

  return (
    <div sx={false}>
      {truncated && (
        <div sx={[nativeChildSpacing.s2, stylexStyles2.rule4]}>
          Showing the first {formatBytes(limitBytes ?? 0)} of{' '}
          {formatBytes(totalBytes ?? 0)}.
        </div>
      )}
      {sections.map((section) => {
        const isCollapsed = collapsed.has(section.path)
        return (
          <div
            key={section.path}
            sx={[nativeChildSpacing.s2, stylexStyles2.rule5]}
          >
            <button
              type="button"
              sx={stylexStyles2.rule6}
              onClick={() => toggle(section.path)}
            >
              {isCollapsed ? (
                <LuChevronRight sx={stylexStyles.rule1} />
              ) : (
                <LuChevronDown sx={stylexStyles.rule1} />
              )}
              <span sx={stylexStyles2.rule7}>{section.path}</span>
              <span sx={stylexStyles2.rule8}>
                {section.deletions > 0 && (
                  <span sx={stylexStyles2.rule9}>-{section.deletions}</span>
                )}
                {section.additions > 0 && (
                  <span sx={stylexStyles2.rule10}>+{section.additions}</span>
                )}
              </span>
            </button>
            {!isCollapsed && (
              <GitDiffPatch patch={section.patch} sx={stylexStyles2.rule11} />
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
  truncated: boolean,
): PatchSection[] {
  if (!patch) return []

  const chunks = splitPatchChunks(patch)
  const byPath = new Map<string, string>()
  for (const chunk of chunks) {
    const path = getPatchPath(chunk)
    if (path) byPath.set(path, chunk)
  }

  if (files?.length) {
    const sections = files.map((file) => ({
      path: file.path ?? '',
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      patch: byPath.get(file.path ?? ''),
    }))
    if (truncated) {
      return sections.filter((section) => section.patch)
    }
    return sections
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

function formatBytes(bytes: bigint | number): string {
  const value = Number(bytes)
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB']
  let scaled = value
  let unit = 0
  while (scaled >= 1024 && unit < units.length - 1) {
    scaled /= 1024
    unit++
  }
  return `${scaled.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

// patchContainerStyles bounds the vendor diff view without changing its skin.
const patchContainerStyles = stylex.create({ root: { overflow: 'hidden' } })
