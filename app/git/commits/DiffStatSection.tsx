import type { DiffFileStat } from '@s4wave/sdk/git/repo.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule3: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule5: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
  },
  rule6: { flexShrink: 0 },
  rule7: { color: themeColors['--color-green-500'] },
  rule8: { color: themeColors['--color-red-500'] },
  rule9: {
    marginTop: themeLayout['--spacing'],
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: themeLayout['--spacing'],
    color: themeColors['--color-foreground-alt'],
  },
})

// DiffStatSectionProps are props for the DiffStatSection component.
export interface DiffStatSectionProps {
  files: DiffFileStat[] | undefined
  loading: boolean
}

// DiffStatSection displays the file-level diff stats for a commit.
export function DiffStatSection({ files, loading }: DiffStatSectionProps) {
  if (loading) {
    return <div sx={stylexStyles.rule1}>Loading diff stat…</div>
  }

  if (!files || files.length === 0) {
    return <div sx={stylexStyles.rule2}>No file changes</div>
  }

  const totalAdditions = files.reduce((sum, f) => sum + (f.additions ?? 0), 0)
  const totalDeletions = files.reduce((sum, f) => sum + (f.deletions ?? 0), 0)

  return (
    <div sx={stylexStyles.rule3}>
      {files.map((file) => (
        <div key={file.path} sx={stylexStyles.rule4}>
          <span sx={stylexStyles.rule5}>{file.path}</span>
          <span sx={stylexStyles.rule6}>
            {(file.additions ?? 0) > 0 && (
              <span sx={stylexStyles.rule7}>+{file.additions}</span>
            )}
            {(file.additions ?? 0) > 0 && (file.deletions ?? 0) > 0 && ' '}
            {(file.deletions ?? 0) > 0 && (
              <span sx={stylexStyles.rule8}>-{file.deletions}</span>
            )}
          </span>
        </div>
      ))}
      <div sx={stylexStyles.rule9}>
        <span>
          {files.length} file{files.length !== 1 ? 's' : ''} changed
        </span>
        {totalAdditions > 0 && (
          <span sx={stylexStyles.rule7}>+{totalAdditions}</span>
        )}
        {totalDeletions > 0 && (
          <span sx={stylexStyles.rule8}>-{totalDeletions}</span>
        )}
      </div>
    </div>
  )
}
