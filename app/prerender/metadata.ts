import { STATIC_ROUTES } from './static-pages.js'

export interface PageMetadata {
  title: string
  description: string
  canonicalPath?: string
  ogImage?: string
  ogType?: string
  twitterCard?: string
  jsonLd?: object
}

// PAGE_METADATA maps prerendered pathnames to their route metadata; the
// inventory itself lives in STATIC_ROUTES.
export const PAGE_METADATA: Record<string, PageMetadata> = Object.fromEntries(
  STATIC_ROUTES.map((route) => [route.path, route.metadata]),
)

export function getMetadata(path: string): PageMetadata {
  return PAGE_METADATA[path] ?? { title: 'Spacewave', description: '' }
}
