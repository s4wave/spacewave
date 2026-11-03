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
    width: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    textAlign: 'center',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
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
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule2: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 3%, transparent)`,
      },
    },
  },
  rule3: { paddingBottom: themeLayout['--spacing'] },
  rule4: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule5: {
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]: 1,
      },
    },
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

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
