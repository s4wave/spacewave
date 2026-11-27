// pluginPathPrefix is the URL path prefix for the spacewave-core plugin.
// The web runtime strips this prefix before the handler receives the path.
export const pluginPathPrefix = '/p/spacewave-core'

// SPACEWAVE_PUBLIC_BASE_URL is the canonical public origin for the hosted
// spacewave web app. The origin is used to build shareable URLs from desktop
// or alternate hosts where window.location does not point at the public app.
export const SPACEWAVE_PUBLIC_BASE_URL = 'https://spacewave.app'

// buildInviteLink builds a Space invite URL that can be shared out of band.
// publicBaseUrl is the configured cloud public origin (e.g. from
// CloudProviderConfig.publicBaseUrl). When publicBaseUrl is absent, the
// function falls back to the canonical hosted origin. window.location.origin
// is unsuitable here because the desktop shell loads the app at
// app://index.html.
export function buildInviteLink(
  publicBaseUrl: string | undefined,
  encoded: string,
): string {
  const base = (publicBaseUrl || SPACEWAVE_PUBLIC_BASE_URL).replace(/\/+$/, '')
  return `${base}/#/join/${encoded}`
}
