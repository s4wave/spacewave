import { useState, useMemo, useCallback } from 'react'
import {
  LuScale,
  LuSearch,
  LuChevronDown,
  LuChevronRight,
} from '@s4wave/web/images/icons.js'
import {
  licenseEntries,
  licenseBases,
  licenseStats,
  reconstructText,
  type AnnotatedLicenseEntry,
} from '@s4wave/app/licenses/data.js'
import { ExternalLink } from './ExternalLink.js'
import { LegalPageLayout } from './LegalPageLayout.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { motion as themeMotion } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: { marginBottom: `calc(${themeLayout['--spacing']} * 3)` },
})

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginLeft: 'auto',
  },
  rule8: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule9: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    whiteSpace: 'pre-wrap',
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    borderBottomStyle: {
      default: themeUtilityState['--tw-border-style'],
      ':last-child': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: { default: '1px', ':last-child': 0 },
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule13: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    cursor: 'pointer',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule14: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule15: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule16: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule17: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: themeLayout['--spacing'],
  },
  rule18: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeLayout['--text-metadata'],
    lineHeight: 'var(--leading-relaxed)',
    whiteSpace: 'pre-wrap',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    maxHeight: `calc(${themeLayout['--spacing']} * 64)`,
    overflowY: 'auto',
  },
  rule19: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    marginTop: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule20: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule21: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule22: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule23: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    marginTop: themeLayout['--spacing'],
  },
  rule24: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule25: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-4xl)',
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    width: '100%',
  },
  rule26: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: { default: null, '@container (min-width: 28rem)': 'center' },
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@container (min-width: 28rem)': 'row',
    },
    justifyContent: {
      default: null,
      '@container (min-width: 28rem)': 'space-between',
    },
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule27: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    display: 'flex',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule28: { position: 'relative' },
  rule29: {
    '--tw-outline-style': { default: null, ':focus': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus': `color-mix(in oklab, ${themeColors['--color-white']} 20%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    boxShadow: {
      default: null,
      ':focus':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: { default: null, ':focus': 'none' },
    paddingLeft: `calc(${themeLayout['--spacing']} * 8)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    width: {
      default: '100%',
      '@container (min-width: 28rem)': `calc(${themeLayout['--spacing']} * 64)`,
    },
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
  },
  rule30: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 12)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule4: {
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    pointerEvents: 'none',
    position: 'absolute',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    left: `calc(${themeLayout['--spacing']} * 3)`,
    top: '50%',
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export const metadata = {
  title: 'Open Source Licenses - Spacewave',
  description:
    'Review the third-party open source software licenses, package notices, SPDX identifiers, and dependency attributions used by Spacewave.',
  canonicalPath: '/licenses',
}

function getLicenseEntryKey(entry: AnnotatedLicenseEntry): string {
  return `${entry.name}:${entry.version}:${entry.spdx}:${entry.source}:${entry.repo ?? 'local'}`
}

function SourceBadge({ source }: { source: string }) {
  const label = source === 'both' ? 'JS + Go' : source === 'js' ? 'JS' : 'Go'
  return <span sx={stylexStyles3.rule1}>{label}</span>
}

function SpdxBadge({ spdx }: { spdx: string }) {
  return <span sx={stylexStyles3.rule2}>{spdx}</span>
}

function LicenseGroup({
  sx,
  spdx,
  entries,
  baseText,
}: {
  spdx: string
  entries: AnnotatedLicenseEntry[]
  baseText?: string
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const [expanded, setExpanded] = useState<string | null>(null)
  const [showBase, setShowBase] = useState(false)

  const toggleBase = useCallback(() => setShowBase((v) => !v), [])
  const toggleEntry = useCallback(
    (key: string) => setExpanded((v) => (v === key ? null : key)),
    [],
  )

  return (
    <div sx={[stylexStyles3.rule3, sx]}>
      <div sx={stylexStyles3.rule4}>
        <SpdxBadge spdx={spdx} />
        <span sx={stylexStyles3.rule5}>{spdx}</span>
        <span sx={stylexStyles3.rule6}>
          {entries.length} {entries.length === 1 ? 'package' : 'packages'}
        </span>
        {baseText && (
          <button type="button" onClick={toggleBase} sx={stylexStyles3.rule7}>
            {showBase ? 'Hide' : 'Show'} license text
            {showBase ? (
              <LuChevronDown sx={stylexStyles.rule1} />
            ) : (
              <LuChevronRight sx={stylexStyles.rule1} />
            )}
          </button>
        )}
      </div>

      {showBase && baseText && (
        <div sx={stylexStyles3.rule8}>
          <pre sx={stylexStyles3.rule9}>{baseText}</pre>
        </div>
      )}

      <div sx={stylexStyles3.rule10}>
        {entries.map((entry) => {
          const entryKey = getLicenseEntryKey(entry)
          const isExpanded = expanded === entryKey
          const hasCustomText = !!entry.fullText
          const hasCopyright = !!entry.copyrightNotice
          const buttonLabel = `${isExpanded ? 'Hide' : 'Show'} details for ${entry.name} ${entry.version}`

          return (
            <div key={entryKey} sx={stylexStyles3.rule11}>
              <div sx={stylexStyles3.rule12}>
                {(hasCustomText || hasCopyright) && (
                  <button
                    type="button"
                    onClick={() => toggleEntry(entryKey)}
                    aria-expanded={isExpanded}
                    aria-label={buttonLabel}
                    sx={stylexStyles3.rule13}
                  >
                    {isExpanded ? (
                      <LuChevronDown sx={stylexStyles.rule2} />
                    ) : (
                      <LuChevronRight sx={stylexStyles.rule2} />
                    )}
                  </button>
                )}
                <span sx={stylexStyles3.rule14}>
                  {entry.repo ? (
                    <ExternalLink href={entry.repo} sx={stylexStyles2.rule1}>
                      {entry.name}
                    </ExternalLink>
                  ) : (
                    entry.name
                  )}
                </span>
                <span sx={stylexStyles3.rule15}>{entry.version}</span>
                <SourceBadge source={entry.source} />
              </div>

              {isExpanded && (
                <div sx={stylexStyles3.rule16}>
                  {hasCopyright && (
                    <p sx={stylexStyles3.rule17}>{entry.copyrightNotice}</p>
                  )}
                  {hasCustomText && (
                    <pre sx={stylexStyles3.rule18}>
                      {reconstructText(entry)}
                    </pre>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// vmComponents are the separately licensed programs in the virtual machine
// images Spacewave publishes. They are not linked into Spacewave, so they are
// listed here instead of in the dependency inventory.
const vmComponents = [
  {
    name: 'v86',
    spdx: 'BSD-2-Clause',
    href: 'https://github.com/aperturerobotics/v86',
    note: 'The x86 emulator, built to WebAssembly.',
  },
  {
    name: 'SeaBIOS and SeaVGABIOS',
    spdx: 'LGPL-3.0-only',
    href: 'https://github.com/aperturerobotics/v86/tree/master/bios',
    note: 'The BIOS images, with their build configuration and fetch script.',
  },
  {
    name: 'Linux',
    spdx: 'GPL-2.0-only',
    href: 'https://github.com/aperturerobotics/linux/tree/v86',
    note: 'The kernel image, built from the v86 branch.',
  },
  {
    name: 'Debian root filesystem',
    spdx: 'Various, mostly GPL',
    href: 'https://github.com/aperturerobotics/v86-debian',
    note: 'Debian packages under their own licenses, assembled by these build scripts. Each package lists its license in /usr/share/doc inside the image.',
  },
] as const

// VmImageNotice lists the virtual machine image components and the offer of
// their source code.
function VmImageNotice() {
  return (
    <div sx={stylexStyles3.rule19}>
      <h2 sx={stylexStyles3.rule20}>Virtual machine images</h2>
      <p sx={stylexStyles3.rule21}>
        Spacewave publishes images for its in-browser virtual machine. They
        contain these programs, each under its own license.
      </p>
      <ul sx={stylexStyles4.rule1}>
        {vmComponents.map((c) => (
          <li key={c.name} sx={[nativeChildSpacing.s2, stylexStyles3.rule22]}>
            <ExternalLink href={c.href} sx={stylexStyles2.rule2}>
              {c.name}
            </ExternalLink>{' '}
            <SpdxBadge spdx={c.spdx} />
            <p sx={stylexStyles3.rule23}>{c.note}</p>
          </li>
        ))}
      </ul>
      <p sx={stylexStyles3.rule24}>
        For at least three years after we last publish an image, we will give
        anyone the complete corresponding source code of its GPL and LGPL
        programs, including the Debian source packages, at no more than the cost
        of shipping it. Write to legal@aperture.us.
      </p>
    </div>
  )
}

// Licenses renders the open source licenses page.
export function Licenses() {
  const [filter, setFilter] = useState('')
  const stats = licenseStats()

  const filtered = useMemo(() => {
    if (!filter) return licenseEntries
    const q = filter.toLowerCase()
    return licenseEntries.filter(
      (e) =>
        e.name.toLowerCase().includes(q) || e.spdx.toLowerCase().includes(q),
    )
  }, [filter])

  const groups = useMemo(() => {
    const map = new Map<string, AnnotatedLicenseEntry[]>()
    for (const entry of filtered) {
      const spdx = entry.spdx
      if (!map.has(spdx)) map.set(spdx, [])
      map.get(spdx)!.push(entry)
    }
    return Array.from(map.entries())
      .toSorted((a, b) => b[1].length - a[1].length)
      .map(([spdx, entries]) => ({
        spdx,
        entries: entries.sort((a, b) => {
          const nameDiff = a.name.localeCompare(b.name)
          if (nameDiff !== 0) return nameDiff
          return a.version.localeCompare(b.version)
        }),
      }))
  }, [filtered])

  const onFilterChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => setFilter(e.target.value),
    [],
  )

  return (
    <LegalPageLayout
      icon={<LuScale sx={stylexStyles.rule3} />}
      title="Open Source Licenses"
      subtitle="Spacewave is built with open source software. This page lists all third-party dependencies and their licenses."
    >
      <section sx={stylexStyles3.rule25}>
        <div sx={stylexStyles3.rule26}>
          <div sx={stylexStyles3.rule27}>
            <span>{stats.total} packages</span>
            <span>{stats.jsCount} JavaScript</span>
            <span>{stats.goCount} Go</span>
            <span>{stats.uniqueLicenses} license types</span>
          </div>

          <div sx={stylexStyles3.rule28}>
            <LuSearch sx={stylexStyles.rule4} />
            <input
              aria-label="Filter packages"
              type="text"
              placeholder="Filter packages…"
              value={filter}
              onChange={onFilterChange}
              sx={stylexStyles3.rule29}
            />
          </div>
        </div>

        <div sx={false}>
          {groups.map(({ spdx, entries }) => (
            <LicenseGroup
              sx={nativeChildSpacingNext.s6}
              key={spdx}
              spdx={spdx}
              entries={entries}
              baseText={licenseBases[spdx]}
            />
          ))}
        </div>

        {filtered.length === 0 && (
          <p sx={stylexStyles3.rule30}>
            No packages match &ldquo;{filter}&rdquo;
          </p>
        )}

        <VmImageNotice />
      </section>
    </LegalPageLayout>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s6: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * var(--tw-space-y-reverse))`,
    },
  },
})
