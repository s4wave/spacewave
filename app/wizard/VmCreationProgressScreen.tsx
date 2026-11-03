import {
  LuCircleAlert,
  LuMonitor,
  LuRotateCw,
} from '@s4wave/web/images/icons.js'

import { PhaseChecklist } from '@s4wave/app/session/setup/PhaseChecklist.js'
import { ProgressBar } from '@s4wave/web/ui/loading/ProgressBar.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import type { VmCreationProgress, VmCreationStage } from '../vm/v86-creation.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  radii as themeRadii,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background'],
    display: 'flex',
    justifyContent: 'center',
    minHeight: `calc(${themeLayout['--spacing']} * 96)`,
    width: '100%',
  },
  rule2: {
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 2%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule4: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule5: { minWidth: 0 },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule7: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule8: { marginTop: `calc(${themeLayout['--spacing']} * 5)` },
  rule9: {
    paddingInline: 0,
    marginTop: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    display: 'inline-flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: {
      default:
        'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    marginTop: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-destructive'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface VmCreationProgressScreenProps {
  progress: VmCreationProgress
  vmName: string
  includesCdnLink: boolean
  error?: string
  onRetry?: () => void
}

const cdnStages: ReadonlyArray<{
  stage: VmCreationStage
  label: string
}> = [
  { stage: 'linking', label: 'Linking catalog image' },
  { stage: 'creating', label: 'Creating VM' },
  { stage: 'ready', label: 'Ready' },
]

const localStages = cdnStages.slice(1)

function getProgressDetail(
  progress: VmCreationProgress,
  vmName: string,
): string {
  switch (progress.stage) {
    case 'linking':
      return 'Adding the catalog image to this Space.'
    case 'creating':
      return 'Image ready. Writing the VM object.'
    case 'ready':
      return `${vmName || 'VM'} is ready. Opening it now.`
  }
}

// VmCreationProgressScreen renders the catalog image link and VM creation
// stages.
export function VmCreationProgressScreen({
  progress,
  vmName,
  includesCdnLink,
  error,
  onRetry,
}: VmCreationProgressScreenProps) {
  const stages = includesCdnLink ? cdnStages : localStages
  const activeIndex = Math.max(
    0,
    stages.findIndex((item) => item.stage === progress.stage),
  )
  const phases = stages.map((item, index) => ({
    label: item.label,
    done: index < activeIndex || progress.stage === 'ready',
    active: !error && index === activeIndex && progress.stage !== 'ready',
  }))
  const title = error
    ? 'VM could not be created'
    : progress.stage === 'ready'
      ? 'VM ready'
      : 'Creating virtual machine'

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <div sx={stylexStyles2.rule4}>
            {error ? (
              <LuCircleAlert sx={stylexStyles.rule1} aria-hidden="true" />
            ) : progress.stage === 'ready' ? (
              <LuMonitor sx={stylexStyles.rule2} aria-hidden="true" />
            ) : (
              <Spinner variant="brand" />
            )}
          </div>
          <div sx={stylexStyles2.rule5}>
            <h2 sx={stylexStyles2.rule6}>{title}</h2>
            <p sx={stylexStyles2.rule7} aria-live="polite">
              {error ?? getProgressDetail(progress, vmName)}
            </p>
          </div>
        </div>

        <div sx={stylexStyles2.rule8}>
          <ProgressBar
            value={progress.stage === 'ready' && !error ? 100 : undefined}
            indeterminate={progress.stage !== 'ready' && !error}
          />
        </div>

        <PhaseChecklist phases={phases} sx={stylexStyles2.rule9} />

        {error && onRetry ? (
          <button type="button" onClick={onRetry} sx={stylexStyles2.rule10}>
            <LuRotateCw sx={stylexStyles.rule3} aria-hidden="true" />
            Retry
          </button>
        ) : null}
      </div>
    </div>
  )
}
