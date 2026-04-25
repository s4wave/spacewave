import type { ComponentPropsWithoutRef } from 'react'

import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

// MarkdownLink resolves app-local markdown links to static or hash hrefs.
export function MarkdownLink({
  href,
  ...props
}: ComponentPropsWithoutRef<'a'>) {
  const staticHref = useStaticHref(href ?? '')
  const resolvedHref =
    href && href.startsWith('/') && !href.startsWith('//') ? staticHref : href

  return <a {...props} href={resolvedHref} />
}
