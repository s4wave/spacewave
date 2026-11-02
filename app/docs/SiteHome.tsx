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
      <header className="mb-10">
        <button
          type="button"
          onClick={goToHub}
          className="text-foreground-alt/50 hover:text-foreground-alt mb-4 flex cursor-pointer items-center gap-1.5 text-xs transition-colors"
        >
          <LuArrowLeft sx={stylexStyles.rule1} />
          All Documentation
        </button>
        <h1 className="text-foreground mb-3 text-2xl font-semibold tracking-tight @lg:text-3xl">
          {site?.label ?? siteId}
        </h1>
        <p className="text-foreground-alt text-sm leading-relaxed @lg:text-base">
          {site?.description}
        </p>
      </header>

      {startPaths.length > 0 && (
        <div className="mb-10 grid gap-4 @lg:grid-cols-3">
          {startPaths.map((path) => (
            <button
              type="button"
              key={path.url}
              onClick={() => goToPage(path.url)}
              sx={[stylexStyles2.rule1, groupMarker2]}
            >
              <h2 className="text-foreground text-base font-semibold">
                {path.title}
              </h2>
              <p className="text-foreground-alt/70 text-sm leading-relaxed">
                {path.summary}
              </p>
              <span className="text-brand group-hover:text-brand-highlight mt-auto flex items-center gap-1.5 text-xs font-medium transition-colors">
                Open
                <LuArrowRight sx={stylexStyles.rule2} />
              </span>
            </button>
          ))}
        </div>
      )}

      <div className="grid gap-5 @lg:grid-cols-2 @5xl:grid-cols-3">
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
              <div className="bg-brand/10 text-brand flex size-10 items-center justify-center rounded-lg">
                <LuBookOpen sx={stylexStyles.rule3} />
              </div>
              <h2 className="text-foreground text-lg font-semibold">
                {section.label}
              </h2>
              <p className="text-foreground-alt/70 text-sm leading-relaxed">
                {section.pages.length}{' '}
                {section.pages.length === 1 ? 'page' : 'pages'}
              </p>
              {firstPage && (
                <span className="text-brand group-hover:text-brand-highlight mt-auto flex items-center gap-1.5 text-xs font-medium transition-colors">
                  Get started
                  <LuArrowRight sx={stylexStyles.rule2} />
                </span>
              )}
            </button>
          )
        })}
      </div>

      {sections.every((s) => s.pages.length === 0) && (
        <div className="border-foreground/6 rounded-xl border border-dashed px-8 py-20 text-center">
          <p className="text-foreground-alt text-sm">
            No pages yet. Check back soon.
          </p>
        </div>
      )}

      {otherSites.length > 0 && (
        <div className="mt-10 border-t border-white/10 pt-6">
          <h3 className="text-foreground-alt/50 mb-3 text-xs font-semibold tracking-widest uppercase">
            Other Documentation
          </h3>
          <div className="flex flex-wrap gap-3">
            {otherSites.map((other) => (
              <button
                type="button"
                key={other.id}
                onClick={() => goToPage(`/docs/${other.id}`)}
                className="text-foreground-alt/70 hover:text-foreground cursor-pointer text-sm transition-colors"
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
