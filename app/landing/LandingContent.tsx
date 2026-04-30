import React, { useCallback, useState, useMemo } from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import { useScrollReveal } from './useScrollReveal.js'
import {
  LuBookOpen,
  LuCheck,
  LuCloudOff,
  LuCircuitBoard,
  LuCode,
  LuCpu,
  LuCreditCard,
  LuDatabase,
  LuDownload,
  LuEyeOff,
  LuFolderSync,
  LuGift,
  LuGithub,
  LuGlobe,
  LuLaptop,
  LuLock,
  LuMessageSquare,
  LuMousePointer,
  LuPlus,
  LuRocket,
  LuServer,
  LuShield,
  LuSmartphone,
  LuTerminal,
  LuUsers,
  LuUserX,
  LuWifiOff,
  LuX,
  LuZap,
} from 'react-icons/lu'
import { PiRocketLaunchDuotone, PiAppStoreLogoBold } from 'react-icons/pi'
import { isDesktop } from '@aptre/bldr'
import { GITHUB_REPO_URL } from '@s4wave/app/github.js'
import { useNavLinks } from '@s4wave/app/nav-links.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

// Section provides consistent section styling with optional scroll-reveal.
function Section({
  children,
  className,
  id,
  withTopSeparator = false,
  reveal = false,
}: {
  children: React.ReactNode
  className?: string
  id?: string
  withTopSeparator?: boolean
  reveal?: boolean
}) {
  const { ref, visible } = useScrollReveal(0.08)
  return (
    <section
      id={id}
      ref={reveal ? ref : undefined}
      className={cn(
        'relative w-full px-4 py-20 @lg:px-8 @2xl:px-12',
        reveal &&
          cn(
            'transition-all duration-700',
            visible ? 'translate-y-0 opacity-100' : 'translate-y-8 opacity-0',
          ),
        className,
      )}
    >
      {withTopSeparator && (
        <div className="via-foreground/10 absolute top-0 right-0 left-0 h-px bg-gradient-to-r from-transparent to-transparent" />
      )}
      <div className="mx-auto max-w-5xl">{children}</div>
    </section>
  )
}

// SectionLabel renders the small uppercase tracking label above section headings.
function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="text-foreground-alt mb-4 text-center text-xs font-semibold tracking-widest uppercase">
      {children}
    </div>
  )
}

// SectionTitle renders a section heading.
function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="text-foreground mb-4 text-center text-3xl font-bold @lg:text-4xl">
      {children}
    </h2>
  )
}

// SectionSubtitle renders subtitle text under a heading.
function SectionSubtitle({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-foreground-alt mx-auto mb-12 max-w-2xl text-center text-sm leading-relaxed @lg:text-base">
      {children}
    </p>
  )
}

// CtaButton renders a call-to-action button.
function CtaButton({
  icon: Icon,
  children,
  variant = 'default',
  onClick,
}: {
  icon: React.ComponentType<{ className?: string }>
  children: React.ReactNode
  variant?: 'default' | 'primary'
  onClick?: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex cursor-pointer items-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none hover:-translate-y-0.5',
        variant === 'primary' ?
          'border-brand/40 bg-brand/10 text-foreground hover:border-brand/60 hover:bg-brand/15'
        : 'border-foreground/15 bg-background/50 text-foreground hover:border-brand/40 hover:bg-brand/8',
      )}
    >
      <Icon className="h-4 w-4" />
      <span>{children}</span>
    </button>
  )
}

// CtaRow renders a centered row of call-to-action buttons.
function CtaRow({ children }: { children: React.ReactNode }) {
  return (
    <div className="mt-10 flex flex-wrap justify-center gap-3">{children}</div>
  )
}

interface SectionHeadingProps {
  children: React.ReactNode
}

const SectionHeading: React.FC<SectionHeadingProps> = ({ children }) => (
  <h2 className="text-foreground mb-8 text-center text-3xl font-bold @lg:text-4xl">
    {children}
  </h2>
)

// Hero Components
interface HeroTextProps {
  className?: string
  children: React.ReactNode
}

const HeroText: React.FC<HeroTextProps> = ({ className, children }) => (
  <p
    className={cn(
      'text-foreground-alt mx-auto max-w-2xl text-sm leading-relaxed font-light @lg:text-base @lg:leading-relaxed',
      className,
    )}
  >
    {children}
  </p>
)

interface HeroButtonProps {
  icon: React.ReactNode
  children: React.ReactNode
  onClick?: () => void
}

const HeroButton: React.FC<HeroButtonProps> = ({ icon, children, onClick }) => (
  <button className="hero-button" onClick={onClick}>
    {icon}
    <span className="select-none">{children}</span>
  </button>
)

interface HeroFeatureProps {
  icon: React.ReactNode
  text: string
  color?: 'primary' | 'emerald' | 'pink' | 'purple' | 'amber'
}

const HeroFeature: React.FC<HeroFeatureProps> = ({
  icon,
  text,
  color = 'primary',
}) => {
  const colorClasses = {
    primary: 'text-brand',
    emerald: 'text-green-400',
    pink: 'text-rose-400',
    purple: 'text-blue-400',
    amber: 'text-yellow-400',
  } as const

  return (
    <li className="group flex items-center transition-all duration-300 ease-in-out select-none hover:-translate-y-[1px]">
      <span
        className={cn(
          colorClasses[color],
          'mr-3 transition-transform duration-300 group-hover:scale-110',
        )}
      >
        {icon}
      </span>
      <span className="text-foreground group-hover:text-brand font-medium whitespace-nowrap drop-shadow transition-colors duration-300 select-none">
        {text}
      </span>
    </li>
  )
}

// FAQ Components
interface FaqItemProps {
  question: React.ReactNode
  answer: React.ReactNode
  isOpen: boolean
  onToggle: () => void
}

const FaqItem: React.FC<FaqItemProps> = ({
  question,
  answer,
  isOpen,
  onToggle,
}) => {
  return (
    <div
      className={cn(
        'group cursor-pointer rounded-lg border p-5 backdrop-blur-sm transition-all',
        isOpen ?
          'border-foreground/12 bg-background-card/60'
        : 'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:-translate-y-0.5',
      )}
      onClick={onToggle}
    >
      <div className="flex items-start justify-between gap-4">
        <h3
          className={cn(
            'font-heading text-sm leading-relaxed font-semibold transition-colors @lg:text-base',
            isOpen ? 'text-foreground' : (
              'text-foreground group-hover:text-brand'
            ),
          )}
        >
          {question}
        </h3>
        <div
          className={cn(
            'mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-md transition-all',
            isOpen ?
              'bg-brand/12 text-brand rotate-45'
            : 'bg-foreground/6 text-foreground-alt group-hover:bg-brand/8 group-hover:text-brand',
          )}
        >
          <LuPlus className="h-3 w-3" />
        </div>
      </div>
      <div
        className={cn(
          'grid transition-all duration-300 ease-in-out',
          isOpen ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
        )}
      >
        <div className="overflow-hidden">
          <p className="text-foreground-alt pt-3 text-sm leading-relaxed">
            {answer}
          </p>
        </div>
      </div>
    </div>
  )
}

const FaqAccordion: React.FC = () => {
  const [openIndex, setOpenIndex] = useState<number>(0)

  const faqItems = useMemo(
    () => [
      {
        question: 'How is Spacewave different from traditional cloud apps?',
        answer: (
          <>
            Spacewave fundamentally rethinks how web apps function. Instead of
            keeping your data locked on remote servers, Spacewave runs
            everything directly on your devices while giving you the freedom to
            store information wherever you want. It's like having all the power
            of the cloud with the independence of your personal devices.
          </>
        ),
      },
      {
        question: 'Is my data secure when using Spacewave?',
        answer: (
          <>
            Absolutely. Spacewave uses end-to-end encryption and optionally
            peer-to-peer (P2P) networking. Your data is encrypted on your device
            and stays private in transport and in storage. Even we can't decrypt
            it. This is{' '}
            <a
              href="https://www.cloudflare.com/learning/privacy/what-is-end-to-end-encryption/"
              target="_blank"
              rel="noopener noreferrer"
              className="text-brand hover:text-brand-highlight underline underline-offset-2"
            >
              a standard approach to data safety
            </a>{' '}
            on the web.
          </>
        ),
      },
      {
        question: 'How does Spacewave handle data portability?',
        answer:
          "Spacewave prioritizes data portability, giving you full control over your information. With just a few clicks, you can move your data between different storage options including your own devices and the cloud. This flexibility ensures you're never tied to a single storage solution and can take your data with you wherever you go.",
      },
      {
        question: 'Why is Spacewave open source?',
        answer:
          'We believe in transparency, community collaboration, and user freedom. Being open source means anyone can inspect our code, contribute improvements, or run their own custom version. This creates trust, accelerates innovation, and ensures Spacewave remains focused on user needs. We invite you to make it your own!',
      },
    ],
    [],
  )

  const handleToggle = useCallback((index: number) => {
    setOpenIndex((prevIndex) => (prevIndex === index ? -1 : index))
  }, [])

  return (
    <div className="mx-auto max-w-3xl">
      <span className="text-foreground-alt mb-8 block text-center text-xs font-semibold tracking-[0.2em] uppercase">
        Frequently asked questions
      </span>
      <div className="flex flex-col gap-3">
        {faqItems.map((item, index) => (
          <FaqItem
            key={index}
            question={item.question}
            answer={item.answer}
            isOpen={index === openIndex}
            onToggle={() => handleToggle(index)}
          />
        ))}
      </div>
    </div>
  )
}

// Section Components
const HeroSection: React.FC = () => {
  const nav = useNavLinks()
  const blogHref = useStaticHref('/blog/2026/04/launch')
  return (
    <Section className="pt-36 pb-32" withTopSeparator>
      <div className="text-center">
        <div className="absolute inset-0 -z-10 overflow-hidden">
          <div className="bg-background absolute inset-0">
            <svg
              className="absolute h-full w-full opacity-[0.03]"
              xmlns="http://www.w3.org/2000/svg"
              width="100%"
              height="100%"
              viewBox="0 0 100 100"
            >
              <defs>
                <pattern
                  id="spacewave-pattern"
                  patternUnits="userSpaceOnUse"
                  width="50"
                  height="50"
                  patternTransform="rotate(11)"
                >
                  <circle
                    cx="25"
                    cy="25"
                    r="1"
                    fill="none"
                    stroke="currentColor"
                    className="text-background-dark"
                  />
                  <circle
                    cx="25"
                    cy="25"
                    r="6"
                    fill="none"
                    stroke="currentColor"
                    className="text-background-dark"
                    strokeWidth="0.2"
                  />
                  <circle
                    cx="25"
                    cy="25"
                    r="12"
                    fill="none"
                    stroke="currentColor"
                    className="text-background-dark"
                    strokeWidth="0.15"
                  />
                  <circle
                    cx="25"
                    cy="25"
                    r="18"
                    fill="none"
                    stroke="currentColor"
                    className="text-background-dark"
                    strokeWidth="0.1"
                  />
                </pattern>
              </defs>
              <rect
                x="0"
                y="0"
                width="100%"
                height="100%"
                fill="url(#spacewave-pattern)"
              />
            </svg>
          </div>
        </div>

        <a
          href={blogHref}
          className="border-brand/30 text-brand hover:border-brand/50 mb-8 inline-block cursor-pointer rounded-full border px-4 py-2 text-sm font-medium no-underline backdrop-blur-sm transition-all duration-300 hover:-translate-y-0.5"
        >
          <PiRocketLaunchDuotone className="mr-2 inline-block h-4 w-4 -translate-y-0.5" />
          Announcing open beta
        </a>

        <SectionHeading>The internet without the internet</SectionHeading>

        <div className="mb-8 space-y-0">
          <HeroText>
            The web was built for servers.{' '}
            <strong className="font-semibold">
              Spacewave was built for you
            </strong>
            .
          </HeroText>

          <HeroText>
            Your spaces work <strong className="font-semibold">offline</strong>,
            sync <strong className="font-semibold">instantly</strong>, and store
            data <strong className="font-semibold">anywhere</strong>.
          </HeroText>

          <HeroText>
            Devices talk <strong className="font-semibold">directly</strong>{' '}
            instead of through distant servers. That's{' '}
            <strong className="font-semibold">real freedom</strong>.
          </HeroText>
        </div>

        <div className="mx-auto mb-8 flex max-w-xl flex-col items-center">
          <ul className="mx-auto grid max-w-2xl grid-cols-1 gap-x-8 gap-y-3 text-base @lg:grid-cols-2">
            <HeroFeature
              icon={<PiAppStoreLogoBold className="h-5 w-5" />}
              text="Spaces for any purpose"
              color="emerald"
            />
            <HeroFeature
              icon={<LuMousePointer className="h-5 w-5" />}
              text="Instant live sync"
              color="pink"
            />
            <HeroFeature
              icon={<LuShield className="h-5 w-5" />}
              text="End-to-end encrypted"
              color="purple"
            />
            <HeroFeature
              icon={<LuGithub className="h-5 w-5" />}
              text="Open-source & extensible"
              color="amber"
            />
          </ul>
        </div>

        <div className="mt-2 flex flex-wrap justify-center gap-4">
          <HeroButton
            icon={<LuRocket className="mr-2 h-4 w-4" />}
            onClick={nav.getStarted}
          >
            Get started (free)
          </HeroButton>
          {!isDesktop && (
            <HeroButton
              icon={<LuDownload className="mr-2 h-4 w-4" />}
              onClick={nav.download}
            >
              Download app
            </HeroButton>
          )}
        </div>
      </div>
    </Section>
  )
}

// SVG Components

// AnimatedConnection renders an animated line between two nodes.
function AnimatedConnection({
  x1,
  y1,
  x2,
  y2,
  delay,
  color,
}: {
  x1: number
  y1: number
  x2: number
  y2: number
  delay: number
  color: string
}) {
  const length = Math.hypot(x2 - x1, y2 - y1)
  const r = (n: number) => Math.round(n * 1e4) / 1e4
  return (
    <line
      x1={x1}
      y1={y1}
      x2={x2}
      y2={y2}
      stroke={color}
      strokeWidth="1"
      strokeDasharray={`${r(length * 0.3)} ${r(length * 0.7)}`}
      opacity="0.5"
    >
      <animate
        attributeName="stroke-dashoffset"
        values={`${r(length)};0;${r(-length)}`}
        dur="4s"
        begin={`${delay}s`}
        repeatCount="indefinite"
      />
      <animate
        attributeName="opacity"
        values="0.2;0.6;0.2"
        dur="4s"
        begin={`${delay}s`}
        repeatCount="indefinite"
      />
    </line>
  )
}

// DataPacket renders an animated dot traveling along a path.
function DataPacket({
  x1,
  y1,
  x2,
  y2,
  delay,
  color,
}: {
  x1: number
  y1: number
  x2: number
  y2: number
  delay: number
  color: string
}) {
  return (
    <circle r="3" fill={color} opacity="0.8">
      <animate
        attributeName="cx"
        values={`${x1};${x2}`}
        dur="2s"
        begin={`${delay}s`}
        repeatCount="indefinite"
      />
      <animate
        attributeName="cy"
        values={`${y1};${y2}`}
        dur="2s"
        begin={`${delay}s`}
        repeatCount="indefinite"
      />
      <animate
        attributeName="opacity"
        values="0;0.9;0.9;0"
        dur="2s"
        begin={`${delay}s`}
        repeatCount="indefinite"
      />
    </circle>
  )
}

// NetworkNode renders an animated device node in the SVG topology.
function NetworkNode({
  x,
  y,
  label,
  icon: Icon,
  delay,
  color,
}: {
  x: number
  y: number
  label: string
  icon: React.ComponentType<{ className?: string }>
  delay: number
  color: string
}) {
  return (
    <g>
      <circle
        cx={x}
        cy={y}
        r="28"
        fill="none"
        stroke={color}
        strokeWidth="1"
        opacity="0.3"
      >
        <animate
          attributeName="r"
          values="28;36;28"
          dur="3s"
          begin={`${delay}s`}
          repeatCount="indefinite"
        />
        <animate
          attributeName="opacity"
          values="0.3;0.1;0.3"
          dur="3s"
          begin={`${delay}s`}
          repeatCount="indefinite"
        />
      </circle>
      <circle
        cx={x}
        cy={y}
        r="24"
        fill="var(--color-background-card)"
        stroke={color}
        strokeWidth="1.5"
      />
      <text
        x={x}
        y={y + 42}
        textAnchor="middle"
        fill="var(--color-foreground-alt)"
        fontSize="10"
        fontFamily="inherit"
      >
        {label}
      </text>
      <foreignObject x={x - 10} y={y - 10} width="20" height="20">
        <div
          className="flex h-full w-full items-center justify-center"
          style={{ color }}
        >
          <Icon className="h-4 w-4" />
        </div>
      </foreignObject>
    </g>
  )
}

// TraditionalDiagram renders the server-centric architecture.
function TraditionalDiagram({ label }: { label: string }) {
  const gray = 'var(--color-foreground)'
  const server = { x: 240, y: 55 }
  const devices = [
    { x: 100, y: 175, label: 'Laptop', icon: LuLaptop },
    { x: 240, y: 175, label: 'Phone', icon: LuSmartphone },
    { x: 380, y: 175, label: 'Pi', icon: LuCpu },
  ]

  return (
    <svg
      viewBox="0 0 480 260"
      className="h-full w-full"
      role="img"
      aria-label="Traditional cloud architecture: all devices route through a central server"
    >
      {devices.map((d, i) => (
        <g key={`conn-${i}`}>
          <AnimatedConnection
            x1={server.x}
            y1={server.y + 24}
            x2={d.x}
            y2={d.y - 24}
            delay={i * 0.7}
            color={gray}
          />
          <DataPacket
            x1={d.x}
            y1={d.y - 24}
            x2={server.x}
            y2={server.y + 24}
            delay={i * 1.1}
            color={gray}
          />
          <DataPacket
            x1={server.x}
            y1={server.y + 24}
            x2={d.x}
            y2={d.y - 24}
            delay={i * 1.1 + 2.5}
            color={gray}
          />
        </g>
      ))}
      <circle
        cx={server.x}
        cy={server.y}
        r="28"
        fill="none"
        stroke={gray}
        strokeWidth="1"
        opacity="0.15"
      >
        <animate
          attributeName="r"
          values="28;34;28"
          dur="3s"
          repeatCount="indefinite"
        />
        <animate
          attributeName="opacity"
          values="0.15;0.06;0.15"
          dur="3s"
          repeatCount="indefinite"
        />
      </circle>
      <circle
        cx={server.x}
        cy={server.y}
        r="24"
        fill="var(--color-background-card)"
        stroke={gray}
        strokeWidth="1.5"
        opacity="0.4"
      />
      <foreignObject x={server.x - 10} y={server.y - 10} width="20" height="20">
        <div className="text-foreground-alt flex h-full w-full items-center justify-center opacity-60">
          <LuServer className="h-4 w-4" />
        </div>
      </foreignObject>
      <text
        x={server.x}
        y={server.y - 32}
        textAnchor="middle"
        fill="var(--color-foreground-alt)"
        fontSize="10"
        opacity="0.5"
      >
        Cloud Server
      </text>
      {devices.map((d, i) => (
        <g key={`dev-${i}`}>
          <circle
            cx={d.x}
            cy={d.y}
            r="20"
            fill="var(--color-background-card)"
            stroke={gray}
            strokeWidth="1"
            opacity="0.3"
          />
          <foreignObject x={d.x - 8} y={d.y - 8} width="16" height="16">
            <div className="text-foreground-alt flex h-full w-full items-center justify-center opacity-40">
              <d.icon className="h-3.5 w-3.5" />
            </div>
          </foreignObject>
          <text
            x={d.x}
            y={d.y + 34}
            textAnchor="middle"
            fill="var(--color-foreground-alt)"
            fontSize="10"
            opacity="0.4"
          >
            {d.label}
          </text>
        </g>
      ))}
      <text
        x={240}
        y={248}
        textAnchor="middle"
        fill="var(--color-foreground-alt)"
        fontSize="11"
        fontWeight="600"
        opacity="0.5"
      >
        {label}
      </text>
    </svg>
  )
}

// SpacewaveDiagram renders the peer-to-peer mesh architecture.
function SpacewaveDiagram({ label }: { label: string }) {
  const brand = 'var(--color-brand)'
  const nodes = [
    { x: 120, y: 55, label: 'Laptop', icon: LuLaptop, delay: 0 },
    { x: 360, y: 55, label: 'Phone', icon: LuSmartphone, delay: 0.5 },
    { x: 240, y: 155, label: 'Pi', icon: LuCpu, delay: 1 },
  ]

  const connections: [number, number][] = [
    [0, 1],
    [1, 2],
    [0, 2],
  ]

  return (
    <svg
      viewBox="0 0 480 260"
      className="h-full w-full"
      role="img"
      aria-label="Spacewave mesh: devices connect directly to each other"
    >
      {connections.map(([a, b], i) => (
        <g key={`c-${i}`}>
          <AnimatedConnection
            x1={nodes[a].x}
            y1={nodes[a].y}
            x2={nodes[b].x}
            y2={nodes[b].y}
            delay={i * 0.6}
            color={brand}
          />
          <DataPacket
            x1={nodes[a].x}
            y1={nodes[a].y}
            x2={nodes[b].x}
            y2={nodes[b].y}
            delay={i * 0.8 + 1}
            color={brand}
          />
        </g>
      ))}
      {nodes.map((n, i) => (
        <NetworkNode
          key={i}
          x={n.x}
          y={n.y}
          label={n.label}
          icon={n.icon}
          delay={n.delay}
          color={brand}
        />
      ))}
      <text
        x={240}
        y={248}
        textAnchor="middle"
        fill="var(--color-brand)"
        fontSize="11"
        fontWeight="600"
      >
        {label}
      </text>
    </svg>
  )
}

// New Sections

const HOW_ICONS = [LuZap, LuLock, LuCloudOff]

// HowItWorksSection renders the architecture comparison and feature cards.
function HowItWorksSection() {
  const cards = [
    {
      title: 'Every device makes it stronger',
      body: 'Your laptop, phone, Raspberry Pi, Linux box... each one joins the swarm and adds power. The more you connect, the more you can do.',
    },
    {
      title: 'Encrypted. Direct. Private.',
      body: 'Devices find each other and connect peer-to-peer. Every connection is encrypted. No server ever sees your data.',
    },
    {
      title: 'Works without the internet',
      body: 'Every device carries a full workspace. Go offline whenever you want. Everything syncs the moment you reconnect.',
    },
  ]

  return (
    <Section id="how-it-works" withTopSeparator reveal>
      <SectionLabel>How it works</SectionLabel>
      <SectionTitle>All your devices. One system.</SectionTitle>
      <SectionSubtitle>
        Spacewave turns your devices into an encrypted network. Every device
        runs a full workspace - apps, files, data - syncing directly with the
        others.
      </SectionSubtitle>

      <div className="grid gap-8 @lg:grid-cols-2">
        <div className="border-foreground/6 bg-background-card/20 rounded-lg border p-6 backdrop-blur-sm">
          <div className="text-foreground-alt mb-4 text-center text-xs font-semibold tracking-widest uppercase opacity-50">
            Traditional Cloud
          </div>
          <div className="h-64">
            <TraditionalDiagram label="Before: Everything goes through their servers" />
          </div>
        </div>

        <div className="border-brand/20 bg-background-card/30 rounded-lg border p-6 backdrop-blur-sm">
          <div className="text-brand mb-4 text-center text-xs font-semibold tracking-widest uppercase">
            Spacewave
          </div>
          <div className="h-64">
            <SpacewaveDiagram label="Spacewave: Your devices talk directly" />
          </div>
        </div>
      </div>

      <div className="mt-8 grid gap-4 @lg:grid-cols-3">
        {cards.map((card, i) => {
          const CardIcon = HOW_ICONS[i]
          return (
            <div
              key={card.title}
              className="border-foreground/6 bg-background-card/30 group rounded-lg border p-5 backdrop-blur-sm transition-all duration-300 hover:-translate-y-0.5"
            >
              <CardIcon className="text-brand mb-3 h-5 w-5" />
              <h3 className="text-foreground mb-2 text-sm font-semibold">
                {card.title}
              </h3>
              <p className="text-foreground-alt text-sm leading-relaxed text-balance">
                {card.body}
              </p>
            </div>
          )
        })}
      </div>
    </Section>
  )
}

const CASE_ICONS = [
  LuFolderSync,
  LuTerminal,
  LuCode,
  LuBookOpen,
  LuMessageSquare,
  LuZap,
]
const CASE_HIGHLIGHTS = [
  'Files & data',
  'Devices & servers',
  'Apps & tools',
  'Knowledge & planning',
  'Social & messaging',
  'CLI & automation',
]

const USE_CASES = [
  {
    title: 'Share and collaborate',
    description:
      'Files, databases, and code \u2014 synced across your whole team in real time. Conflicts resolve on their own.',
    href: '/landing/drive',
  },
  {
    title: 'Control from anywhere',
    description:
      'Reach any device in your swarm from anywhere. Terminal, desktop, or custom interface. It just works.',
    href: '/landing/devices',
  },
  {
    title: 'Build anything',
    description:
      'Create tools and apps with the full-stack SDK. Ship them instantly to every device in your swarm.',
    href: '/landing/plugins',
  },
  {
    title: 'Think clearly',
    description:
      'Notes, tasks, and plans on your devices. Structured for how you think. Synced across everything.',
    href: '/landing/notes',
  },
  {
    title: 'Talk privately',
    description:
      'Encrypted messaging for your people. Friends, family, or team. A space that belongs to you.',
    href: '/landing/chat',
  },
  {
    title: 'Command your stack',
    description:
      'Terminal-first tools for everything. Script it, automate it, pipe it. Full control from the command line.',
    href: '/landing/cli',
  },
]

// UseCaseCard renders a single use-case card with scroll-reveal animation.
function UseCaseCard({
  icon: Icon,
  highlight,
  title,
  description,
  href,
  index,
}: {
  icon: React.ComponentType<{ className?: string }>
  highlight: string
  title: string
  description: string
  href: string
  index: number
}) {
  const { ref, visible } = useScrollReveal<HTMLAnchorElement>(0.1)
  const resolvedHref = useStaticHref(href)
  return (
    <a
      ref={ref}
      href={resolvedHref}
      className={cn(
        'border-foreground/6 bg-background-card/30 group cursor-pointer rounded-lg border p-6 no-underline backdrop-blur-sm transition-all duration-500',
        visible ?
          'translate-y-0 opacity-100 hover:-translate-y-1'
        : 'translate-y-8 opacity-0',
      )}
      style={{ transitionDelay: `${index * 80}ms` }}
    >
      <div className="mb-4 flex items-center gap-3">
        <div className="bg-brand/8 group-hover:bg-brand/15 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg transition-colors">
          <Icon className="text-brand h-5 w-5" />
        </div>
        <span className="text-brand/70 text-metadata font-semibold tracking-widest uppercase">
          {highlight}
        </span>
      </div>
      <h3 className="text-foreground mb-2 text-base font-semibold">{title}</h3>
      <p className="text-foreground-alt text-sm leading-relaxed text-balance">
        {description}
      </p>
    </a>
  )
}

// UseCasesSection renders the use-case cards grid.
function UseCasesSection() {
  const nav = useNavLinks()
  return (
    <Section id="use-cases" reveal>
      <SectionLabel>What you can do</SectionLabel>
      <SectionTitle>Start with one thing. Then do everything.</SectionTitle>
      <SectionSubtitle>
        Pick what matters to you. Combine them however you want. This is your
        system.
      </SectionSubtitle>

      <div className="grid gap-4 @lg:grid-cols-2 @2xl:grid-cols-3">
        {USE_CASES.map((c, i) => (
          <UseCaseCard
            key={c.title}
            icon={CASE_ICONS[i]}
            highlight={CASE_HIGHLIGHTS[i]}
            title={c.title}
            description={c.description}
            href={c.href}
            index={i}
          />
        ))}
      </div>

      <CtaRow>
        <CtaButton icon={LuRocket} variant="primary" onClick={nav.getStarted}>
          Get Started
        </CtaButton>
      </CtaRow>
    </Section>
  )
}

const STACK_LAYERS = [
  { name: 'Bifrost', desc: 'Encrypted P2P networking', icon: LuGlobe },
  { name: 'Hydra', desc: 'Data storage and sync', icon: LuDatabase },
  { name: 'Bldr', desc: 'Cross-platform runtime', icon: LuCpu },
  { name: 'Spacewave SDK', desc: 'Full-stack developer API', icon: LuCode },
]

// ArchitectureStackDiagram renders the Spacewave stack as peeking tabs with the top card expanded.
function ArchitectureStackDiagram() {
  const { ref, visible } = useScrollReveal(0.1)

  return (
    <div ref={ref} className="mx-auto max-w-lg px-4">
      <div className="space-y-[-1px]">
        {STACK_LAYERS.map((layer, i) => {
          const LayerIcon = layer.icon
          const isTop = i === STACK_LAYERS.length - 1
          return (
            <div
              key={layer.name}
              className={cn(
                'transition-all duration-500',
                visible ?
                  'translate-x-0 opacity-100'
                : '-translate-x-4 opacity-0',
              )}
              style={{
                transitionDelay: `${(STACK_LAYERS.length - 1 - i) * 100}ms`,
              }}
            >
              {isTop ?
                <div
                  className="border-foreground/8 bg-background-card relative rounded-lg border px-5 py-4 backdrop-blur-sm"
                  style={{
                    zIndex: STACK_LAYERS.length - i,
                    boxShadow: '0 4px 20px rgba(0,0,0,0.3)',
                  }}
                >
                  <div className="flex items-center gap-3">
                    <div className="bg-brand/8 flex h-9 w-9 shrink-0 items-center justify-center rounded-md">
                      <LayerIcon className="text-brand h-4 w-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="text-foreground text-sm font-semibold">
                        {layer.name}
                      </div>
                      <div className="text-foreground-alt text-xs">
                        {layer.desc}
                      </div>
                    </div>
                    <div
                      className={cn(
                        'border-brand/15 text-brand/60 flex items-center gap-1 rounded-full border px-2 py-0.5 text-[0.55rem] font-medium transition-all duration-500',
                        visible ? 'opacity-100' : 'opacity-0',
                      )}
                      style={{
                        transitionDelay: `${STACK_LAYERS.length * 100 + 300}ms`,
                      }}
                    >
                      <LuPlus className="h-2.5 w-2.5" />
                      plugins
                    </div>
                  </div>
                </div>
              : <div
                  className="border-foreground/8 bg-background-card/80 flex items-center gap-2 rounded-t-lg border-x border-t px-4 py-2"
                  style={{
                    zIndex: STACK_LAYERS.length - i,
                    marginLeft: `${(STACK_LAYERS.length - 1 - i) * 6}px`,
                  }}
                >
                  <div className="bg-brand/8 flex h-5 w-5 shrink-0 items-center justify-center rounded">
                    <LayerIcon className="text-brand h-3 w-3" />
                  </div>
                  <span className="text-foreground-alt text-xs font-medium">
                    {layer.name}
                  </span>
                  <span className="text-foreground-alt/50 text-[0.6rem]">
                    {layer.desc}
                  </span>
                </div>
              }
            </div>
          )
        })}
      </div>

      <div
        className={cn(
          'mt-3 flex items-center justify-end gap-1.5 transition-all duration-500',
          visible ? 'translate-y-0 opacity-100' : 'translate-y-2 opacity-0',
        )}
        style={{ transitionDelay: '600ms' }}
      >
        <div className="border-brand/20 bg-brand/5 flex items-center gap-1.5 rounded-full border px-3 py-1">
          <LuCircuitBoard className="text-brand h-3 w-3" />
          <span className="text-brand text-[0.5rem] font-semibold tracking-widest uppercase">
            ControllerBus connects all layers
          </span>
        </div>
      </div>
    </div>
  )
}

const DEV_CARDS = [
  {
    title: 'Full-stack, one API',
    body: 'Databases, networking, files, and UI \u2014 one SDK covers everything you need. Write in Go or TypeScript.',
  },
  {
    title: 'Deploy everywhere',
    body: 'Write it once. Run it in browsers, on desktops, on embedded devices. WebAssembly, native, and cross-compiled.',
  },
  {
    title: 'Open ecosystem',
    body: 'Publish your plugins or keep them private. Browse community contributions. Install with one click.',
  },
]

const DEV_CARD_ICONS = [LuCode, LuGlobe, LuUsers]

// ForDevelopersSection renders the developer-focused plugin ecosystem diagram and SDK info.
function ForDevelopersSection() {
  const nav = useNavLinks()
  return (
    <Section id="plugins" withTopSeparator reveal>
      <SectionLabel>For developers</SectionLabel>
      <SectionTitle>If you can imagine it, you can build it</SectionTitle>
      <SectionSubtitle>
        Spacewave was built with the same SDK you have access to. Every plugin,
        every feature, every interface was created with the open-source tools
        right in front of you.
      </SectionSubtitle>

      <div className="border-brand/20 bg-background-card/30 mx-auto max-w-2xl rounded-lg border p-6 backdrop-blur-sm">
        <ArchitectureStackDiagram />
      </div>

      <div className="mx-auto mt-10 max-w-3xl">
        <div className="border-foreground/6 bg-background-card/20 rounded-lg border p-6 backdrop-blur-sm">
          <div className="grid gap-6 @lg:grid-cols-3">
            {DEV_CARDS.map((card, i) => {
              const CardIcon = DEV_CARD_ICONS[i]
              return (
                <div key={card.title}>
                  <div className="text-brand mb-2 flex items-center gap-2 text-sm font-semibold">
                    <CardIcon className="h-4 w-4" />
                    {card.title}
                  </div>
                  <p className="text-foreground-alt text-sm leading-relaxed text-balance">
                    {card.body}
                  </p>
                </div>
              )
            })}
          </div>
        </div>
      </div>

      <CtaRow>
        <CtaButton icon={LuBookOpen} variant="primary" onClick={nav.docs}>
          Read the Docs
        </CtaButton>
        <a
          href={GITHUB_REPO_URL}
          className="border-foreground/15 bg-background/50 text-foreground hover:border-brand/40 hover:bg-brand/8 flex cursor-pointer items-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium no-underline transition-all duration-300 select-none hover:-translate-y-0.5"
        >
          <LuGithub className="h-4 w-4" />
          <span>Browse Source</span>
        </a>
      </CtaRow>
    </Section>
  )
}

interface FeatureItem {
  name: string
  spacewave: boolean | 'partial'
  traditional: boolean | 'partial'
  icon: React.ReactNode
}

const ComparisonChart: React.FC = () => {
  // Features organized by key benefits and capabilities
  const features: FeatureItem[] = [
    // Core features
    {
      name: 'Free and open-source',
      spacewave: true,
      traditional: false,
      icon: <LuGift className="h-4 w-4" />,
    },
    {
      name: 'No account required',
      spacewave: true,
      traditional: false,
      icon: <LuUserX className="h-4 w-4" />,
    },
    {
      name: 'Works offline without limitations',
      spacewave: true,
      traditional: 'partial',
      icon: <LuWifiOff className="h-4 w-4" />,
    },
    {
      name: 'Runs on your devices w/ p2p sync',
      spacewave: true,
      traditional: false,
      icon: <PiAppStoreLogoBold className="h-4 w-4" />,
    },
    {
      name: 'End-to-end encryption by default',
      spacewave: true,
      traditional: 'partial',
      icon: <LuLock className="h-4 w-4" />,
    },
    {
      name: 'Low-cost cloud storage and APIs',
      spacewave: true,
      traditional: 'partial',
      icon: <LuCreditCard className="h-4 w-4" />,
    },
    {
      name: 'No telemetry or tracking of any kind',
      spacewave: true,
      traditional: false,
      icon: <LuEyeOff className="h-4 w-4" />,
    },
  ]

  // Legend for partial support
  const renderLegend = () => (
    <div className="text-foreground-alt mt-4 flex items-center justify-center gap-6 text-xs">
      <div className="flex items-center gap-2">
        <LuCheck className="text-success h-5 w-5 font-bold" />
        <span>Full support</span>
      </div>
      <div className="flex items-center gap-2">
        <div className="bg-warning h-0.5 w-3 rounded-full" />
        <span>Partial support</span>
      </div>
      <div className="flex items-center gap-2">
        <LuX className="h-4 w-4 text-red-400" />
        <span>Not supported</span>
      </div>
    </div>
  )

  return (
    <div className="mx-auto max-w-4xl">
      <div className="border-foreground/10 bg-background-card-alt overflow-hidden rounded-lg border backdrop-blur-sm">
        {/* Header */}
        <div className="border-foreground/10 grid grid-cols-[60%_20%_20%] border-b">
          <div className="p-4 font-medium">Feature</div>
          <div className="p-4 text-center font-medium text-white">
            Spacewave
          </div>
          <div className="text-foreground-alt p-4 text-center font-medium">
            Cloud
          </div>
        </div>

        {/* Features */}
        <div className="divide-foreground/10 divide-y">
          {features.map((feature, index) => (
            <div
              key={index}
              className="grid grid-cols-[60%_20%_20%] items-center"
            >
              <div className="flex items-center gap-2 p-4">
                <span className="text-foreground-alt flex-shrink-0">
                  {feature.icon}
                </span>
                <span>{feature.name}</span>
              </div>
              <div className="flex justify-center p-4">
                {feature.spacewave === true ?
                  <LuCheck className="text-success h-6 w-6 font-bold" />
                : feature.spacewave === 'partial' ?
                  <div className="bg-partial h-5 w-5 rounded-full" />
                : <LuX className="h-5 w-5 text-red-400" />}
              </div>
              <div className="flex justify-center p-4">
                {feature.traditional === true ?
                  <LuCheck className="text-success h-6 w-6 font-bold" />
                : feature.traditional === 'partial' ?
                  <div className="bg-warning h-0.5 w-4 rounded-full" />
                : <LuX className="h-5 w-5 text-red-400" />}
              </div>
            </div>
          ))}
        </div>
      </div>

      {renderLegend()}

      <p className="text-foreground-alt mt-6 text-center text-sm">
        Spacewave is free software and can run directly on your devices without
        relying on the cloud.
      </p>
    </div>
  )
}

const OpenSourceSection: React.FC = () => {
  return (
    <Section
      id="open-source"
      className="relative w-full bg-gradient-to-r from-blue-500/5 via-indigo-500/5 to-cyan-500/5 py-12"
      withTopSeparator
    >
      <div className="container mx-auto flex flex-col items-center justify-between gap-4 px-4 @lg:flex-row @2xl:px-6">
        <div className="flex items-center gap-4">
          <div className="rounded-lg bg-blue-500/10 p-3">
            <LuCode className="text-brand h-6 w-6" />
          </div>
          <div>
            <h2 className="text-xl font-semibold text-white">
              Open Source Software
            </h2>
            <p className="text-sm text-gray-400">
              Built in the open, for everyone
            </p>
          </div>
        </div>
        <a
          href={GITHUB_REPO_URL}
          className="group hover:border-brand/30 hover:bg-brand/10 flex cursor-pointer items-center rounded-md border border-gray-700 bg-black/50 px-6 py-2 text-sm font-medium text-white no-underline transition-all duration-300"
        >
          <LuGithub className="mr-2 h-4 w-4 transition-transform duration-300 group-hover:scale-110" />
          <span className="select-none">View on GitHub</span>
        </a>
      </div>
    </Section>
  )
}

function Footer() {
  const dmcaHref = useStaticHref('/dmca')
  const tosHref = useStaticHref('/tos')
  const privacyHref = useStaticHref('/privacy')

  return (
    <footer className="flex w-full shrink-0 flex-col items-center gap-2 border-t border-gray-800 bg-black/90 px-4 py-6 @lg:flex-row @2xl:px-6">
      <p className="text-xs text-gray-400 select-none">
        &copy; 2018-2026{' '}
        <a
          href="https://github.com/aperturerobotics"
          className="text-gray-300 hover:text-white hover:underline"
        >
          Aperture Robotics
        </a>
        , LLC. and contributors
      </p>
      <nav className="flex gap-2 @lg:ml-auto @lg:gap-6">
        <a
          className="text-xs text-gray-400 underline-offset-4 select-none hover:text-white hover:underline"
          href={dmcaHref}
        >
          DMCA
        </a>

        <a
          className="text-xs text-gray-400 underline-offset-4 select-none hover:text-white hover:underline"
          href={tosHref}
        >
          Terms of Service
        </a>

        <a
          className="text-xs text-gray-400 underline-offset-4 select-none hover:text-white hover:underline"
          href={privacyHref}
        >
          Privacy
        </a>
      </nav>
    </footer>
  )
}

// LandingContent renders the main Spacewave landing page.
export default function LandingContent(): React.ReactElement {
  return (
    <>
      <HeroSection />
      <HowItWorksSection />
      <UseCasesSection />
      <Section id="comparison" withTopSeparator>
        <ComparisonChart />
      </Section>
      <ForDevelopersSection />
      <Section id="faq" withTopSeparator>
        <FaqAccordion />
      </Section>
      <OpenSourceSection />
      <Footer />
    </>
  )
}
