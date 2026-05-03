// pluginPathPrefix is the URL path prefix for the spacewave-core plugin.
// The web runtime strips this before the handler receives the path.
export const pluginPathPrefix = '/p/spacewave-core'

// SPACEWAVE_PUBLIC_BASE_URL is the canonical public origin for the hosted
// spacewave web app. Used to build shareable URLs from desktop or alternate
// hosts where window.location does not point at the public app.
export const SPACEWAVE_PUBLIC_BASE_URL = 'https://spacewave.app'
