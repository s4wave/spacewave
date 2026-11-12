import {
  Suspense,
  use,
  useMemo,
  type ComponentType,
  type ReactNode,
} from 'react'
import type { WebDocument } from '@aptre/bldr'
import { useBldrContext } from '@aptre/bldr-react'

/** FrontendStartup loads editable startup UI after the document connects. */
export function FrontendStartup({ fallback }: { fallback?: ReactNode }) {
  const webDocument = useBldrContext()?.webDocument
  const startup = useMemo(() => loadStartup(webDocument), [webDocument])
  return (
    <Suspense fallback={fallback}>
      <StartupView startup={startup} />
    </Suspense>
  )
}

/** loadStartup imports the startup module once the document serves it. */
async function loadStartup(
  webDocument: WebDocument | undefined,
): Promise<ComponentType> {
  if (!webDocument) {
    throw new Error('A live startup view requires a Bldr document')
  }
  const path = await webDocument.resolveFrontend(
    globalThis.__bldrFrontendStartup!,
  )
  const mod = (await import(/* @vite-ignore */ path)) as {
    default: ComponentType
  }
  return mod.default
}

/** StartupView suspends until the startup module loads. */
function StartupView({ startup }: { startup: Promise<ComponentType> }) {
  return <StartupMount view={use(startup)} />
}

/** StartupMount renders the loaded startup component. */
function StartupMount({ view: View }: { view: ComponentType }) {
  return <View />
}
