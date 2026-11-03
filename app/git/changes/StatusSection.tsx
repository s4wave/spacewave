import { useCallback, useState } from 'react'
import { LuChevronDown, LuChevronRight } from '@s4wave/web/images/icons.js'

import type { StatusEntry } from '@s4wave/sdk/git/worktree.pb.js'
import { FileStatusCode } from '@s4wave/sdk/git/worktree.pb.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import { colors as themeColors } from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../../web/style/relations.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    textAlign: 'center',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles4 = stylex.create({
  rule1: { color: themeColors['--color-green-400'] },
  rule2: { color: themeColors['--color-yellow-400'] },
  rule3: { color: themeColors['--color-red-400'] },
  rule4: { color: themeColors['--color-blue-400'] },
  rule5: { color: themeColors['--color-foreground-alt'] },
  rule6: { color: 'var(--color-orange-400)' },
})

const stylexStyles3 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 3%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    width: '100%',
  },
  rule3: { paddingBottom: themeLayout['--spacing'] },
  rule4: {
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule5: {
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]: 1,
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 3%, transparent)`,
      },
    },
    display: 'flex',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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

// statusCodeStyles selects the native text tone for a FileStatusCode.
function statusCodeStyles(code: FileStatusCode): stylex.StyleXStyles {
  switch (code) {
    case FileStatusCode.ADDED:
      return stylexStyles4.rule1
    case FileStatusCode.MODIFIED:
      return stylexStyles4.rule2
    case FileStatusCode.DELETED:
      return stylexStyles4.rule3
    case FileStatusCode.RENAMED:
      return stylexStyles4.rule4
    case FileStatusCode.COPIED:
      return stylexStyles4.rule4
    case FileStatusCode.UNTRACKED:
      return stylexStyles4.rule5
    case FileStatusCode.UPDATED_BUT_UNMERGED:
      return stylexStyles4.rule6
    default:
      return stylexStyles4.rule5
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
    <div sx={stylexStyles3.rule1}>
      <button type="button" sx={stylexStyles3.rule2} onClick={toggle}>
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
        <div sx={stylexStyles3.rule3}>
          {entries.map((entry) => {
            const path = entry.filePath ?? ''
            const code =
              statusField === 'staging'
                ? (entry.stagingStatus ?? FileStatusCode.UNMODIFIED)
                : (entry.worktreeStatus ?? FileStatusCode.UNMODIFIED)
            const letter = statusCodeToLetter(code)
            const color = statusCodeStyles(code)

            return (
              <div key={path} sx={[stylexStyles2.rule1, groupMarker]}>
                <span sx={[[stylexStyles5.rule1], [color]]}>{letter}</span>
                <button
                  type="button"
                  sx={stylexStyles3.rule4}
                  onClick={() => onFileClick?.(path)}
                >
                  {path}
                </button>
                <button
                  type="button"
                  sx={stylexStyles3.rule5}
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

export { statusCodeToLetter, statusCodeStyles }
