import { useCallback, useMemo } from 'react'
import { LuArrowRight, LuBookOpen, LuArrowLeft } from 'react-icons/lu'
import { useNavigate } from '@s4wave/web/router/router.js'
import { siteDefs } from './sections.js'
import type { DocSection } from './types.js'

const siteHomePaths: Record<
  string,
  { title: string; url: string; summary: string }[]
> = {
  users: [
    {
      title: 'Create your first Space',
      url: '/docs/users/start/create-your-first-space',
      summary: 'Pick the smallest useful Quickstart and verify persistence.',
    },
    {
      title: 'Drive and files',
      url: '/docs/users/files/drive-and-files',
      summary: 'Use Drive for folders, uploads, downloads, and CLI paths.',
    },
    {
      title: 'Backup and lock setup',
      url: '/docs/users/accounts/backup-and-lock-setup',
      summary: 'Protect a local or cloud session before it holds real work.',
    },
  ],
  'self-hosters': [
    {
      title: 'Choose how to run Spacewave',
      url: '/docs/self-hosters/start/choose-how-to-run-spacewave',
      summary: 'Pick browser, desktop, daemon, or cloud-backed operation.',
    },
    {
      title: 'Storage modes',
      url: '/docs/self-hosters/storage/storage-modes',
      summary: 'Tie backups to the state path or provider that owns the data.',
    },
    {
      title: 'Upgrades and daemons',
      url: '/docs/self-hosters/operations/upgrades-and-daemons',
      summary: 'Keep daemon, socket, and state-path ownership explicit.',
    },
  ],
  developers: [
    {
      title: 'Developer start here',
      url: '/docs/developers/start/developer-start-here',
      summary:
        'Start from Space, ObjectType, viewer, wizard, and plugin owners.',
    },
    {
      title: 'Build a plugin',
      url: '/docs/developers/plugins/build-a-plugin',
      summary: 'Own the ObjectType, viewer, manifest, and registration path.',
    },
    {
      title: 'CLI reference',
      url: '/docs/developers/cli/cli-reference',
      summary:
        'Use the current command tree and connection rules while building.',
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
          onClick={goToHub}
          className="text-foreground-alt/50 hover:text-foreground-alt mb-4 flex cursor-pointer items-center gap-1.5 text-xs transition-colors"
        >
          <LuArrowLeft className="size-3" />
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
              key={path.url}
              onClick={() => goToPage(path.url)}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-start gap-3 rounded-xl border p-5 text-left transition-all duration-200"
            >
              <h2 className="text-foreground text-base font-semibold">
                {path.title}
              </h2>
              <p className="text-foreground-alt/70 text-sm leading-relaxed">
                {path.summary}
              </p>
              <span className="text-brand group-hover:text-brand-highlight mt-auto flex items-center gap-1.5 text-xs font-medium transition-colors">
                Open
                <LuArrowRight className="size-3 transition-transform duration-200 group-hover:translate-x-0.5" />
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
              key={section.id}
              onClick={() => firstPage && goToPage(firstPage.url)}
              disabled={!firstPage}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-start gap-3 rounded-xl border p-6 text-left transition-all duration-200 disabled:cursor-default disabled:opacity-50"
            >
              <div className="bg-brand/10 text-brand flex size-10 items-center justify-center rounded-lg">
                <LuBookOpen className="size-5" />
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
                  <LuArrowRight className="size-3 transition-transform duration-200 group-hover:translate-x-0.5" />
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
