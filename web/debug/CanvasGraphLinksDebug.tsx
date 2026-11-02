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
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 24)`,
    width: `calc(${themeLayout['--spacing']} * 44)`,
    flexDirection: 'column',
    justifyContent: 'space-between',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
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
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-brand'],
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule8: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule10: {
    borderRadius: '.25rem',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule11: {
    borderRadius: '.25rem',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    color: themeColors['--color-warning'],
  },
  rule12: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule13: {
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule14: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: themeRadii['--radius-md'],
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
  },
  rule15: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule16: {
    borderRadius: themeRadii['--radius-md'],
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
  },
  rule17: {
    borderRadius: themeRadii['--radius-md'],
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    color: themeColors['--color-destructive'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
  },
  rule18: {
    display: 'flex',
    alignItems: 'center',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    color: themeColors['--color-foreground'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule19: {
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.55rem',
    lineHeight: 1.5,
  },
  rule20: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: '.6rem',
    lineHeight: 1.5,
  },
  rule21: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
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
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
  },
  rule26: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule27: {
    maxWidth: `calc(${themeLayout['--spacing']} * 28)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule28: {
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 52)`,
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
  },
  rule29: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 14)`,
    left: `calc(${themeLayout['--spacing']} * 8)`,
    zIndex: 10,
  },
  rule30: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 14)`,
    right: `calc(${themeLayout['--spacing']} * 8)`,
    zIndex: 10,
  },
  rule31: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 26)`,
    right: `calc(${themeLayout['--spacing']} * 52)`,
    left: `calc(${themeLayout['--spacing']} * 52)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule32: { '--tw-border-style': 'dashed', borderStyle: 'dashed' },
  rule33: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 23)`,
    left: '50%',
    zIndex: 20,
    '--tw-translate-x': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
  },
  rule34: {
    position: 'absolute',
    bottom: `calc(${themeLayout['--spacing']} * 3)`,
    left: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    textTransform: 'uppercase',
  },
  rule35: {
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * 3)`,
    bottom: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule36: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule37: {
    display: 'flex',
    alignItems: 'flex-end',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule38: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule39: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule40: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule41: { display: 'grid', gap: `calc(${themeLayout['--spacing']} * 3)` },
  rule42: {
    containerType: 'inline-size',
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'auto',
    backgroundColor: themeColors['--color-background'],
  },
  rule43: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule44: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule45: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule46: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule47: {
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule48: {
    marginInline: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
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
export function CanvasGraphLinksDebug() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  return (
    <div sx={stylexStyles.rule42}>
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
