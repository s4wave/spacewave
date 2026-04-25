import { useCallback, useRef, useEffect, useState } from 'react'

import { QuickstartCommands } from '@s4wave/app/quickstart/QuickstartCommands.js'
import {
  getQuickstartPath,
  type QuickstartOption,
} from '@s4wave/app/quickstart/options.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import LandingContent from './LandingContent.js'

export const metadata = {
  title: 'Spacewave - Self-host anything in the browser',
  description:
    'Free, open-source, local-first platform for file sync, encrypted messaging, device management, and plugins. End-to-end encrypted. Runs in your browser.',
  canonicalPath: '/',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
  jsonLd: {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'Organization',
        name: 'Aperture Robotics',
        url: 'https://spacewave.app',
        logo: 'https://spacewave.app/images/spacewave-icon.png',
        sameAs: [
          'https://github.com/aperturerobotics',
          'https://discord.gg/KJutMESRsT',
        ],
      },
      {
        '@type': 'WebSite',
        name: 'Spacewave',
        url: 'https://spacewave.app',
        description: 'Self-host anything in the browser.',
      },
      {
        '@type': 'WebApplication',
        name: 'Spacewave',
        url: 'https://spacewave.app',
        applicationCategory: 'DeveloperApplication',
        operatingSystem: 'Any',
        browserRequirements: 'Chrome, Firefox, Safari, Edge',
        offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
      },
      {
        '@type': 'FAQPage',
        mainEntity: [
          {
            '@type': 'Question',
            name: 'How is Spacewave different from traditional cloud apps?',
            acceptedAnswer: {
              '@type': 'Answer',
              text: "Spacewave fundamentally rethinks how web apps function. Instead of keeping your data locked on remote servers, Spacewave runs everything directly on your devices while giving you the freedom to store information wherever you want. It's like having all the power of the cloud with the independence of your personal devices.",
            },
          },
          {
            '@type': 'Question',
            name: 'Is my data secure when using Spacewave?',
            acceptedAnswer: {
              '@type': 'Answer',
              text: "Absolutely. Spacewave uses end-to-end encryption and optionally peer-to-peer (P2P) networking. Your data is encrypted on your device and stays private in transport and in storage. Even we can't decrypt it.",
            },
          },
          {
            '@type': 'Question',
            name: 'How does Spacewave handle data portability?',
            acceptedAnswer: {
              '@type': 'Answer',
              text: "Spacewave prioritizes data portability, giving you full control over your information. With just a few clicks, you can move your data between different storage options including your own devices and the cloud. This flexibility ensures you're never tied to a single storage solution and can take your data with you wherever you go.",
            },
          },
          {
            '@type': 'Question',
            name: 'Why is Spacewave open source?',
            acceptedAnswer: {
              '@type': 'Answer',
              text: 'We believe in transparency, community collaboration, and user freedom. Being open source means anyone can inspect our code, contribute improvements, or run their own custom version. This creates trust, accelerates innovation, and ensures Spacewave remains focused on user needs. We invite you to make it your own!',
            },
          },
        ],
      },
    ],
  },
}
import { CornerText } from './CornerText.js'
import { GetStartedSection } from './GetStartedSection.js'
import { useIsGridMode } from '@s4wave/app/ShellContext.js'
import { useIsTabActive } from '@s4wave/app/ShellTabContext.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'

const MIN_WIDTH_CORNER_TEXT = 480
const MIN_WIDTH_CONTENT = 335

export function Landing() {
  const navigate = useNavigate()
  const homeRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const isGridMode = useIsGridMode()
  const isTabActive = useIsTabActive()
  const sessionResource = useSessionList()
  const sessions = sessionResource.value?.sessions
  const [visibility, setVisibility] = useState({
    scrollIndicator: true,
    cornerTextScroll: true,
    cornerTextWidth: true,
    tooNarrow: false,
  })

  useEffect(() => {
    const handleScroll = (event: Event) => {
      const scrollTop = (event.target as Element).scrollTop
      setVisibility((prev) => ({
        ...prev,
        scrollIndicator: scrollTop <= 100,
        cornerTextScroll: scrollTop <= 20,
      }))
    }

    const checkWidth = () => {
      const containerWidth =
        containerRef.current?.clientWidth ?? window.innerWidth
      // Use window width for corner text since it's fixed-positioned to viewport
      const windowWidth = window.innerWidth
      setVisibility((prev) => ({
        ...prev,
        cornerTextWidth: windowWidth >= MIN_WIDTH_CORNER_TEXT,
        tooNarrow: containerWidth < MIN_WIDTH_CONTENT,
      }))
    }

    checkWidth()

    if (containerRef.current) {
      const container = containerRef.current
      container.addEventListener('scroll', handleScroll)
      window.addEventListener('resize', checkWidth)

      const resizeObserver = new ResizeObserver(checkWidth)
      resizeObserver.observe(container)

      return () => {
        container.removeEventListener('scroll', handleScroll)
        window.removeEventListener('resize', checkWidth)
        resizeObserver.disconnect()
      }
    }
  }, [])

  const scrollDown = useCallback(() => {
    const homeHeight = homeRef.current?.clientHeight || 0
    containerRef.current?.scrollTo({
      top: homeHeight,
      behavior: 'smooth',
    })
  }, [])

  const handleQuickstartCommand = useCallback(
    (opt: QuickstartOption) => {
      navigate({ path: getQuickstartPath(opt) })
    },
    [navigate],
  )

  return (
    <div
      ref={containerRef}
      className="bg-background-landing @container relative flex w-full flex-1 flex-col overflow-auto"
    >
      <QuickstartCommands onQuickstart={handleQuickstartCommand} />
      {visibility.tooNarrow && (
        <div className="bg-background/95 pointer-events-none absolute inset-0 z-50 flex flex-col items-center justify-center text-sm">
          <span className="text-foreground-alt animate-[pulse_2s_ease-in-out_infinite]">
            &larr; Wider please! &rarr;
          </span>
        </div>
      )}

      <CornerText
        show={
          visibility.cornerTextScroll &&
          visibility.cornerTextWidth &&
          !isGridMode &&
          isTabActive
        }
      />

      <GetStartedSection
        homeRef={homeRef}
        showScrollIndicator={visibility.scrollIndicator}
        scrollDown={scrollDown}
        sessions={sessions}
      />

      <LandingContent />
    </div>
  )
}
