const FOOTER_LINKS = [
  { href: '#/tos', label: 'Terms' },
  { href: '#/dmca', label: 'DMCA' },
  { href: '#/privacy', label: 'Privacy' },
]

// PageFooter renders the bottom attribution and legal links.
export function PageFooter() {
  return (
    <p className="text-foreground-alt/40 pt-4 pb-2 text-center text-xs">
      <span className="block">
        Spacewave Cloud by{' '}
        <a
          href="https://aperture.us"
          target="_blank"
          rel="noopener noreferrer"
          className="text-foreground/50 hover:text-foreground/70 transition-colors"
        >
          Aperture Robotics
        </a>
        , LLC. powered by Cloudflare
      </span>
      <span className="mt-1 block">
        {FOOTER_LINKS.map((link, index) => (
          <span key={link.href}>
            {index > 0 && <span className="mx-1.5">|</span>}
            <a
              href={link.href}
              className="text-foreground/50 hover:text-foreground/70 transition-colors"
            >
              {link.label}
            </a>
          </span>
        ))}
      </span>
    </p>
  )
}
