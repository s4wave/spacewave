import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

// LegalFooter renders the footer for legal pages (ToS, Privacy, Pricing, DMCA).
export function LegalFooter() {
  const tosHref = useStaticHref('/tos')
  const privacyHref = useStaticHref('/privacy')
  const dmcaHref = useStaticHref('/dmca')
  const pricingHref = useStaticHref('/pricing')
  const downloadHref = useStaticHref('/download')
  const licensesHref = useStaticHref('/licenses')
  const blogHref = useStaticHref('/blog')

  return (
    <footer className="relative z-10 mt-auto border-t border-white/10 px-4 py-6">
      <div className="mx-auto flex max-w-4xl flex-col items-center gap-3 sm:flex-row">
        <p className="text-foreground-alt/50 text-xs">
          &copy; 2018&ndash;2026{' '}
          <a
            href="https://github.com/aperturerobotics"
            className="font-medium text-white/80 transition-colors hover:text-white"
          >
            Aperture Robotics
          </a>
          , LLC. and contributors
        </p>
        <nav className="flex gap-4 sm:ml-auto">
          <a
            href={tosHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Terms
          </a>
          <a
            href={privacyHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Privacy
          </a>
          <a
            href={dmcaHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            DMCA
          </a>
          <a
            href={pricingHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Pricing
          </a>
          <a
            href={downloadHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Download
          </a>
          <a
            href={licensesHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Licenses
          </a>
          <a
            href={blogHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Blog
          </a>
        </nav>
      </div>
    </footer>
  )
}
