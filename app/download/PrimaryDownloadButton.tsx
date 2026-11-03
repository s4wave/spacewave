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

const stylexStyles3 = stylex.create({
  rule1: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: {
      default: 'var(--text-base)',
      '@container (min-width: 32rem)': 'var(--text-lg)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-lg--line-height))',
    },
    textAlign: 'center',
    userSelect: 'none',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    WebkitUserSelect: 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'inline-flex',
    fontSize: {
      default: 'var(--text-base)',
      '@container (min-width: 32rem)': 'var(--text-lg)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-lg--line-height))',
    },
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    userSelect: 'none',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
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
    return <p sx={stylexStyles3.rule1}>Pick a build below.</p>
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
