import React, { useCallback } from 'react'
import AnimatedLogo from './AnimatedLogo.js'
import GetStarted from './GetStarted.js'
import { NavigationLinks } from './NavigationLinks.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'

interface GetStartedSectionProps {
  homeRef: React.RefObject<HTMLDivElement | null>
  showScrollIndicator: boolean
  scrollDown: () => void
  sessions?: SessionListEntry[]
}

export function GetStartedSection({
  homeRef,
  showScrollIndicator,
  scrollDown,
  sessions,
}: GetStartedSectionProps) {
  const navigate = useNavigate()
  const goToCommunity = useCallback(() => {
    navigate({ path: '/community' })
  }, [navigate])
  return (
    <div
      ref={homeRef}
      className="relative flex min-h-full w-full flex-col pt-6 @lg:pt-8 @2xl:pt-[2.84rem]"
    >
      <ShootingStars className="pointer-events-none absolute inset-0" />

      {/* Spacer to center content on tall screens */}
      <div className="tall:block tall:flex-1 hidden" />

      {/* Logo and Navigation Section */}
      <div className="tall:flex-initial mb-1 flex min-h-0 flex-1 flex-col items-center gap-2 @lg:gap-3 @2xl:gap-4">
        <AnimatedLogo
          followMouse={true}
          containerClassName="very-short:hidden"
        />
        <h1 className="ultra-short:hidden text-2xl font-bold tracking-[0.1rem] whitespace-nowrap @lg:text-3xl @lg:tracking-[0.142rem]">
          [SPACEWAVE]
        </h1>

        <NavigationLinks />

        <div className="tall:flex-initial flex min-h-0 w-full max-w-2xl flex-1 flex-col gap-4 px-4 text-sm @lg:gap-6 @lg:px-8">
          <GetStarted className="relative z-10" sessions={sessions} />

          {/* Description Section */}
          <div className="text-foreground-alt text-center">
            <p className="text-xs">
              local-first, end-to-end encrypted,{' '}
              <span className="text-white">no account required</span>
            </p>
          </div>
        </div>
      </div>

      {/* Bottom spacer to center content on tall screens */}
      <div className="tall:block tall:flex-1 hidden" />

      {/* Footer content pinned to bottom - fades out on short screens to avoid overlap */}
      <div className="short:opacity-0 mt-auto flex flex-shrink-0 flex-col items-center pt-4 text-center transition-opacity duration-300 @lg:pt-6">
        <div className="text-foreground-alt flex flex-col items-center justify-center space-y-1 text-xs @lg:space-y-2">
          <p>
            Made with ❤️ by{' '}
            <button
              onClick={goToCommunity}
              className="text-foreground hover:text-brand cursor-pointer underline transition-colors"
            >
              the community
            </button>
          </p>
          <p className="text-foreground font-semibold tracking-wide">
            Proudly free software
          </p>
          <p className="text-[10px]">
            <a
              href="https://spacemacs.org"
              target="_blank"
              rel="noopener noreferrer"
              className="text-brand underline"
            >
              Inspired by Spacemacs
            </a>
          </p>
        </div>

        {/* Scroll indicator */}
        <div
          className={cn(
            'mt-2 mb-3 flex cursor-pointer flex-col items-center transition-opacity duration-300',
            showScrollIndicator ?
              'animate-[pulse_8s_ease-in-out_infinite] opacity-100'
            : 'pointer-events-none opacity-0',
          )}
          onClick={scrollDown}
        >
          <span className="text-foreground-alt/60 text-xs font-bold">▼</span>
        </div>
      </div>
    </div>
  )
}
