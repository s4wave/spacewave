import { BrowserStartupPhaseRail } from '@s4wave/app/loading/BrowserStartupPhaseRail.js'
import { useBrowserStartupProjection } from '@s4wave/app/loading/status/browser-startup.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { usePath } from '@s4wave/web/router/router.js'
import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'

import { PUBLIC_QUICKSTART_OPTIONS, type QuickstartOption } from './options.js'
import { QuickstartUnavailable } from './QuickstartUnavailable.js'

// QuickstartLoading presents browser preparation until hydration hands the
// route to the app. The startup projection retains diagnostic progress.
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
        state: startup.view.state,
        title:
          startup.view.state === 'error'
            ? 'Unable to open your space'
            : option.id === 'drive'
              ? 'Preparing your Drive'
              : 'Preparing your workspace',
        detail: startup.view.detail,
        error: startup.view.error,
        onRetry: startup.view.onRetry,
      }}
      footer={<a href={landingHref}>Back to home</a>}
    >
      <BrowserStartupPhaseRail phases={startup.phases} />
    </LoadingScreen>
  )
}

// buildQuickstartMetadata generates page metadata for a quickstart option.
export function buildQuickstartMetadata(option: QuickstartOption) {
  return {
    title: `${option.name} - Spacewave`,
    description: option.seoDescription ?? option.description,
    canonicalPath: `/quickstart/${option.id}`,
  }
}
