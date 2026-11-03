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
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: {
      default: 'var(--text-2xl)',
      '@container (min-width: 32rem)': 'var(--text-3xl)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: {
      default: 'var(--tw-leading, var(--text-2xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-3xl--line-height))',
    },
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(3, minmax(0, 1fr))',
    },
    marginBottom: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule6: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-base)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
  },
  rule7: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule8: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand-highlight'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginTop: 'auto',
  },
  rule9: {
    gap: `calc(${themeLayout['--spacing']} * 5)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
      '@container (min-width: 64rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule10: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule11: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
  },
  rule12: {
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: 'dashed',
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 20)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'center',
  },
  rule13: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule14: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    marginTop: `calc(${themeLayout['--spacing']} * 10)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule15: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule16: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
  },
  rule17: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.2s',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'left',
    transitionDuration: '.2s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    '--tw-duration': '.2s',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
    cursor: { default: 'pointer', ':disabled': 'default' },
    display: 'flex',
    flexDirection: 'column',
    opacity: { default: null, ':disabled': 0.5 },
    textAlign: 'left',
    transitionDuration: '.2s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    '--tw-duration': '.2s',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `calc(${themeLayout['--spacing']} * .5)`,
      },
    },
    transitionDuration: '.2s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
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
