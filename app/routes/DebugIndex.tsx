import { useNavigate } from '@s4wave/web/router/router.js'

interface DebugIndexLink {
  path: string
  title: string
  description: string
}

interface DebugIndexProps {
  links: DebugIndexLink[]
}

// DebugIndex renders the development route index.
export function DebugIndex({ links }: DebugIndexProps) {
  const navigate = useNavigate()

  return (
    <div className="bg-background flex h-full w-full flex-1 overflow-y-auto p-6">
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
        <div>
          <p className="text-brand text-xs font-semibold uppercase tracking-[0.22em]">
            Developer
          </p>
          <h1 className="text-foreground mt-2 text-2xl font-semibold tracking-tight">
            Debug tools
          </h1>
          <p className="text-foreground-alt/70 mt-2 text-sm">
            Inspect rendering fixtures, storage benchmarks, and debug-only UI
            surfaces.
          </p>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          {links.map((link) => (
            <a
              key={link.path}
              href={`#${link.path}`}
              onClick={(event) => {
                event.preventDefault()
                navigate({ path: link.path })
              }}
              className="border-foreground/10 bg-background-card/60 hover:border-brand/40 hover:bg-background-card flex flex-col gap-1 rounded-xl border p-4 text-left transition-colors"
            >
              <span className="text-foreground text-sm font-medium">
                {link.title}
              </span>
              <span className="text-foreground-alt/65 text-xs leading-relaxed">
                {link.description}
              </span>
              <span className="text-brand/80 mt-1 text-xs font-mono">
                {link.path}
              </span>
            </a>
          ))}
        </div>
      </div>
    </div>
  )
}
