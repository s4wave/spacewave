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
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    color: themeColors['--color-destructive'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule3: {
    WebkitUserSelect: 'none',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule4: {
    borderRadius: '.25rem',
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 3%, transparent)`,
      },
    },
    display: 'flex',
    minWidth: 0,
  },
  rule5: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-brand'],
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule7: { color: themeColors['--color-foreground-alt'], flexShrink: 0 },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    flexShrink: 0,
  },
  rule9: {
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    borderRadius: '.25rem',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    flexShrink: 0,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
