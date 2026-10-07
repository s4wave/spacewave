import {
  lazy,
  Suspense,
  useMemo,
  type ComponentType,
  type ReactNode,
} from 'react'
import { useBldrContext } from '@aptre/bldr-react'

/** FrontendStartup loads editable startup UI after the document connects. */
export function FrontendStartup({ fallback }: { fallback?: ReactNode }) {
  const webDocument = useBldrContext()?.webDocument
  const Startup = useMemo(
    () =>
      lazy(async () => {
        if (!webDocument) {
          throw new Error('A live startup view requires a Bldr document')
        }
        const path = await webDocument.resolveFrontend(
          globalThis.__bldrFrontendStartup!,
        )
        return (await import(/* @vite-ignore */ path)) as {
          default: ComponentType
        }
      }),
    [webDocument],
  )
  return (
    <Suspense fallback={fallback}>
      <Startup />
    </Suspense>
  )
}
