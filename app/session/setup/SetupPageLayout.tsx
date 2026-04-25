import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { cn } from '@s4wave/web/style/utils.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

// SetupPageLayoutProps configures the shared setup page layout.
export interface SetupPageLayoutProps {
  title: string
  subtitle?: string
  maxWidth?: string
  topLeft?: React.ReactNode
  children: React.ReactNode
}

// SetupPageLayout renders the shared outer layout for setup pages: shooting
// stars background, centered content column, animated logo, title, subtitle.
export function SetupPageLayout({
  title,
  subtitle,
  maxWidth = 'max-w-md',
  topLeft,
  children,
}: SetupPageLayoutProps) {
  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 overflow-y-auto p-6 outline-none md:p-10">
      <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />
      {topLeft && <div className="absolute top-4 left-4 z-20">{topLeft}</div>}
      <div className={cn('relative z-10 flex w-full flex-col gap-6', maxWidth)}>
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
          <h1 className="text-xl font-bold tracking-wide">{title}</h1>
          {subtitle && (
            <p className="text-foreground-alt text-sm">{subtitle}</p>
          )}
        </div>
        {children}
      </div>
    </div>
  )
}
