import { useCallback } from 'react'
import { LuChevronDown, LuChevronRight } from '@s4wave/web/images/icons.js'

import type { CommitInfo, DiffFileStat } from '@s4wave/sdk/git/repo.pb.js'

import { formatRelativeTime } from '../util.js'
import { DiffStatSection } from './DiffStatSection.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderBottomStyle: {
      default: themeUtilityState['--tw-border-style'],
      ':last-child': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: { default: '1px', ':last-child': 0 },
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule2: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
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
  rule3: { flexShrink: 0, backgroundColor: '#0000', padding: 0 },
  rule4: {
    flexShrink: 0,
    cursor: 'pointer',
    backgroundColor: '#0000',
    padding: 0,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-brand'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule5: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: '#0000',
    padding: 0,
    textAlign: 'left',
  },
  rule6: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
  },
  rule7: { flexShrink: 0, color: themeColors['--color-foreground-alt'] },
  rule8: {
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule9: {
    backgroundColor: 'color-mix(in oklab, var(--color-black) 2%, transparent)',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule10: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    whiteSpace: 'pre-wrap',
    color: themeColors['--color-foreground'],
  },
  rule11: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground-alt'],
  },
})

// CommitRowProps are props for the CommitRow component.
export interface CommitRowProps {
  commit: CommitInfo
  expanded: boolean
  onToggle: (hash: string) => void
  onCommitClick?: (hash: string) => void
  diffStat: DiffFileStat[] | undefined
  diffStatLoading: boolean
  onLoadDiffStat: (hash: string) => void
}

// CommitRow displays a single commit with expand/collapse for details.
export function CommitRow({
  commit,
  expanded,
  onToggle,
  onCommitClick,
  diffStat,
  diffStatLoading,
  onLoadDiffStat,
}: CommitRowProps) {
  const hash = commit.hash ?? ''
  const shortHash = hash.slice(0, 7)
  const message = commit.message ?? ''
  const subject = message.split('\n')[0]
  const body = message.split('\n').slice(1).join('\n').trim()
  const author = commit.authorName ?? ''
  const timeAgo = formatRelativeTime(commit.authorTimestamp)

  const handleCommitExpand = useCallback(() => {
    onToggle(hash)
    if (!expanded) {
      onLoadDiffStat(hash)
    }
  }, [hash, expanded, onToggle, onLoadDiffStat])

  const handleHashClick = useCallback(() => {
    onCommitClick?.(hash)
  }, [hash, onCommitClick])

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <button
          type="button"
          aria-label={expanded ? 'Collapse commit' : 'Expand commit'}
          aria-expanded={expanded}
          sx={stylexStyles2.rule3}
          onClick={handleCommitExpand}
        >
          {expanded ? (
            <LuChevronDown sx={stylexStyles.rule1} />
          ) : (
            <LuChevronRight sx={stylexStyles.rule1} />
          )}
        </button>
        <button
          type="button"
          sx={stylexStyles2.rule4}
          onClick={handleHashClick}
        >
          {shortHash}
        </button>
        <button
          type="button"
          aria-expanded={expanded}
          sx={stylexStyles2.rule5}
          onClick={handleCommitExpand}
        >
          <span sx={stylexStyles2.rule6}>{subject}</span>
          {author && <span sx={stylexStyles2.rule7}>{author}</span>}
          {timeAgo && <span sx={stylexStyles2.rule8}>{timeAgo}</span>}
        </button>
      </div>
      {expanded && (
        <div sx={stylexStyles2.rule9}>
          {body && <pre sx={stylexStyles2.rule10}>{body}</pre>}
          {(commit.parentHashes?.length ?? 0) > 0 && (
            <div sx={stylexStyles2.rule11}>
              {(commit.parentHashes?.length ?? 0) > 1
                ? 'Parents: '
                : 'Parent: '}
              {commit.parentHashes?.map((ph) => ph.slice(0, 7)).join(', ')}
            </div>
          )}
          <DiffStatSection files={diffStat} loading={diffStatLoading} />
        </div>
      )}
    </div>
  )
}
