import * as stylex from '@stylexjs/stylex'
import {
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule3: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'auto',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'pre-wrap',
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
