import { useCallback } from 'react'
import { LuArrowRight, LuUser, LuServer, LuCode } from 'react-icons/lu'
import { useNavigate } from '@s4wave/web/router/router.js'
import { siteDefs } from './sections.js'

const siteIcons: Record<string, React.ComponentType<{ className?: string }>> = {
  users: LuUser,
  'self-hosters': LuServer,
  developers: LuCode,
}

// DocsHub renders the documentation hub with audience surface cards.
export function DocsHub() {
  const navigate = useNavigate()

  const goToSite = useCallback(
    (siteId: string) => {
      navigate({ path: `/docs/${siteId}` })
    },
    [navigate],
  )

  return (
    <div>
      <header className="mb-10">
        <h1 className="text-foreground mb-3 text-2xl font-bold tracking-tight @lg:text-3xl">
          Documentation
        </h1>
        <p className="text-foreground-alt text-sm leading-relaxed @lg:text-base">
          Guides and references for building with Spacewave.
        </p>
      </header>

      <div className="grid gap-5 @lg:grid-cols-3">
        {siteDefs.map((site) => {
          const Icon = siteIcons[site.id] ?? LuUser
          return (
            <button
              key={site.id}
              onClick={() => goToSite(site.id)}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-start gap-3 rounded-xl border p-6 text-left transition-all duration-200"
            >
              <div className="bg-brand/10 text-brand flex h-10 w-10 items-center justify-center rounded-lg">
                <Icon className="h-5 w-5" />
              </div>
              <h2 className="text-foreground text-lg font-semibold">
                {site.label}
              </h2>
              <p className="text-foreground-alt/70 text-sm leading-relaxed">
                {site.description}
              </p>
              <span className="text-brand group-hover:text-brand-highlight mt-auto flex items-center gap-1.5 text-xs font-medium transition-colors">
                Browse
                <LuArrowRight className="h-3 w-3 transition-transform duration-200 group-hover:translate-x-0.5" />
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
