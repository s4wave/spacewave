import type { IconType } from '@s4wave/web/images/icons.js'
import type { ReactNode } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

interface UseCaseCtaRowProps {
  children: ReactNode
}

// UseCaseCtaRow renders a centered row of call-to-action buttons.
export function UseCaseCtaRow({ children }: UseCaseCtaRowProps) {
  return <div className="flex flex-wrap justify-center gap-3">{children}</div>
}

interface UseCaseCtaLinkProps {
  href: string
  icon: IconType
  variant?: 'primary' | 'default'
  children: ReactNode
}

// UseCaseCtaLink renders a single CTA anchor matching the landing page style.
export function UseCaseCtaLink({
  href,
  icon: Icon,
  variant = 'default',
  children,
}: UseCaseCtaLinkProps) {
  return (
    <a
      href={href}
      data-landing-primary-cta={variant === 'primary' || undefined}
      className={cn(
        'rounded-landing-control shadow-landing-button flex items-center gap-2 border px-5 py-2.5 text-sm font-medium no-underline transition-all duration-300 select-none hover:-translate-y-0.5 [@media(pointer:coarse)]:min-h-11',
        variant === 'primary'
          ? 'border-brand/40 bg-brand/10 text-foreground hover:border-brand/60 hover:bg-brand/15'
          : 'border-foreground/15 bg-background/50 text-foreground hover:border-brand/40 hover:bg-brand/8',
      )}
    >
      <Icon sx={stylexStyles.rule1} />
      <span>{children}</span>
    </a>
  )
}
