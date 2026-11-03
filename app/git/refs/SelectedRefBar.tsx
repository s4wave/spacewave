import { LuFolder, LuHistory } from '@s4wave/web/images/icons.js'

import type { CommitInfo } from '@s4wave/sdk/git/repo.pb.js'

import { formatRelativeTime } from '../util.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 3%, transparent)`,
      },
    },
  },
  rule5: {
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-brand'],
  },
  rule6: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule7: { flexShrink: 0, color: themeColors['--color-foreground-alt'] },
  rule8: {
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule9: {
    flexShrink: 0,
    borderRadius: '.25rem',
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
    return <div sx={stylexStyles2.rule1}>Loading commit info…</div>
  }

  if (error) {
    return <div sx={stylexStyles2.rule2}>Failed to load commit info</div>
  }

  if (!lastCommit) return null

  const subject = lastCommit.message?.split('\n')[0] ?? ''
  const shortHash = (lastCommit.hash ?? '').slice(0, 7)
  const author = lastCommit.authorName ?? ''
  const timeAgo = formatRelativeTime(lastCommit.authorTimestamp)

  return (
    <div sx={stylexStyles2.rule3}>
      <button type="button" sx={stylexStyles2.rule4} onClick={onClickCommit}>
        <span sx={stylexStyles2.rule5}>{shortHash}</span>
        <span sx={stylexStyles2.rule6}>{subject}</span>
        {author && <span sx={stylexStyles2.rule7}>by {author}</span>}
        {timeAgo && <span sx={stylexStyles2.rule8}>{timeAgo}</span>}
      </button>
      {onClickTree && (
        <button
          type="button"
          sx={stylexStyles2.rule9}
          onClick={onClickTree}
          title="Browse files"
        >
          <LuFolder sx={stylexStyles.rule1} />
        </button>
      )}
      {onClickLog && (
        <button
          type="button"
          sx={stylexStyles2.rule9}
          onClick={onClickLog}
          title="View commit log"
        >
          <LuHistory sx={stylexStyles.rule1} />
        </button>
      )}
    </div>
  )
}
