import React, { useCallback } from 'react'

import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

import AnimatedLogo from './AnimatedLogo.js'
import { ExternalLink } from './ExternalLink.js'
import GetStarted from './GetStarted.js'
import { NavigationLinks } from './NavigationLinks.js'

interface GetStartedSectionProps {
  homeRef: React.RefObject<HTMLDivElement | null>
  showScrollIndicator: boolean
  animateScrollIndicator: boolean
  scrollDown: () => void
  sessions?: SessionListEntry[]
}

export function GetStartedSection({
  homeRef,
  showScrollIndicator,
  animateScrollIndicator,
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
      {/* Spacer to center content on tall screens */}
      <div className="tall:block tall:flex-1 hidden" />

      {/* Logo and Navigation Section */}
      <div className="tall:flex-initial mb-1 flex min-h-0 flex-1 flex-col items-center gap-2 @lg:gap-3 @2xl:gap-4">
        <AnimatedLogo
          followMouse={true}
          containerClassName="very-short:hidden"
        />
        <h1 className="ultra-short:hidden text-2xl font-semibold tracking-[0.1rem] whitespace-nowrap @lg:text-3xl @lg:tracking-[0.142rem]">
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
        <div className="text-foreground-alt/60 flex flex-wrap items-center justify-center gap-x-1 text-xs">
          <span>
            Made by{' '}
            <button
              type="button"
              onClick={goToCommunity}
              className="hover:text-brand cursor-pointer underline transition-colors"
            >
              the community
            </button>
          </span>
          <span aria-hidden>·</span>
          <span>Free software</span>
          <span aria-hidden>·</span>
          <ExternalLink
            href="https://spacemacs.org"
            className="hover:text-brand underline transition-colors"
          >
            Inspired by Spacemacs
          </ExternalLink>
        </div>

        {/* Scroll indicator */}
        <button
          type="button"
          tabIndex={showScrollIndicator ? 0 : -1}
          aria-label="Scroll down to learn more"
          className={cn(
            'mt-2 mb-3 flex cursor-pointer flex-col items-center gap-0.5 transition-opacity duration-300',
            showScrollIndicator
              ? 'opacity-100'
              : 'pointer-events-none opacity-0',
            showScrollIndicator &&
              animateScrollIndicator &&
              'animate-[pulse_8s_ease-in-out_infinite]',
          )}
          onClick={scrollDown}
        >
          <span className="text-foreground-alt/60 text-[10px] tracking-wide uppercase select-none">
            Learn more
          </span>
          <span className="text-foreground-alt/60 text-xs font-bold">▼</span>
        </button>
      </div>
    </div>
  )
}
