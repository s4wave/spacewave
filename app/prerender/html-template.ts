import { FUNNEL_BEACON_SCRIPT } from './funnel-beacon.js'
import { serializeJsonScriptData } from './json-script.js'

interface PageHtmlOptions {
  body: string
  title: string
  description: string
  canonicalUrl: string
  ogImage: string
  ogType?: string
  twitterCard?: string
  // robots overrides the default "index, follow" crawler directive.
  robots?: string
  jsonLd?: object
  themeColor?: string
  headScript?: string
  bootstrapScript: string
  hydrateScript?: string
  criticalCss: string
  mainCssUrl: string
  additionalCssUrls?: string[]
  iconUrl: string
  importMap?: string
  // When false, omits data-prerendered from bldr-root so the bldr
  // entrypoint uses createRoot instead of hydrateRoot.
  prerendered?: boolean
}

// escapeHtml escapes text for an HTML text node or quoted attribute value.
function escapeHtml(text: string): string {
  return text
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
}

// buildPageHtml builds the complete HTML document for a pre-rendered page,
// including the funnel beacon. Every page names its canonical URL and link
// preview image.
export function buildPageHtml(opts: PageHtmlOptions): string {
  const title = escapeHtml(opts.title)
  const description = escapeHtml(opts.description)
  const canonicalUrl = escapeHtml(opts.canonicalUrl)
  const ogImage = escapeHtml(opts.ogImage)
  const ogType = opts.ogType ?? 'website'
  const twitterCard = opts.twitterCard ?? 'summary_large_image'
  const themeColor = opts.themeColor ?? '#0a0a0a'
  const robots = opts.robots ?? 'index, follow'

  const jsonLdTag = opts.jsonLd
    ? `\n  <script type="application/ld+json">${serializeJsonScriptData(opts.jsonLd)}</script>`
    : ''
  const headScript = opts.headScript ? `\n  ${opts.headScript}` : ''
  const criticalStyle = opts.criticalCss
    ? `\n  <style>${opts.criticalCss}</style>`
    : ''
  const importMapTag = opts.importMap
    ? `\n  <script type="importmap">${opts.importMap}</script>`
    : ''
  const stylesheetTags = [opts.mainCssUrl, ...(opts.additionalCssUrls ?? [])]
    .map(
      (cssUrl) =>
        `\n  <link rel="preload" href="${cssUrl}" as="style"/>\n  <link rel="stylesheet" href="${cssUrl}"/>`,
    )
    .join('')

  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <meta name="darkreader-lock"/>
  <title>${title}</title>
  <meta name="description" content="${description}"/>
  <meta name="robots" content="${robots}"/>
  <link rel="canonical" href="${canonicalUrl}"/>
  <link rel="icon" href="${opts.iconUrl}"/>
  <link rel="apple-touch-icon" href="${opts.iconUrl}"/>
  <meta name="theme-color" content="${themeColor}"/>
  <meta property="og:type" content="${ogType}"/>
  <meta property="og:site_name" content="Spacewave"/>
  <meta property="og:locale" content="en_US"/>
  <meta property="og:title" content="${title}"/>
  <meta property="og:description" content="${description}"/>
  <meta property="og:url" content="${canonicalUrl}"/>
  <meta property="og:image" content="${ogImage}"/>
  <meta name="twitter:card" content="${twitterCard}"/>
  <meta name="twitter:title" content="${title}"/>
  <meta name="twitter:description" content="${description}"/>
  <meta name="twitter:image" content="${ogImage}"/>${headScript}${jsonLdTag}${criticalStyle}${importMapTag}${stylesheetTags}
</head>
<body>
  <div id="bldr-root"${opts.prerendered !== false ? ' data-prerendered="true"' : ''} role="main">${opts.body}</div>
  ${opts.bootstrapScript}
  ${opts.hydrateScript ?? ''}
  ${FUNNEL_BEACON_SCRIPT}
</body>
</html>`
}
