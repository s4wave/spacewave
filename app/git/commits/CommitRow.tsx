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
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderBottomStyle: {
      default: themeUtilityState['--tw-border-style'],
      ':last-child': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: { default: '1px', ':last-child': 0 },
  },
  rule2: {
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
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
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    width: '100%',
  },
  rule3: { padding: 0, backgroundColor: '#0000', flexShrink: 0 },
  rule4: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: 0,
    backgroundColor: '#0000',
    color: themeColors['--color-brand'],
    cursor: 'pointer',
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule5: {
    padding: 0,
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: '#0000',
    display: 'flex',
    textAlign: 'left',
    minWidth: 0,
  },
  rule6: {
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule7: { color: themeColors['--color-foreground-alt'], flexShrink: 0 },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    flexShrink: 0,
  },
  rule9: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: 'color-mix(in oklab, var(--color-black) 2%, transparent)',
  },
  rule10: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'pre-wrap',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule11: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
