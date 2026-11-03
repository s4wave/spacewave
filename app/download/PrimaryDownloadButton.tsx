import { LuDownload } from '@s4wave/web/images/icons.js'

import { ExternalLink } from '@s4wave/app/landing/ExternalLink.js'

import type { DownloadEntry } from './manifest.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'inline-flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: {
      default: 'var(--text-base)',
      '@container (min-width: 32rem)': 'var(--text-lg)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-lg--line-height))',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

interface PrimaryDownloadButtonProps {
  entry: DownloadEntry | null
  releaseLabel?: string
}

// PrimaryDownloadButton renders the hero download CTA for the detected
// platform. Falls back to a "Pick a build below." headline when detection
// missed or the detected tuple is absent from the manifest.
export function PrimaryDownloadButton({
  entry,
  releaseLabel,
}: PrimaryDownloadButtonProps) {
  if (!entry) {
    return (
      <p className="text-foreground-alt text-center text-base select-none @lg:text-lg">
        Pick a build below.
      </p>
    )
  }

  return (
    <ExternalLink href={entry.url} download sx={[stylexStyles2.rule1]}>
      <LuDownload sx={stylexStyles.rule1} />
      <span>
        Download{releaseLabel ? ` ${releaseLabel}` : ''} for {entry.osLabel} (
        {entry.archLabel}) · {entry.ext}
      </span>
    </ExternalLink>
  )
}
