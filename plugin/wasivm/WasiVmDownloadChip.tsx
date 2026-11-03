import { LuCloudDownload } from '@s4wave/web/images/icons.js'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { formatBytes } from '@s4wave/app/system/format.js'
import type { WasiVmHandle } from '@s4wave/sdk/vm/wasivm.js'
import { WasiVmDownloads } from '@s4wave/sdk/vm/wasivm.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    position: 'absolute',
    insetBlock: 0,
    left: 0,
    width: {
      default: '33.3333%',
      '@media (prefers-reduced-motion: reduce)': '100%',
    },
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
    animationName: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationDuration: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationTimingFunction: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'ease',
    },
    animationIterationCount: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 1,
    },
    animationDirection: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'normal',
    },
    animationPlayState: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'running',
    },
    animationDelay: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationFillMode: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationTimeline: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'auto',
    },
    opacity: { default: null, '@media (prefers-reduced-motion: reduce)': 0.5 },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    opacity: { default: 0, ':is([data-active="true"])': 1 },
    transitionProperty: 'opacity',
    transitionTimingFunction: 'var(--ease-out)',
    transitionDuration: '.2s',
    transitionDelay: { default: '.5s', ':is([data-active="true"])': '0s' },
    '--tw-duration': '.2s',
    '--tw-ease': 'var(--ease-out)',
  },
  rule2: {
    display: { default: 'none', '@container (min-width: 28rem)': 'inline' },
  },
  rule3: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
  rule4: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
  rule5: {
    pointerEvents: 'none',
    position: 'absolute',
    insetInline: 0,
    bottom: 0,
    height: `calc(${themeLayout['--spacing']} * .5)`,
    overflow: 'hidden',
    opacity: { default: 0, ':is([data-active="true"])': 1 },
    transitionProperty: 'opacity',
    transitionTimingFunction: 'var(--ease-out)',
    transitionDuration: '.2s',
    transitionDelay: { default: '.5s', ':is([data-active="true"])': '0s' },
    '--tw-duration': '.2s',
    '--tw-ease': 'var(--ease-out)',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    animationName: {
      default: 'pulse',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationDuration: {
      default: '2s',
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationTimingFunction: {
      default: 'cubic-bezier(.4, 0, .6, 1)',
      '@media (prefers-reduced-motion: reduce)': 'ease',
    },
    animationIterationCount: {
      default: 'infinite',
      '@media (prefers-reduced-motion: reduce)': 1,
    },
    animationDirection: {
      default: 'normal',
      '@media (prefers-reduced-motion: reduce)': 'normal',
    },
    animationPlayState: {
      default: 'running',
      '@media (prefers-reduced-motion: reduce)': 'running',
    },
    animationDelay: {
      default: '0s',
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationFillMode: {
      default: 'none',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationTimeline: {
      default: 'auto',
      '@media (prefers-reduced-motion: reduce)': 'auto',
    },
    color: themeColors['--color-brand'],
  },
})

// useWasiVmDownloads watches what the VM's run has downloaded for its root
// filesystem, starting again for each run. It is undefined until the run's
// first report and after the run ends.
export function useWasiVmDownloads(
  handleResource: Resource<WasiVmHandle>,
  runGeneration: bigint,
): WasiVmDownloads | undefined {
  return (
    useStreamingResource(
      handleResource,
      (handle: WasiVmHandle, signal: AbortSignal) =>
        handle.watchDownloads(signal),
      [runGeneration],
      { equals: WasiVmDownloads.equals },
    ).value ?? undefined
  )
}

// WasiVmDownloadChip shows a run downloading its root filesystem image from
// the CDN: a chip with the bytes so far and a sweeping line along the
// header's bottom edge. Both fade in at once and fade out half a second after
// the last read, so a burst of reads shows as one download. A root filesystem
// in the Space downloads nothing and shows nothing.
export function WasiVmDownloadChip({
  downloads,
}: {
  downloads: WasiVmDownloads | undefined
}) {
  if (!downloads?.remote || !downloads.bytes) return null
  const active = (downloads.inFlight ?? 0) > 0
  const bytes = formatBytes(downloads.bytes)
  return (
    <>
      <span
        data-active={active}
        aria-hidden={!active}
        title={`Downloaded ${bytes} of the VM image`}
        sx={stylexStyles2.rule1}
      >
        <LuCloudDownload sx={stylexStyles.rule1} />
        <span sx={stylexStyles2.rule2}>Downloading</span>
        <span sx={stylexStyles2.rule3}>{bytes}</span>
      </span>
      <span role="status" sx={stylexStyles2.rule4}>
        {active ? 'Downloading the VM image' : ''}
      </span>
      <div aria-hidden data-active={active} sx={stylexStyles2.rule5}>
        <div sx={[stylexStyles3.rule1, downloadProgressStyles.motion]} />
      </div>
    </>
  )
}

// downloadProgressStyles reuses the shared plain-CSS progress keyframes.
const downloadProgressStyles = stylex.create({
  motion: {
    animationName: {
      default: 'progress-indeterminate',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationDuration: '1.4s',
    animationTimingFunction: 'ease-in-out',
    animationIterationCount: 'infinite',
  },
})
