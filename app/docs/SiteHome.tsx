import { useCallback, useMemo } from 'react'
import {
  LuArrowRight,
  LuBookOpen,
  LuArrowLeft,
} from '@s4wave/web/images/icons.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { siteDefs } from './sections.js'
import type { DocSection } from './types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { marginBottom: `calc(${themeLayout['--spacing']} * 10)` },
  rule2: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
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
  },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: {
      default: 'var(--text-2xl)',
      '@container (min-width: 32rem)': 'var(--text-3xl)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-2xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-3xl--line-height))',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule4: {
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
  rule5: {
    marginBottom: `calc(${themeLayout['--spacing']} * 10)`,
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule6: {
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule7: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule8: {
    marginTop: 'auto',
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand-highlight'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule9: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 5)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
      '@container (min-width: 64rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule10: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule11: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule12: {
    borderRadius: 'var(--radius-xl)',
    borderStyle: 'dashed',
    borderWidth: '1px',
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 8)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 20)`,
    textAlign: 'center',
  },
  rule13: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule14: {
    marginTop: `calc(${themeLayout['--spacing']} * 10)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule15: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule16: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule17: {
    cursor: 'pointer',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    cursor: 'pointer',
    flexDirection: 'column',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
  },
  rule2: {
    display: 'flex',
    cursor: { default: 'pointer', ':disabled': 'default' },
    flexDirection: 'column',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `calc(${themeLayout['--spacing']} * .5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const siteHomePaths: Record<
  string,
  { title: string; url: string; summary: string }[]
> = {
  users: [
    {
      title: 'Create your first Space',
      url: '/docs/users/start/create-your-first-space',
      summary: 'Pick a Quickstart and check that your work is really saved.',
    },
    {
      title: 'Drive and files',
      url: '/docs/users/files/drive-and-files',
      summary:
        'Folders, uploads, downloads, and the same files from a terminal.',
    },
    {
      title: 'Backup and lock setup',
      url: '/docs/users/accounts/backup-and-lock-setup',
      summary: 'Keep a way back into your account before it holds real work.',
    },
  ],
  'self-hosters': [
    {
      title: 'Choose how to run Spacewave',
      url: '/docs/self-hosters/start/choose-how-to-run-spacewave',
      summary: 'Browser, desktop, background service, or Cloud.',
    },
    {
      title: 'Storage modes',
      url: '/docs/self-hosters/storage/storage-modes',
      summary: 'Find out what actually holds each Space, and back that up.',
    },
    {
      title: 'Run the background service',
      url: '/docs/self-hosters/operations/upgrades-and-daemons',
      summary:
        'Run the service, point it at a directory, hand it over cleanly.',
    },
  ],
  developers: [
    {
      title: 'Developer start here',
      url: '/docs/developers/start/developer-start-here',
      summary: 'Pick a path: a live app, a plugin, or a change to Spacewave.',
    },
    {
      title: 'Build a live application',
      url: '/docs/developers/sync/build-a-live-application',
      summary: 'Share a typed schema and subscribe to live data from React.',
    },
    {
      title: 'Build a plugin',
      url: '/docs/developers/plugins/build-a-plugin',
      summary: 'Add an ObjectType and viewer that run inside a Space.',
    },
  ],
}

// SiteHomeProps defines the props for SiteHome.
interface SiteHomeProps {
  siteId: string
  sections: DocSection[]
}

// SiteHome renders the per-site documentation landing with section cards.
export function SiteHome({ siteId, sections }: SiteHomeProps) {
  const navigate = useNavigate()
  const site = useMemo(() => siteDefs.find((s) => s.id === siteId), [siteId])

  const goToPage = useCallback(
    (url: string) => {
      navigate({ path: url })
    },
    [navigate],
  )

  const goToHub = useCallback(() => {
    navigate({ path: '/docs' })
  }, [navigate])

  // Other sites for cross-links.
  const otherSites = useMemo(
    () => siteDefs.filter((s) => s.id !== siteId),
    [siteId],
  )
  const startPaths = siteHomePaths[siteId] ?? []

  return (
    <div>
      <header sx={stylexStyles3.rule1}>
        <button type="button" onClick={goToHub} sx={stylexStyles3.rule2}>
          <LuArrowLeft sx={stylexStyles.rule1} />
          All Documentation
        </button>
        <h1 sx={stylexStyles3.rule3}>{site?.label ?? siteId}</h1>
        <p sx={stylexStyles3.rule4}>{site?.description}</p>
      </header>

      {startPaths.length > 0 && (
        <div sx={stylexStyles3.rule5}>
          {startPaths.map((path) => (
            <button
              type="button"
              key={path.url}
              onClick={() => goToPage(path.url)}
              sx={[stylexStyles2.rule1, groupMarker2]}
            >
              <h2 sx={stylexStyles3.rule6}>{path.title}</h2>
              <p sx={stylexStyles3.rule7}>{path.summary}</p>
              <span sx={stylexStyles3.rule8}>
                Open
                <LuArrowRight sx={stylexStyles.rule2} />
              </span>
            </button>
          ))}
        </div>
      )}

      <div sx={stylexStyles3.rule9}>
        {sections.map((section) => {
          const firstPage = section.pages[0]
          return (
            <button
              type="button"
              key={section.id}
              onClick={() => firstPage && goToPage(firstPage.url)}
              disabled={!firstPage}
              sx={[stylexStyles2.rule2, groupMarker2]}
            >
              <div sx={stylexStyles3.rule10}>
                <LuBookOpen sx={stylexStyles.rule3} />
              </div>
              <h2 sx={stylexStyles3.rule11}>{section.label}</h2>
              <p sx={stylexStyles3.rule7}>
                {section.pages.length}{' '}
                {section.pages.length === 1 ? 'page' : 'pages'}
              </p>
              {firstPage && (
                <span sx={stylexStyles3.rule8}>
                  Get started
                  <LuArrowRight sx={stylexStyles.rule2} />
                </span>
              )}
            </button>
          )
        })}
      </div>

      {sections.every((s) => s.pages.length === 0) && (
        <div sx={stylexStyles3.rule12}>
          <p sx={stylexStyles3.rule13}>No pages yet. Check back soon.</p>
        </div>
      )}

      {otherSites.length > 0 && (
        <div sx={stylexStyles3.rule14}>
          <h3 sx={stylexStyles3.rule15}>Other Documentation</h3>
          <div sx={stylexStyles3.rule16}>
            {otherSites.map((other) => (
              <button
                type="button"
                key={other.id}
                onClick={() => goToPage(`/docs/${other.id}`)}
                sx={stylexStyles3.rule17}
              >
                {other.label}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
