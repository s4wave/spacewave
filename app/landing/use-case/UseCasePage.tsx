import type { ComponentType, ReactElement, ReactNode } from 'react'

import { LegalPageLayout } from '../LegalPageLayout.js'
import { UseCaseCtaLink, UseCaseCtaRow } from '../UseCaseCtaRow.js'

// UseCasePoint is one short statement about what the demo showed.
export interface UseCasePoint {
  title: string
  body: string
}

// UseCaseAction is a call to action below the demo.
export interface UseCaseAction {
  href: string
  label: string
  icon: ComponentType<{ className?: string }>
}

// UseCasePageProps configures a use-case landing page.
export interface UseCasePageProps {
  icon: ReactElement
  title: string
  subtitle: string
  children: ReactNode
  points: UseCasePoint[]
  keepTitle: string
  keepBody: string
  primary: UseCaseAction
  secondary?: UseCaseAction
}

// UseCasePage renders a use-case landing page: the job, the product doing it
// (children, usually a UseCaseDemo), what that showed, and how to keep it.
export function UseCasePage({
  icon,
  title,
  subtitle,
  children,
  points,
  keepTitle,
  keepBody,
  primary,
  secondary,
}: UseCasePageProps) {
  return (
    <LegalPageLayout icon={icon} title={title} subtitle={subtitle}>
      {children}

      <section className="relative z-10 mx-auto grid w-full max-w-6xl gap-8 px-4 pt-20 @lg:grid-cols-3 @lg:px-8">
        {points.map((point) => (
          <div key={point.title} className="flex flex-col gap-2">
            <h3 className="text-foreground text-base font-semibold">
              {point.title}
            </h3>
            <p className="text-foreground-alt text-sm leading-relaxed">
              {point.body}
            </p>
          </div>
        ))}
      </section>

      <section className="relative z-10 mx-auto flex w-full max-w-2xl flex-col items-center gap-5 px-4 pt-20 pb-24 text-center @lg:px-8">
        <h2 className="text-foreground text-2xl font-semibold tracking-tight">
          {keepTitle}
        </h2>
        <p className="text-foreground-alt text-sm leading-relaxed @lg:text-base">
          {keepBody}
        </p>
        <UseCaseCtaRow>
          <UseCaseCtaLink
            href={primary.href}
            icon={primary.icon}
            variant="primary"
          >
            {primary.label}
          </UseCaseCtaLink>
          {secondary && (
            <UseCaseCtaLink href={secondary.href} icon={secondary.icon}>
              {secondary.label}
            </UseCaseCtaLink>
          )}
        </UseCaseCtaRow>
      </section>
    </LegalPageLayout>
  )
}
