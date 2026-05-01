import { QUICKSTART_OPTIONS, type QuickstartOption } from './options.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { usePath } from '@s4wave/web/router/router.js'

// QuickstartLoading is a static prerendered page for /quickstart/{id}.
// Shows the quickstart metadata with a loading indicator. When the
// entrypoint finishes background boot (WASM ready), hydrate.tsx
// auto-transitions to the full app at #/quickstart/{id}.
export function QuickstartLoading() {
  const path = usePath()
  const id = path.split('/').pop() ?? ''
  const option = QUICKSTART_OPTIONS.find((o) => o.id === id)
  const landingHref = useStaticHref('/')

  if (!option) {
    return (
      <div className="flex min-h-screen flex-1 items-center justify-center bg-[var(--color-neutral-950)]">
        <p className="text-[var(--color-neutral-400)]">
          Unknown quickstart option.
        </p>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen flex-1 flex-col items-center justify-center bg-[var(--color-neutral-950)] px-4 text-[var(--color-neutral-100)]">
      <div className="flex max-w-md flex-col items-center gap-6 text-center">
        <QuickstartIcon option={option} />
        <h1 className="text-2xl font-semibold">{option.name}</h1>
        <p className="text-lg text-[var(--color-neutral-400)]">
          {option.description}
        </p>
        <LoadingDots />
        <a
          href={landingHref}
          className="mt-4 text-sm text-[var(--color-neutral-500)] transition-colors hover:text-[var(--color-neutral-300)]"
        >
          Back to home
        </a>
      </div>
    </div>
  )
}

function QuickstartIcon(props: { option: QuickstartOption }) {
  const Icon = props.option.icon
  return (
    <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-[var(--color-neutral-900)]">
      <Icon className="h-8 w-8 text-[var(--color-neutral-300)]" />
    </div>
  )
}

function LoadingDots() {
  return (
    <div className="flex gap-1.5">
      {[0, 1, 2].map((i) => (
        <div
          key={i}
          className="h-2 w-2 rounded-full bg-[var(--color-neutral-500)]"
          style={{
            animation: 'pulse 1.4s ease-in-out infinite',
            animationDelay: `${i * 0.2}s`,
          }}
        />
      ))}
    </div>
  )
}

// buildQuickstartMetadata generates page metadata for a quickstart option.
export function buildQuickstartMetadata(option: QuickstartOption) {
  return {
    title: `${option.name} - Spacewave`,
    description: option.seoDescription ?? option.description,
    canonicalPath: `/quickstart/${option.id}`,
    ogImage: 'https://cdn.spacewave.app/og-default.png',
  }
}
