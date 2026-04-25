import { LuArrowLeft } from 'react-icons/lu'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { LegalFooter } from './LegalFooter.js'
import { useLandingBackNavigation } from './useLandingBackNavigation.js'

// LegalPageLayoutProps defines the props for LegalPageLayout.
interface LegalPageLayoutProps {
  icon?: React.ReactElement
  title: string
  subtitle?: string
  children: React.ReactNode
  lastUpdated?: string
  draftBanner?: boolean
}

// LegalPageLayout renders the shared layout for legal and informational pages.
export function LegalPageLayout({
  icon,
  title,
  subtitle,
  children,
  lastUpdated,
  draftBanner,
}: LegalPageLayoutProps) {
  const goBack = useLandingBackNavigation()

  return (
    <div className="bg-background-landing @container flex w-full flex-1 flex-col overflow-y-auto">
      <ShootingStars className="pointer-events-none fixed inset-0 opacity-40" />

      <div className="relative z-10 px-4 pt-6 @lg:px-8">
        <button
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground flex cursor-pointer items-center gap-2 text-sm transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          Back
        </button>
      </div>

      <header className="relative z-10 mx-auto w-full max-w-4xl px-4 pt-14 pb-14 text-center @lg:px-8 @lg:pt-20 @lg:pb-16">
        {icon && (
          <div className="text-brand mb-6 flex items-center justify-center gap-2">
            {icon}
          </div>
        )}
        <h1 className="text-foreground mb-6 text-4xl font-bold tracking-tight @lg:text-5xl">
          {title}
        </h1>
        {subtitle && (
          <p className="text-foreground-alt mx-auto max-w-xl text-base leading-relaxed @lg:text-lg">
            {subtitle}
          </p>
        )}
      </header>

      {(lastUpdated || draftBanner) && (
        <div className="relative z-10 mx-auto w-full max-w-4xl px-4 @lg:px-8">
          {lastUpdated && (
            <p className="text-foreground-alt mb-2 text-center text-xs">
              {lastUpdated}
            </p>
          )}
          {draftBanner && (
            <>
              <p className="text-foreground-alt mb-6 text-center text-xs">
                Effective Date: TBD
              </p>
              <div className="mb-6 rounded-lg border border-yellow-500/30 bg-yellow-500/10 p-4 text-center">
                <p className="text-sm font-medium text-yellow-400">DRAFT</p>
              </div>
            </>
          )}
        </div>
      )}

      {children}

      <LegalFooter />
    </div>
  )
}
