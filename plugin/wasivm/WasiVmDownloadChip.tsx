import {keyframeNames} from "../../web/style/tokens.stylex.js"
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
    borderRadius: '3.40282e38px',
    insetBlock: 0,
    animationDelay: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationDirection: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'normal',
    },
    animationDuration: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationFillMode: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationIterationCount: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 1,
    },
    animationName: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationPlayState: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'running',
    },
    animationTimeline: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'auto',
    },
    animationTimingFunction: {
      default: null,
      '@media (prefers-reduced-motion: reduce)': 'ease',
    },
    backgroundColor: themeColors['--color-brand'],
    opacity: { default: null, '@media (prefers-reduced-motion: reduce)': 0.5 },
    position: 'absolute',
    left: 0,
    width: {
      default: '33.3333%',
      '@media (prefers-reduced-motion: reduce)': '100%',
    },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.2s',
    '--tw-ease': 'var(--ease-out)',
    '--tw-numeric-spacing': 'tabular-nums',
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: 0, ':is([data-active="true"])': 1 },
    transitionDelay: { default: '.5s', ':is([data-active="true"])': '0s' },
    transitionDuration: '.2s',
    transitionProperty: 'opacity',
    transitionTimingFunction: 'var(--ease-out)',
    minWidth: 0,
  },
  rule2: {
    display: { default: 'none', '@container (min-width: 28rem)': 'inline' },
  },
  rule3: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', },
  rule4: {
    margin: '-1px',
    padding: 0,
    borderWidth: 0,
    overflow: 'hidden',
    clipPath: 'inset(50%)',
    position: 'absolute',
    whiteSpace: 'nowrap',
    height: '1px',
    width: '1px',
  },
  rule5: {
    '--tw-duration': '.2s',
    '--tw-ease': 'var(--ease-out)',
    insetInline: 0,
    overflow: 'hidden',
    opacity: { default: 0, ':is([data-active="true"])': 1 },
    pointerEvents: 'none',
    position: 'absolute',
    transitionDelay: { default: '.5s', ':is([data-active="true"])': '0s' },
    transitionDuration: '.2s',
    transitionProperty: 'opacity',
    transitionTimingFunction: 'var(--ease-out)',
    bottom: 0,
    height: `calc(${themeLayout['--spacing']} * .5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    animationDelay: {
      default: '0s',
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationDirection: {
      default: 'normal',
      '@media (prefers-reduced-motion: reduce)': 'normal',
    },
    animationDuration: {
      default: '2s',
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationFillMode: {
      default: 'none',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationIterationCount: {
      default: 'infinite',
      '@media (prefers-reduced-motion: reduce)': 1,
    },
    animationName: {
      default: keyframeNames["pulse"],
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationPlayState: {
      default: 'running',
      '@media (prefers-reduced-motion: reduce)': 'running',
    },
    animationTimeline: {
      default: 'auto',
      '@media (prefers-reduced-motion: reduce)': 'auto',
    },
    animationTimingFunction: {
      default: 'cubic-bezier(.4, 0, .6, 1)',
      '@media (prefers-reduced-motion: reduce)': 'ease',
    },
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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
    animationDuration: '1.4s',
    animationIterationCount: 'infinite',
    animationName: {
      default: keyframeNames["progress-indeterminate"],
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationTimingFunction: 'ease-in-out',
  },
})
