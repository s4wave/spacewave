import * as stylex from '@stylexjs/stylex'
import {
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    overflow: 'auto',
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
})

// ReadmeSectionProps are props for the ReadmeSection component.
export interface ReadmeSectionProps {
  readmePath: string
  content: string | null
  loading: boolean
}

// ReadmeSection displays the README file content as raw preformatted text.
// Silently hidden when content fails to load (README is supplementary).
export function ReadmeSection({
  readmePath,
  content,
  loading,
}: ReadmeSectionProps) {
  if (loading || !content) return null

  const filename = readmePath.split('/').pop() ?? readmePath

  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>{filename}</div>
      <div sx={stylexStyles.rule3}>
        <pre sx={stylexStyles.rule4}>{content}</pre>
      </div>
    </div>
  )
}
