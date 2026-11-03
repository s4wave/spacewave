import { useCallback } from 'react'
import {
  LuArrowLeft,
  LuEyeOff,
  LuGitBranch,
  LuLock,
  LuLocateFixed,
  LuPlus,
  LuTrash2,
} from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'

import { GraphLinkPill as ProductionGraphLinkPill } from '@s4wave/app/canvas/GraphLinkPill.js'
import type { EphemeralEdge } from '@s4wave/app/canvas/types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'space-between',
    height: `calc(${themeLayout['--spacing']} * 24)`,
    width: `calc(${themeLayout['--spacing']} * 44)`,
  },
  rule2: {
    '--tw-border-style': 'dashed',
    borderStyle: 'dashed',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 40%, transparent)`,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule5: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule6: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
  },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: themeLayout['--spacing'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: themeLayout['--spacing'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    color: themeColors['--color-warning'],
  },
  rule12: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: '.25rem',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
  },
  rule13: {
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule14: {
    borderRadius: themeRadii['--radius-md'],
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
    display: 'flex',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule15: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule16: {
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule17: {
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
    color: themeColors['--color-destructive'],
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule18: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    alignItems: 'center',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-foreground'],
    display: 'flex',
  },
  rule19: {
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: '.55rem',
    lineHeight: 1.5,
  },
  rule20: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
  },
  rule21: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule22: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
  },
  rule23: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule24: { opacity: 0.55 },
  rule25: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule26: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule27: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    fontWeight: themeFonts['--font-weight-medium'],
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    maxWidth: `calc(${themeLayout['--spacing']} * 28)`,
  },
  rule28: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 52)`,
  },
  rule29: {
    position: 'absolute',
    zIndex: 10,
    left: `calc(${themeLayout['--spacing']} * 8)`,
    top: `calc(${themeLayout['--spacing']} * 14)`,
  },
  rule30: {
    position: 'absolute',
    zIndex: 10,
    right: `calc(${themeLayout['--spacing']} * 8)`,
    top: `calc(${themeLayout['--spacing']} * 14)`,
  },
  rule31: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    position: 'absolute',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    left: `calc(${themeLayout['--spacing']} * 52)`,
    right: `calc(${themeLayout['--spacing']} * 52)`,
    top: `calc(${themeLayout['--spacing']} * 26)`,
  },
  rule32: { '--tw-border-style': 'dashed', borderStyle: 'dashed' },
  rule33: {
    '--tw-translate-x': 'calc(calc(1 / 2 * 100%) * -1)',
    position: 'absolute',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    zIndex: 20,
    left: '50%',
    top: `calc(${themeLayout['--spacing']} * 23)`,
  },
  rule34: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: '.55rem',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    position: 'absolute',
    textTransform: 'uppercase',
    bottom: `calc(${themeLayout['--spacing']} * 3)`,
    left: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule35: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    position: 'absolute',
    bottom: `calc(${themeLayout['--spacing']} * 3)`,
    right: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule36: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule37: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'flex-end',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule38: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule39: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule40: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule41: { gap: `calc(${themeLayout['--spacing']} * 3)`, display: 'grid' },
  rule42: {
    overflow: 'auto',
    backgroundColor: themeColors['--color-background'],
    containerType: 'inline-size',
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule43: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule44: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule45: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule46: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule47: {
    flex: 1,
    overflow: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule48: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    marginInline: 'auto',
    display: 'flex',
    flexDirection: 'column',
    maxWidth: 'var(--container-5xl)',
    width: '100%',
  },
})

type GraphLinkState = 'unloaded' | 'loaded'
type PillVariant = 'compact' | 'balanced' | 'metadata'

interface GraphLinkFixture {
  id: string
  state: GraphLinkState
  predicate: string
  targetKey?: string
  targetLabel: string
  targetType: string
  direction: 'out' | 'in'
  hidden?: boolean
  policy?: 'normal' | 'protected' | 'deletable'
  truncated?: boolean
}

interface PillVariantFixture {
  id: PillVariant
  name: string
  detail: string
}

const primaryFixtures: GraphLinkFixture[] = [
  {
    id: 'source-parent-workdir',
    state: 'unloaded',
    predicate: 'parent',
    targetLabel: 'workdir/main',
    targetType: 'Drive',
    direction: 'out',
    policy: 'normal',
  },
  {
    id: 'repo-worktree-source',
    state: 'loaded',
    predicate: 'git/worktree',
    targetLabel: 'repo/main',
    targetType: 'Git Repo',
    direction: 'in',
    policy: 'normal',
  },
]

const stateFixtures: GraphLinkFixture[] = [
  {
    id: 'hidden-parent',
    state: 'unloaded',
    predicate: 'parent',
    targetLabel: 'archive/notes',
    targetType: 'Drive',
    direction: 'out',
    hidden: true,
    policy: 'normal',
  },
  {
    id: 'truncated-children',
    state: 'unloaded',
    predicate: 'child',
    targetLabel: 'project-assets',
    targetType: 'Folder',
    direction: 'out',
    policy: 'normal',
    truncated: true,
  },
  {
    id: 'protected-type',
    state: 'loaded',
    predicate: '<type>',
    targetLabel: 'types/canvas',
    targetType: 'ObjectType',
    direction: 'out',
    policy: 'protected',
  },
  {
    id: 'deletable-related',
    state: 'loaded',
    predicate: 'relatedTo',
    targetLabel: 'scratch-plan',
    targetType: 'Canvas',
    direction: 'in',
    policy: 'deletable',
  },
]

const variants: PillVariantFixture[] = [
  {
    id: 'compact',
    name: 'Compact',
    detail: 'Small chip. Fast scanning, minimal metadata.',
  },
  {
    id: 'balanced',
    name: 'Balanced',
    detail: 'Selected direction. Compact padding with readable inline context.',
  },
  {
    id: 'metadata',
    name: 'Metadata',
    detail: 'More type context. Higher readability, larger edge footprint.',
  },
]

function GraphNode({
  label,
  detail,
  muted,
}: {
  label: string
  detail: string
  muted?: boolean
}) {
  return (
    <div
      sx={[stylexStyles.rule1, muted ? stylexStyles.rule2 : stylexStyles.rule3]}
    >
      <div sx={stylexStyles.rule4}>
        <div sx={stylexStyles.rule5}>
          <LuGitBranch sx={stylexStyles.rule6} />
        </div>
        <span sx={stylexStyles.rule7}>{label}</span>
      </div>
      <span sx={stylexStyles.rule8}>{detail}</span>
    </div>
  )
}

// GraphLinkPillBadges renders the target type, cap, and protection badges.
function GraphLinkPillBadges({
  fixture,
  variant,
}: {
  fixture: GraphLinkFixture
  variant: PillVariant
}) {
  if (variant === 'compact') return null

  return (
    <>
      <span
        sx={[stylexStyles.rule9, variant === 'metadata' && stylexStyles.rule10]}
      >
        {fixture.targetType}
      </span>
      {fixture.truncated && <span sx={stylexStyles.rule11}>capped</span>}
      {fixture.policy === 'protected' && (
        <span sx={stylexStyles.rule12}>
          <LuLock sx={stylexStyles.rule13} />
          protected
        </span>
      )}
    </>
  )
}

// GraphLinkPillActions renders the focus or load, hide, and delete buttons.
function GraphLinkPillActions({
  fixture,
  compact,
}: {
  fixture: GraphLinkFixture
  compact: boolean
}) {
  const loaded = fixture.state === 'loaded'
  const hidden = fixture.hidden ?? false

  return (
    <>
      <button
        type="button"
        sx={stylexStyles.rule14}
        aria-label={`${loaded ? 'Focus' : 'Load'} ${fixture.targetLabel}`}
        disabled={hidden}
      >
        {loaded ? (
          <LuLocateFixed sx={stylexStyles.rule15} />
        ) : (
          <LuPlus sx={stylexStyles.rule15} />
        )}
        {!compact && (loaded ? 'Focus' : 'Load')}
      </button>
      <button
        type="button"
        sx={stylexStyles.rule16}
        aria-label={`Hide ${fixture.predicate} link`}
        disabled={hidden}
      >
        <LuEyeOff sx={stylexStyles.rule15} />
      </button>
      {fixture.policy === 'deletable' && (
        <button
          type="button"
          sx={stylexStyles.rule17}
          aria-label={`Delete ${fixture.predicate} link`}
        >
          <LuTrash2 sx={stylexStyles.rule15} />
        </button>
      )}
    </>
  )
}

// FixtureGraphLinkPill renders the static pill variants for visual comparison.
function FixtureGraphLinkPill({
  fixture,
  variant,
}: {
  fixture: GraphLinkFixture
  variant: PillVariant
}) {
  const compact = variant === 'compact'

  return (
    <div
      data-testid={`graph-link-pill-${variant}-${fixture.state}`}
      sx={[
        stylexStyles.rule18,
        compact ? stylexStyles.rule19 : stylexStyles.rule20,
        variant === 'metadata' && stylexStyles.rule21,
        fixture.state === 'loaded' ? stylexStyles.rule22 : stylexStyles.rule23,
        !!fixture.hidden && stylexStyles.rule24,
      ]}
    >
      <span sx={stylexStyles.rule25}>{fixture.predicate}</span>
      <span sx={stylexStyles.rule26}>/</span>
      <span sx={stylexStyles.rule27}>{fixture.targetLabel}</span>
      <GraphLinkPillBadges fixture={fixture} variant={variant} />
      <GraphLinkPillActions fixture={fixture} compact={compact} />
    </div>
  )
}

function GraphLinkPill({
  fixture,
  variant,
}: {
  fixture: GraphLinkFixture
  variant: PillVariant
}) {
  if (variant !== 'balanced') {
    return <FixtureGraphLinkPill fixture={fixture} variant={variant} />
  }

  return (
    <div data-testid={`graph-link-pill-${variant}-${fixture.state}`}>
      <ProductionGraphLinkPill
        edge={fixtureToEphemeralEdge(fixture)}
        loaded={fixture.state === 'loaded'}
        onPrimary={() => undefined}
        onHide={() => undefined}
      />
    </div>
  )
}

function fixtureToEphemeralEdge(fixture: GraphLinkFixture): EphemeralEdge {
  const policy = fixture.policy ?? 'normal'
  const targetKey = fixture.targetKey ?? `debug/${fixture.id}`
  return {
    renderKey: fixture.id,
    subject: '<debug/source>',
    predicate: fixture.predicate,
    object: `<${targetKey}>`,
    label: undefined,
    sourceNodeId: 'debug-source',
    sourceObjectKey: 'debug/source',
    sourceGroupKey: 'debug-source',
    sourceGroupIndex: 0,
    sourceGroupOffset: 0,
    outgoingTruncated: fixture.truncated ?? false,
    incomingTruncated: false,
    hiddenCount: fixture.hidden ? 1 : 0,
    direction: fixture.direction,
    linkedObjectKey: targetKey,
    linkedObjectLabel: fixture.targetLabel,
    linkedObjectType: fixture.targetType,
    linkedObjectTypeLabel: fixture.targetType,
    hideable: policy !== 'protected',
    userRemovable: policy === 'deletable',
    protected: policy === 'protected',
    ownerManaged: policy === 'protected',
    targetNodeId: fixture.state === 'loaded' ? 'debug-target' : undefined,
    stubX: fixture.state === 'loaded' ? undefined : 0,
    stubY: fixture.state === 'loaded' ? undefined : 0,
  }
}

function GraphLinkPreview({
  fixture,
  variant,
}: {
  fixture: GraphLinkFixture
  variant: PillVariant
}) {
  const loaded = fixture.state === 'loaded'
  const hidden = fixture.hidden ?? false
  return (
    <div
      data-testid={`graph-link-preview-${variant}-${fixture.state}`}
      sx={stylexStyles.rule28}
    >
      <div sx={stylexStyles.rule29}>
        <GraphNode label="Canvas source" detail="world object node" />
      </div>

      <div sx={stylexStyles.rule30}>
        <GraphNode
          label={fixture.targetLabel}
          detail={
            loaded ? 'already loaded on canvas' : 'available graph target'
          }
          muted={!loaded}
        />
      </div>

      <div sx={[stylexStyles.rule31, !loaded && stylexStyles.rule32]} />

      <div sx={stylexStyles.rule33}>
        <GraphLinkPill fixture={fixture} variant={variant} />
      </div>

      <span sx={stylexStyles.rule34}>
        {fixture.direction === 'out' ? 'Outgoing' : 'Incoming'} /{' '}
        {hidden ? 'hidden' : fixture.state}
      </span>
      {hidden && <span sx={stylexStyles.rule35}>hidden on canvas</span>}
    </div>
  )
}

function VariantPreview({ variant }: { variant: PillVariantFixture }) {
  return (
    <section
      data-testid={`graph-link-variant-${variant.id}`}
      sx={stylexStyles.rule36}
    >
      <div sx={stylexStyles.rule37}>
        <div>
          <h2 sx={stylexStyles.rule38}>{variant.name}</h2>
          <p sx={stylexStyles.rule39}>{variant.detail}</p>
        </div>
        <span sx={stylexStyles.rule40}>{variant.id}</span>
      </div>
      <div sx={stylexStyles.rule41}>
        {primaryFixtures.map((fixture) => (
          <GraphLinkPreview
            key={fixture.id}
            fixture={fixture}
            variant={variant.id}
          />
        ))}
      </div>
    </section>
  )
}

function StateCoverage() {
  return (
    <section sx={stylexStyles.rule36}>
      <div>
        <h2 sx={stylexStyles.rule38}>State Coverage</h2>
        <p sx={stylexStyles.rule39}>
          Balanced pills with edge cases the production graph model must carry.
        </p>
      </div>
      <div sx={stylexStyles.rule41}>
        {stateFixtures.map((fixture) => (
          <GraphLinkPreview
            key={fixture.id}
            fixture={fixture}
            variant="balanced"
          />
        ))}
      </div>
    </section>
  )
}

// CanvasGraphLinksDebug renders static graph-link states for Canvas UI
// iteration before the production graph model is wired.
export function CanvasGraphLinksDebug({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  return (
    <div sx={[stylexStyles.rule42, sx]}>
      <div sx={stylexStyles.rule43}>
        <button
          type="button"
          onClick={goBack}
          aria-label="Go back"
          sx={stylexStyles.rule44}
        >
          <LuArrowLeft sx={stylexStyles.rule45} />
        </button>
        <span sx={stylexStyles.rule46}>Canvas Graph Links</span>
      </div>

      <div sx={stylexStyles.rule47}>
        <div sx={stylexStyles.rule48}>
          {variants.map((variant) => (
            <VariantPreview key={variant.id} variant={variant} />
          ))}
          <StateCoverage />
        </div>
      </div>
    </div>
  )
}
