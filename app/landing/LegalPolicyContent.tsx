import Markdown, { type MarkdownToJSX } from 'markdown-to-jsx'

import { MarkdownLink } from '@s4wave/app/docs/MarkdownLink.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-4xl)',
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    width: '100%',
  },
  rule2: {
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
  },
})

// policyMarkdownStyles styles the public link component and Markdown-owned paragraphs.
const policyMarkdownStyles = stylex.create({
  link: {
    color: {
      default: 'var(--color-brand)',
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--color-brand-highlight)',
      },
    },
    textDecorationLine: 'underline',
  },
  paragraph: {
    marginBottom: { default: 'calc(var(--spacing) * 3)', ':last-child': 0 },
  },
})

const markdownOptions: MarkdownToJSX.Options = {
  overrides: {
    a: {
      component: MarkdownLink,
      props: { sx: policyMarkdownStyles.link },
    },
    p: { props: stylex.props(policyMarkdownStyles.paragraph) },
  },
}

interface LegalPolicyContentProps {
  sections: readonly { title: string; content: string }[]
}

// LegalPolicyContent renders the canonical policy sections with routable links.
export function LegalPolicyContent({ sections }: LegalPolicyContentProps) {
  return (
    <section sx={stylexStyles.rule1}>
      <div sx={false}>
        {sections.map(({ title, content }) => (
          <section key={title} sx={[nativeChildSpacing.s6, stylexStyles.rule2]}>
            <h2 sx={stylexStyles.rule3}>{title}</h2>
            <div sx={stylexStyles.rule4}>
              <Markdown options={markdownOptions}>{content}</Markdown>
            </div>
          </section>
        ))}
      </div>
    </section>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s6: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * var(--tw-space-y-reverse))`,
    },
  },
})
