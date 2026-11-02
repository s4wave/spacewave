import * as ownerStylex from '@stylexjs/stylex'
import { cn as ownerClasses } from '@s4wave/web/style/utils.js'
import { groupMarker as ownergroupMarker } from '@s4wave/web/style/relations.stylex.js'
/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState } from 'react'
import {
  LuArrowLeft,
  LuGithub,
  LuHeart,
  LuShield,
  LuWifi,
  LuHardDrive,
  LuPuzzle,
  LuGitFork,
  LuMessageSquare,
  LuBookOpen,
  LuArrowRight,
  LuSearch,
} from '@s4wave/web/images/icons.js'
import {
  DISCORD_INVITE_URL,
  GITHUB_ISSUES_URL,
  GITHUB_REPO_URL,
} from '@s4wave/app/github.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { useInvokeCommand } from '@s4wave/web/command/index.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { cn } from '@s4wave/web/style/utils.js'
import { ExternalLink } from './ExternalLink.js'
import {
  licenseEntries,
  groupByCategory,
  type AnnotatedLicenseEntry,
} from '@s4wave/app/licenses/data.js'
import { useLandingBackNavigation } from './useLandingBackNavigation.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  motion as themeMotion,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeLayout['--spacing'],
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule4: {
    pointerEvents: 'none',
    position: 'absolute',
    top: '50%',
    left: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
})

export const metadata = {
  title: 'Community - Spacewave',
  description:
    'Join the Spacewave open-source community. Built with Go, TypeScript, React, and WebAssembly. Contribute code, docs, ideas.',
  canonicalPath: '/community',
}

const PRINCIPLES = [
  {
    icon: LuShield,
    title: 'End-to-end encrypted',
    body: 'All data is end-to-end encrypted by default. Protect your account with a password, a long PIN, or a passkey that can derive keys, and only your devices can unlock it.',
  },
  {
    icon: LuWifi,
    title: 'Local-first, always',
    body: 'Your devices talk directly. Cloud relay and backup are optional and always encrypted.',
  },
  {
    icon: LuHardDrive,
    title: 'Your data, wherever you want it',
    body: "Most apps store your data in someone else's database. Spacewave runs on your device and stores data wherever you want.",
  },
  {
    icon: LuPuzzle,
    title: 'Fully extensible',
    body: 'Add plugins, modify the source, or build something entirely new. Share your creations with others through Spacewave itself.',
  },
]

const CONTRIBUTE_PATHS = [
  {
    icon: LuGitFork,
    title: 'Code',
    description: 'Fix a bug, build a feature, improve the architecture.',
    href: GITHUB_ISSUES_URL,
    linkText: 'Browse open issues',
  },
  {
    icon: LuBookOpen,
    title: 'Documentation',
    description: 'Write guides, improve docs, translate for new audiences.',
    href: `${GITHUB_REPO_URL}/blob/master/CONTRIBUTING.md`,
    linkText: 'Help with docs',
  },
  {
    icon: LuMessageSquare,
    title: 'Community',
    description: 'Connect with other Spacewave users and contributors.',
    href: DISCORD_INVITE_URL,
    linkText: 'Join our Discord',
  },
]

const devCount = licenseEntries.filter((e) => e.isDev).length

function describeSource(entry: AnnotatedLicenseEntry): string {
  if (entry.source === 'both') return 'Go + JS'
  if (entry.source === 'go') return 'Go'
  return 'JS'
}

function formatDependencyDetails(entry: AnnotatedLicenseEntry): string {
  const lines = [
    `Name: ${entry.name}`,
    `Version: ${entry.version}`,
    `License: ${entry.spdx}`,
    `Source: ${describeSource(entry)}`,
    `Category: ${entry.category}`,
  ]
  if (entry.purpose) lines.push(`Purpose: ${entry.purpose}`)
  if (entry.repo) lines.push(`URL: ${entry.repo}`)
  return lines.join('\n')
}

export function Community() {
  const goBack = useLandingBackNavigation()
  const invokeCommand = useInvokeCommand()
  const licensesHref = useStaticHref('/licenses')
  const [showDev, setShowDev] = useState(false)
  const [search, setSearch] = useState('')
  const filtered = useMemo(() => {
    let entries = showDev
      ? licenseEntries
      : licenseEntries.filter((e) => !e.isDev)
    if (search) {
      const q = search.toLowerCase()
      entries = entries.filter(
        (e) =>
          e.name.toLowerCase().includes(q) ||
          e.purpose?.toLowerCase().includes(q),
      )
    }
    return entries
  }, [showDev, search])
  const ossGroups = useMemo(() => groupByCategory(filtered), [filtered])
  const ossStats = useMemo(() => {
    const goCount = filtered.filter(
      (e) => e.source === 'go' || e.source === 'both',
    ).length
    const jsCount = filtered.filter(
      (e) => e.source === 'js' || e.source === 'both',
    ).length
    const spdxSet = new Set(filtered.map((e) => e.spdx))
    return {
      total: filtered.length,
      goCount,
      jsCount,
      uniqueLicenses: spdxSet.size,
    }
  }, [filtered])
  const handleCopyDependency = useCallback((entry: AnnotatedLicenseEntry) => {
    navigator.clipboard.writeText(formatDependencyDetails(entry)).then(
      () => {
        toast.success('Copied dependency details', {
          description: entry.name,
          duration: 2000,
        })
      },
      () => {
        toast.error('Copy failed', {
          description: `Could not copy ${entry.name}.`,
        })
      },
    )
  }, [])
  const handleEmailSupport = useCallback(() => {
    invokeCommand('spacewave.help.email-support')
  }, [invokeCommand])

  return (
    <div className="bg-background-landing @container flex w-full flex-1 flex-col overflow-y-auto">
      {/* Back button */}
      <div className="px-4 pt-6 @lg:px-8">
        <button
          type="button"
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground flex cursor-pointer items-center gap-2 text-sm transition-colors"
        >
          <LuArrowLeft sx={stylexStyles.rule1} />
          Back
        </button>
      </div>

      {/* Hero */}
      <header className="mx-auto w-full max-w-3xl px-4 pt-14 pb-14 text-center @lg:px-8 @lg:pt-20 @lg:pb-16">
        <div className="text-brand mb-6 flex items-center justify-center gap-2">
          <LuHeart sx={stylexStyles.rule2} />
        </div>
        <h1 className="text-foreground mb-6 text-4xl font-semibold tracking-tight @lg:text-5xl">
          Built in the open,
          <br />
          <span className="text-brand">for&nbsp;everyone</span>
        </h1>
        <p className="text-foreground-alt mx-auto max-w-xl text-base leading-relaxed @lg:text-lg">
          Spacewave is free and open-source software built by the community.
        </p>
        <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
          <ExternalLink
            href={GITHUB_REPO_URL}
            className="border-foreground/20 hover:border-brand/40 hover:bg-brand/5 text-foreground inline-flex items-center gap-2.5 rounded-lg border px-6 py-3 text-sm font-medium transition"
          >
            <LuGithub sx={stylexStyles.rule2} />
            View on GitHub
          </ExternalLink>
          <button
            type="button"
            onClick={handleEmailSupport}
            className="border-foreground/20 hover:border-brand/40 hover:bg-brand/5 text-foreground inline-flex cursor-pointer items-center gap-2.5 rounded-lg border px-6 py-3 text-sm font-medium transition"
          >
            <LuMessageSquare sx={stylexStyles.rule2} />
            Email Support
          </button>
        </div>
      </header>

      {/* Contribute */}
      <section className="mx-auto w-full max-w-4xl px-4 pb-14 @lg:px-8 @lg:pb-16">
        <span className="text-foreground-alt tracking-brand-wide mb-8 block text-center text-xs font-semibold uppercase">
          Get involved
        </span>
        <div className="grid gap-4 @lg:grid-cols-3">
          {CONTRIBUTE_PATHS.map((c) => (
            <ExternalLink
              key={c.title}
              href={c.href}
              className={ownerClasses(
                'border-foreground/8 hover:border-foreground/15 bg-background-card/50 group flex flex-col gap-3 rounded-lg border p-5 transition hover:-translate-y-0.5',
                ownerStylex.props(ownergroupMarker).className,
              )}
            >
              <c.icon sx={stylexStyles2.rule1} />
              <h3 className="text-foreground text-sm font-semibold">
                {c.title}
              </h3>
              <p className="text-foreground-alt text-sm leading-relaxed">
                {c.description}
              </p>
              <span className="text-brand mt-auto flex items-center gap-1 text-sm font-medium">
                {c.linkText}
                <LuArrowRight sx={stylexStyles.rule3} />
              </span>
            </ExternalLink>
          ))}
        </div>
      </section>

      {/* Divider */}
      <div className="bg-foreground/7 mx-auto h-px w-full max-w-4xl" />

      {/* Principles */}
      <section className="mx-auto w-full max-w-4xl px-4 py-14 @lg:px-8 @lg:py-16">
        <span className="text-foreground-alt tracking-brand-wide mb-8 block text-center text-xs font-semibold uppercase">
          What we believe
        </span>
        <div className="grid gap-x-8 gap-y-10 @lg:grid-cols-2">
          {PRINCIPLES.map((p) => (
            <div key={p.title} sx={[stylexStyles3.rule1, groupMarker3]}>
              <div className="bg-brand/8 group-hover:bg-brand/12 flex size-10 shrink-0 items-center justify-center rounded-lg transition-colors">
                <p.icon sx={stylexStyles2.rule2} />
              </div>
              <div>
                <h3 className="text-foreground mb-1 text-sm font-semibold">
                  {p.title}
                </h3>
                <p className="text-foreground-alt text-sm leading-relaxed">
                  {p.body}
                </p>
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* Divider */}
      <div className="bg-foreground/7 mx-auto h-px w-full max-w-4xl" />

      {/* Built With Open Source */}
      <section className="mx-auto w-full max-w-4xl px-4 py-14 @lg:px-8 @lg:py-16">
        <span className="text-foreground-alt tracking-brand-wide mb-2 block text-center text-xs font-semibold uppercase">
          Built with open source
        </span>
        <p className="text-foreground-alt/50 mb-4 text-center text-xs">
          {ossStats.total} packages &middot; {ossStats.goCount} Go &middot;{' '}
          {ossStats.jsCount} JS &middot; {ossStats.uniqueLicenses} license types
        </p>
        {devCount > 0 && (
          <div className="mb-10 flex justify-center">
            <button
              type="button"
              onClick={() => setShowDev((v) => !v)}
              className={cn(
                'flex cursor-pointer items-center gap-2 rounded-full border px-3 py-1.5 text-xs transition-all duration-150',
                showDev
                  ? 'border-brand/30 bg-brand/10 text-brand'
                  : 'border-foreground/10 text-foreground-alt/50 hover:border-foreground/20',
              )}
            >
              <span
                className={cn(
                  'size-3 rounded-sm border transition-colors',
                  showDev ? 'border-brand bg-brand' : 'border-foreground/20',
                )}
              />
              Show {devCount} dev{' '}
              {devCount === 1 ? 'dependency' : 'dependencies'}
            </button>
          </div>
        )}
        {!devCount && <div className="mb-6" />}
        <div className="relative mx-auto mb-8 max-w-md">
          <LuSearch sx={stylexStyles.rule4} />
          <input
            aria-label="Filter packages"
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Filter packages…"
            className="border-foreground/10 bg-background-card/30 text-foreground placeholder:text-foreground-alt/30 focus:border-brand/50 w-full rounded-lg border py-2 pr-3 pl-9 text-xs transition-colors outline-none"
          />
        </div>
        {ossGroups.length === 0 ? (
          <div className="text-foreground-alt/40 py-8 text-center text-xs">
            No packages match &ldquo;{search}&rdquo;
          </div>
        ) : (
          <div className="space-y-8">
            {ossGroups.map(({ category, entries }) => (
              <div key={category.id}>
                <div className="mb-3 flex items-center gap-2">
                  <h3
                    className={cn(
                      'text-xs font-medium select-none',
                      category.id === 'internal'
                        ? 'text-brand'
                        : 'text-foreground',
                    )}
                  >
                    {category.id === 'internal' ? 'Our Stack' : category.label}
                  </h3>
                  <span className="text-foreground-alt/40 text-xs">
                    {entries.length}
                  </span>
                </div>
                <div className="grid gap-3 @xl:grid-cols-2">
                  {entries.map((entry) => (
                    <div
                      key={`${entry.name}:${entry.version}:${entry.spdx}:${entry.source}:${entry.repo ?? 'local'}`}
                      onClick={() => handleCopyDependency(entry)}
                      onKeyDown={(e) => {
                        if (e.key !== 'Enter' && e.key !== ' ') return
                        e.preventDefault()
                        handleCopyDependency(entry)
                      }}
                      role="button"
                      tabIndex={0}
                      aria-label={`Copy dependency details for ${entry.name}`}
                      className={cn(
                        'flex cursor-pointer flex-col gap-1.5 rounded-lg border p-3 transition-all duration-150',
                        entry.internal
                          ? 'border-brand/15 bg-brand/5 hover:border-brand/30'
                          : 'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50',
                      )}
                    >
                      <div className="flex items-center gap-2">
                        <div className="flex min-w-0 items-center gap-2">
                          {entry.repo ? (
                            <ExternalLink
                              href={entry.repo}
                              onClick={(e) => e.stopPropagation()}
                              className={cn(
                                'truncate text-xs font-medium hover:underline',
                                entry.internal
                                  ? 'text-brand'
                                  : 'text-foreground',
                              )}
                            >
                              {entry.name}
                            </ExternalLink>
                          ) : (
                            <span
                              className={cn(
                                'truncate text-xs font-medium',
                                entry.internal
                                  ? 'text-brand'
                                  : 'text-foreground',
                              )}
                            >
                              {entry.name}
                            </span>
                          )}
                          <span
                            title={entry.version}
                            className="text-foreground-alt/40 micro-fine max-w-28 truncate"
                          >
                            {entry.version}
                          </span>
                        </div>
                        <div className="ml-auto flex shrink-0 gap-1">
                          <span className="border-foreground/8 text-foreground-alt/50 micro-fine rounded-full border px-2 py-0.5 font-medium">
                            {entry.spdx}
                          </span>
                          {(entry.source === 'go' ||
                            entry.source === 'both') && (
                            <span className="micro-fine rounded-full border border-cyan-500/15 px-2 py-0.5 font-medium text-cyan-400/60">
                              Go
                            </span>
                          )}
                          {(entry.source === 'js' ||
                            entry.source === 'both') && (
                            <span className="micro-fine rounded-full border border-yellow-500/15 px-2 py-0.5 font-medium text-yellow-400/60">
                              JS
                            </span>
                          )}
                        </div>
                      </div>
                      {entry.purpose && (
                        <p className="text-foreground-alt/50 micro-text leading-relaxed">
                          {entry.purpose}
                        </p>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
        <p className="text-foreground-alt/40 mt-8 text-center text-xs">
          Full license texts available at{' '}
          <a
            href={licensesHref}
            className="text-brand/60 hover:text-brand underline transition-colors"
          >
            /licenses
          </a>
        </p>
      </section>

      {/* Divider */}
      <div className="bg-foreground/7 mx-auto h-px w-full max-w-4xl" />

      {/* Open source acknowledgment */}
      <section className="mx-auto w-full max-w-3xl px-4 py-10 text-center @lg:px-8 @lg:py-12">
        <p className="text-foreground-alt mb-4 text-sm leading-relaxed">
          Spacewave is built with Go, TypeScript, React, WebAssembly, and
          WebRTC, running in web browsers, Linux, and other operating systems.
          We are grateful to everyone behind these projects.
        </p>
        <ExternalLink
          href={GITHUB_REPO_URL}
          className="text-brand hover:text-brand-highlight mt-4 inline-flex items-center gap-2 text-sm underline"
        >
          <LuGithub sx={stylexStyles.rule1} />
          Contribute to Spacewave
        </ExternalLink>
      </section>

      {/* Footer */}
      <footer className="pb-8 text-center">
        <p className="text-foreground-alt/50 text-xs">
          © 2018–2026 Aperture Robotics, LLC. and contributors
        </p>
      </footer>
    </div>
  )
}
