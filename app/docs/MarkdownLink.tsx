import type { ComponentPropsWithoutRef } from 'react'

import { ExternalLink } from '@s4wave/app/landing/ExternalLink.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

function isExternalHref(href: string): boolean {
  return (
    /^https?:\/\//i.test(href) &&
    !/^https?:\/\/([^/]+\.)?spacewave\.app(\/|$)/i.test(href)
  )
}

// MarkdownLink resolves app-local markdown links to static or hash hrefs.
export function MarkdownLink({
  href,
  ...props
}: ComponentPropsWithoutRef<'a'>) {
  const staticHref = useStaticHref(href ?? '')
  const resolvedHref =
    href && href.startsWith('/') && !href.startsWith('//') ? staticHref : href

  if (resolvedHref && isExternalHref(resolvedHref)) {
    return <ExternalLink {...props} href={resolvedHref} />
  }

  return <a {...props} href={resolvedHref} />
}
