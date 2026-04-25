interface PageHtmlOptions {
  body: string
  title: string
  description: string
  canonicalUrl?: string
  ogImage?: string
  ogType?: string
  ogSiteName?: string
  twitterCard?: string
  jsonLd?: object
  themeColor?: string
  bootstrapScript: string
  hydrateScript?: string
  criticalCss: string
  mainCssUrl: string
  iconUrl: string
  importMap?: string
  // When false, omits data-prerendered from bldr-root so the bldr
  // entrypoint uses createRoot instead of hydrateRoot.
  prerendered?: boolean
}

// buildPageHtml builds the complete HTML document for a pre-rendered page.
export function buildPageHtml(opts: PageHtmlOptions): string {
  const ogType = opts.ogType ?? 'website'
  const twitterCard = opts.twitterCard ?? 'summary_large_image'
  const themeColor = opts.themeColor ?? '#0a0a0a'

  const canonicalTag =
    opts.canonicalUrl ?
      `\n  <link rel="canonical" href="${opts.canonicalUrl}"/>`
    : ''
  const ogImageTag =
    opts.ogImage ?
      `\n  <meta property="og:image" content="${opts.ogImage}"/>`
    : ''
  const twitterImageTag =
    opts.ogImage ?
      `\n  <meta name="twitter:image" content="${opts.ogImage}"/>`
    : ''
  const ogUrlTag =
    opts.canonicalUrl ?
      `\n  <meta property="og:url" content="${opts.canonicalUrl}"/>`
    : ''
  const jsonLdTag =
    opts.jsonLd ?
      `\n  <script type="application/ld+json">${JSON.stringify(opts.jsonLd)}</script>`
    : ''
  const criticalStyle =
    opts.criticalCss ? `\n  <style>${opts.criticalCss}</style>` : ''
  const importMapTag =
    opts.importMap ?
      `\n  <script type="importmap">${opts.importMap}</script>`
    : ''

  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <title>${opts.title}</title>
  <meta name="description" content="${opts.description}"/>
  <meta name="robots" content="index, follow"/>${canonicalTag}
  <link rel="icon" href="/favicon.ico" type="image/x-icon"/>
  <link rel="apple-touch-icon" href="${opts.iconUrl}"/>
  <meta name="theme-color" content="${themeColor}"/>
  <meta property="og:type" content="${ogType}"/>
  <meta property="og:site_name" content="Spacewave"/>
  <meta property="og:title" content="${opts.title}"/>
  <meta property="og:description" content="${opts.description}"/>${ogUrlTag}${ogImageTag}
  <meta name="twitter:card" content="${twitterCard}"/>
  <meta name="twitter:title" content="${opts.title}"/>
  <meta name="twitter:description" content="${opts.description}"/>${twitterImageTag}${jsonLdTag}${criticalStyle}${importMapTag}
  <link rel="preload" href="${opts.mainCssUrl}" as="style"/>
  <link rel="stylesheet" href="${opts.mainCssUrl}"/>
</head>
<body>
  <div id="bldr-root"${opts.prerendered !== false ? ' data-prerendered="true"' : ''} role="main">${opts.body}</div>
  ${opts.bootstrapScript}
  ${opts.hydrateScript ?? ''}
</body>
</html>`
}
