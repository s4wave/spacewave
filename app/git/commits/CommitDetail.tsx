import type { ReactNode } from 'react'

import type { GitRepoHandle } from '@s4wave/sdk/git/repo.js'
import type { CommitInfo } from '@s4wave/sdk/git/repo.pb.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { formatRelativeTime } from '../util.js'
import { GitDiffPatchFiles } from './GitDiffPatch.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 16)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
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
  rule6: {
    display: 'flex',
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-foreground'],
  },
  rule8: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 1.5)` },
  rule9: {
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
  rule10: { color: themeColors['--color-foreground'] },
  rule11: {
    marginLeft: themeLayout['--spacing'],
    color: themeColors['--color-foreground-alt'],
  },
  rule12: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule13: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule15: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule16: { minHeight: 0, flex: 1, overflow: 'auto' },
  rule17: { padding: `calc(${themeLayout['--spacing']} * 3)` },
})

// CommitDetailProps are props for the CommitDetail component.
export interface CommitDetailProps {
  handle: GitRepoHandle
  commitHash: string
  onNavigateCommit?: (hash: string) => void
}

interface CommitFieldProps {
  label: string
  children: ReactNode
}

/** CommitField renders one labeled row of commit metadata. */
function CommitField({ label, children }: CommitFieldProps) {
  return (
    <div sx={stylexStyles.rule1}>
      <span sx={stylexStyles.rule2}>{label}</span>
      {children}
    </div>
  )
}

interface CommitHeaderProps {
  commit: CommitInfo
  onNavigateCommit?: (hash: string) => void
}

/** CommitHeader renders the commit message and its hash, parent, author, and date. */
function CommitHeader({ commit, onNavigateCommit }: CommitHeaderProps) {
  const message = commit.message ?? ''
  const subject = message.split('\n')[0]
  const body = message.split('\n').slice(1).join('\n').trim()
  const parentHashes = commit.parentHashes ?? []
  const authorTimestamp = commit.authorTimestamp
  const authorDate = authorTimestamp
    ? new Date(Number(authorTimestamp) * 1000)
    : null

  return (
    <div sx={stylexStyles.rule3}>
      <div sx={stylexStyles.rule4}>{subject}</div>
      {body && <pre sx={stylexStyles.rule5}>{body}</pre>}
      <div sx={stylexStyles.rule6}>
        <CommitField label="Commit">
          <span sx={stylexStyles.rule7}>{commit.hash ?? ''}</span>
        </CommitField>
        {parentHashes.length > 0 && (
          <CommitField label={parentHashes.length > 1 ? 'Parents' : 'Parent'}>
            <span sx={stylexStyles.rule8}>
              {parentHashes.map((ph) => (
                <button
                  type="button"
                  key={ph}
                  sx={stylexStyles.rule9}
                  onClick={() => onNavigateCommit?.(ph)}
                >
                  {ph.slice(0, 7)}
                </button>
              ))}
            </span>
          </CommitField>
        )}
        <CommitField label="Author">
          <span sx={stylexStyles.rule10}>
            {commit.authorName}
            {commit.authorEmail && (
              <span sx={stylexStyles.rule11}>
                {'<'}
                {commit.authorEmail}
                {'>'}
              </span>
            )}
          </span>
        </CommitField>
        {authorDate && (
          <CommitField label="Date">
            <span sx={stylexStyles.rule10}>{authorDate.toLocaleString()}</span>
            <span sx={stylexStyles.rule12}>
              ({formatRelativeTime(authorTimestamp)})
            </span>
          </CommitField>
        )}
      </div>
    </div>
  )
}

// CommitDetail displays a full commit detail page.
export function CommitDetail({
  handle,
  commitHash,
  onNavigateCommit,
}: CommitDetailProps) {
  const commitResource = useResource(
    { value: handle, loading: false, error: null, retry: () => {} },
    async (h) => {
      if (!h) return null
      return (await h.getCommit(commitHash)) ?? null
    },
    [commitHash],
  )

  const diffStatResource = useResource(
    { value: handle, loading: false, error: null, retry: () => {} },
    async (h) => {
      if (!h) return null
      return h.getDiffStat(commitHash)
    },
    [commitHash],
  )

  const diffPatchResource = useResource(
    { value: handle, loading: false, error: null, retry: () => {} },
    async (h) => {
      if (!h) return null
      return h.getDiffPatch(commitHash)
    },
    [commitHash],
  )

  const commit = commitResource.value

  if (commitResource.loading) {
    return (
      <div sx={stylexStyles.rule13}>
        <div sx={stylexStyles.rule14}>Loading commit…</div>
      </div>
    )
  }

  if (commitResource.error) {
    return (
      <div sx={stylexStyles.rule13}>
        <div sx={stylexStyles.rule15}>
          Failed to load commit: {commitResource.error.message}
        </div>
      </div>
    )
  }

  if (!commit) {
    return (
      <div sx={stylexStyles.rule13}>
        <div sx={stylexStyles.rule14}>Commit not found</div>
      </div>
    )
  }

  return (
    <div sx={stylexStyles.rule16}>
      <CommitHeader commit={commit} onNavigateCommit={onNavigateCommit} />
      <div sx={stylexStyles.rule17}>
        <GitDiffPatchFiles
          files={diffStatResource.value?.files}
          patch={diffPatchResource.value?.patch}
          loading={diffStatResource.loading || diffPatchResource.loading}
          truncated={diffPatchResource.value?.truncated}
          totalBytes={diffPatchResource.value?.totalBytes}
          limitBytes={diffPatchResource.value?.limitBytes}
          error={diffPatchResource.error}
        />
      </div>
    </div>
  )
}
