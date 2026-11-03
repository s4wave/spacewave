import type { IconType } from '@s4wave/web/images/icons.js'
import type { ReactElement, ReactNode } from 'react'

import { LegalPageLayout } from '../LegalPageLayout.js'
import { UseCaseCtaLink, UseCaseCtaRow } from '../UseCaseCtaRow.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    display: 'grid',
    width: '100%',
    maxWidth: 'var(--container-6xl)',
    gap: `calc(${themeLayout['--spacing']} * 8)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: `calc(${themeLayout['--spacing']} * 20)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-2xl)',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 5)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: `calc(${themeLayout['--spacing']} * 20)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 24)`,
    textAlign: 'center',
  },
  rule6: {
    fontSize: 'var(--text-2xl)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule7: {
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
})

// UseCasePoint is one short statement about what the demo showed.
export interface UseCasePoint {
  title: string
  body: string
}

// UseCaseAction is a call to action below the demo.
export interface UseCaseAction {
  href: string
  label: string
  icon: IconType
}

// UseCasePageProps configures a use-case landing page.
export interface UseCasePageProps {
  icon: ReactElement
  title: string
  subtitle: string
  children: ReactNode
  points: UseCasePoint[]
  keepTitle: string
  keepBody: string
  primary: UseCaseAction
  secondary?: UseCaseAction
}

// UseCasePage renders a use-case landing page: the job, the product doing it
// (children, usually a UseCaseDemo), what that showed, and how to keep it.
export function UseCasePage({
  icon,
  title,
  subtitle,
  children,
  points,
  keepTitle,
  keepBody,
  primary,
  secondary,
}: UseCasePageProps) {
  return (
    <LegalPageLayout icon={icon} title={title} subtitle={subtitle}>
      {children}

      <section sx={stylexStyles.rule1}>
        {points.map((point) => (
          <div key={point.title} sx={stylexStyles.rule2}>
            <h3 sx={stylexStyles.rule3}>{point.title}</h3>
            <p sx={stylexStyles.rule4}>{point.body}</p>
          </div>
        ))}
      </section>

      <section sx={stylexStyles.rule5}>
        <h2 sx={stylexStyles.rule6}>{keepTitle}</h2>
        <p sx={stylexStyles.rule7}>{keepBody}</p>
        <UseCaseCtaRow>
          <UseCaseCtaLink
            href={primary.href}
            icon={primary.icon}
            variant="primary"
          >
            {primary.label}
          </UseCaseCtaLink>
          {secondary && (
            <UseCaseCtaLink href={secondary.href} icon={secondary.icon}>
              {secondary.label}
            </UseCaseCtaLink>
          )}
        </UseCaseCtaRow>
      </section>
    </LegalPageLayout>
  )
}
