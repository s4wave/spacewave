import * as stylex from '@stylexjs/stylex'
import type { ComponentPropsWithoutRef } from 'react'

import { stylexProps } from '@s4wave/web/style/props.js'

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
  sx,
  href,
  ...props
}: ComponentPropsWithoutRef<'a'> & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const staticHref = useStaticHref(href ?? '')
  const resolvedHref =
    href && href.startsWith('/') && !href.startsWith('//') ? staticHref : href

  if (resolvedHref && isExternalHref(resolvedHref)) {
    return <ExternalLink sx={sx} {...props} href={resolvedHref} />
  }

  return <a {...stylexProps([sx], props)} href={resolvedHref} />
}
