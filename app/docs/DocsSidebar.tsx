import { useCallback, useMemo } from 'react'
import { LuArrowLeft, LuExternalLink } from '@s4wave/web/images/icons.js'
import { GITHUB_REPO_URL } from '@s4wave/app/github.js'
import { ExternalLink } from '@s4wave/app/landing/ExternalLink.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import { siteDefs } from './sections.js'
import type { DocSection, DocPage } from './types.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

import { utilityState as themeUtilityState } from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { display: 'flex', flex: 1, flexDirection: 'column' },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 9)`,
    },
    cursor: 'pointer',
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule4: { color: themeColors['--color-brand'] },
  rule5: {
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
  },
  rule6: { marginBottom: `calc(${themeLayout['--spacing']} * 4)` },
  rule7: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: '.08em',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule8: { marginBottom: `calc(${themeLayout['--spacing']} * 2.5)` },
  rule9: {
    marginBottom: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.08em',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    textTransform: 'uppercase',
  },
  rule10: { display: 'flex', flexDirection: 'column' },
  rule11: {
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 9)`,
    },
    width: '100%',
    cursor: 'pointer',
    borderTopRightRadius: themeRadii['--radius-md'],
    borderBottomRightRadius: themeRadii['--radius-md'],
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '2px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-leading': `calc(${themeLayout['--spacing']} * 4)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
    '--tw-ring-inset': { default: null, ':focus-visible': 'inset' },
  },
  rule12: {
    borderColor: themeColors['--color-brand'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule13: {
    borderColor: {
      default: '#0000',
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 20%, transparent)`,
      },
    },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule14: {
    marginTop: 'auto',
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule15: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 9)`,
    },
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 9)`,
    },
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// DocsSidebarProps defines the props for DocsSidebar.
interface DocsSidebarProps {
  sections: DocSection[]
  currentDoc?: DocPage
}

// DocsSidebar renders the navigation sidebar showing all sections and pages.
export function DocsSidebar({ sections, currentDoc }: DocsSidebarProps) {
  const navigate = useNavigate()

  const goToPage = useCallback(
    (url: string) => {
      navigate({ path: url })
    },
    [navigate],
  )

  const goToIndex = useCallback(() => {
    navigate({ path: '/docs' })
  }, [navigate])

  const goHome = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  const githubUrl = currentDoc
    ? `${GITHUB_REPO_URL}/blob/master/app/docs/content/${currentDoc.site}/${currentDoc.section}/${currentDoc.filename}`
    : `${GITHUB_REPO_URL}/tree/master/app/docs/content`

  // Group sections by site, preserving order.
  const siteLabels = useMemo(
    () => new Map(siteDefs.map((s) => [s.id, s.label])),
    [],
  )
  const categories = useMemo(() => {
    const cats: { name: string; sections: DocSection[] }[] = []
    for (const section of sections) {
      const label = siteLabels.get(section.site) ?? section.site
      const last = cats[cats.length - 1]
      if (last && last.name === label) {
        last.sections.push(section)
      } else {
        cats.push({ name: label, sections: [section] })
      }
    }
    return cats
  }, [sections, siteLabels])

  return (
    <nav aria-label="Documentation navigation" sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <button
          type="button"
          onClick={goToIndex}
          aria-current={!currentDoc ? 'page' : undefined}
          sx={[
            stylexStyles3.rule3,
            !currentDoc ? stylexStyles3.rule4 : stylexStyles3.rule5,
          ]}
        >
          Documentation
        </button>

        {categories.map((cat) => (
          <div key={cat.name} sx={stylexStyles3.rule6}>
            <h2 sx={stylexStyles3.rule7}>{cat.name}</h2>
            {cat.sections.map((section) => (
              <div
                key={`${section.site}/${section.id}`}
                sx={stylexStyles3.rule8}
              >
                <h3 sx={stylexStyles3.rule9}>{section.label}</h3>
                <ul sx={stylexStyles3.rule10}>
                  {section.pages.map((page) => (
                    <li key={page.url}>
                      <button
                        type="button"
                        onClick={() => goToPage(page.url)}
                        aria-current={
                          currentDoc?.url === page.url ? 'page' : undefined
                        }
                        sx={[
                          stylexStyles3.rule11,
                          currentDoc?.url === page.url
                            ? stylexStyles3.rule12
                            : stylexStyles3.rule13,
                        ]}
                      >
                        {page.title}
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        ))}
      </div>

      <div sx={stylexStyles3.rule14}>
        <ExternalLink href={githubUrl} sx={stylexStyles2.rule1}>
          <LuExternalLink sx={stylexStyles.rule1} />
          View on GitHub
        </ExternalLink>
        <button
          type="button"
          onClick={currentDoc ? goToIndex : goHome}
          sx={stylexStyles3.rule15}
        >
          <LuArrowLeft sx={stylexStyles.rule1} />
          {currentDoc ? 'Back to Documentation' : 'Back to Home'}
        </button>
      </div>
    </nav>
  )
}
