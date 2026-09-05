import { BrowserStartupPhaseRail } from '@s4wave/app/loading/BrowserStartupPhaseRail.js'
import { useBrowserStartupProjection } from '@s4wave/app/loading/status/browser-startup.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { usePath } from '@s4wave/web/router/router.js'
import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'

import { PUBLIC_QUICKSTART_OPTIONS, type QuickstartOption } from './options.js'
import { QuickstartUnavailable } from './QuickstartUnavailable.js'

// QuickstartLoading is a static prerendered page for /quickstart/{id}.
// Shows the quickstart metadata with a loading indicator. When the
// entrypoint finishes background boot (WASM ready), hydrate.tsx
// auto-transitions to the full app at #/quickstart/{id}.
export function QuickstartLoading() {
  const path = usePath()
  const id = path.split('/').pop() ?? ''
  const option = PUBLIC_QUICKSTART_OPTIONS.find((o) => o.id === id)
  const landingHref = useStaticHref('/')
  const startup = useBrowserStartupProjection()

  if (!option) {
    return <QuickstartUnavailable quickstartId={id} homeHref={landingHref} />
  }

  return (
    <LoadingScreen
      view={{
        ...startup.view,
        title: option.name,
      }}
      logo={<QuickstartIcon option={option} />}
      showShineBorder={false}
    >
      <div className="flex w-[min(30rem,calc(100vw-2rem))] flex-col items-center gap-5 text-center">
        <p className="text-foreground-alt/70 max-w-md text-sm leading-relaxed">
          {option.description}
        </p>
        <BrowserStartupPhaseRail phases={startup.phases} />
        <a
          href={landingHref}
          className="text-foreground-alt hover:text-foreground text-sm transition-colors motion-reduce:transition-none"
        >
          Back to home
        </a>
      </div>
    </LoadingScreen>
  )
}

function QuickstartIcon(props: { option: QuickstartOption }) {
  const Icon = props.option.icon
  return (
    <div className="flex size-16 items-center justify-center rounded-2xl bg-[var(--color-neutral-900)]">
      <Icon className="size-8 text-[var(--color-neutral-300)]" />
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
