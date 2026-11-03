import { useCallback, useMemo, useState } from 'react'
import { OptimizedLayout, Model, TabNode, IJsonModel } from '@aptre/flex-layout'
import { LuArrowLeft, LuFile } from '@s4wave/web/images/icons.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

import { groupMarker } from '../style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    overflow: 'hidden',
    backgroundColor: themeColors['--color-editor-border'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    cursor: 'pointer',
    display: 'flex',
    flexDirection: 'column',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule3: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    alignItems: 'center',
    backgroundColor: themeColors['--color-background'],
    display: 'flex',
    justifyContent: 'center',
    opacity: 0.25,
    height: '100%',
    width: '100%',
  },
  rule2: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule6: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule7: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    color: themeColors['--color-brand'],
    fontSize: '10px',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 1.5,
  },
  rule9: {
    borderColor: themeColors['--color-window-border'],
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
  },
  rule10: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule11: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    paddingBottom: themeLayout['--spacing'],
  },
  rule12: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule13: {
    flex: 1,
    backgroundColor: themeColors['--color-background'],
    containerType: 'inline-size',
    display: 'flex',
    flexDirection: 'column',
    overflowY: 'auto',
    width: '100%',
  },
  rule14: {
    marginInline: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    maxWidth: 'var(--container-5xl)',
    width: '100%',
  },
  rule15: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule16: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule17: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-3xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-3xl--line-height))',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule18: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule19: {
    color: themeColors['--color-text-muted'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule20: {
    gap: `calc(${themeLayout['--spacing']} * 10)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule21: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule22: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
  },
  rule23: { display: 'flex', flexDirection: 'column' },
  rule24: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule25: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginLeft: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule26: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    cursor: 'pointer',
    display: 'flex',
    flexDirection: 'column',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule27: {
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule28: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
  },
  rule29: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'baseline',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule30: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'baseline',
    display: 'flex',
  },
  rule31: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule32: {
    color: themeColors['--color-foreground-alt'],
    fontSize: '10px',
    lineHeight: 1.5,
  },
  rule33: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    color: themeColors['--color-brand'],
    flexShrink: 0,
    fontSize: '10px',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 1.5,
  },
  rule34: {
    '--tw-leading': 'var(--leading-relaxed)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    color: themeColors['--color-foreground-alt'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    overflowX: 'auto',
  },
  rule35: { paddingBottom: `calc(${themeLayout['--spacing']} * 8)` },
})

// Variant CSS lives in web/style/app.css after the baked Heavy Frost styles.
// Selectors omit :not(.flexlayout__tab *) because this page renders inside the
// shell's tab content panel.

type TabBarVariant =
  | 'frosted-glass'
  | 'wire-outline'
  | 'brand-strip'
  | 'soft-blend'
  | 'elevated'
  | 'flat-segment'

// Demo model — split layout with 2 tabsets and sample tabs.
// Unique IDs per instance prevent DOM conflicts.
function makeDemoModel(prefix: string): IJsonModel {
  return {
    global: {
      tabEnableRename: false,
      tabEnableClose: false,
      tabSetEnableMaximize: false,
      splitterSize: 4,
      splitterExtra: 0,
      tabDragSpeed: 0.1,
      enableEdgeDock: false,
      tabSetEnableDivide: false,
      tabEnableRenderOnDemand: false,
      tabSetEnableDeleteWhenEmpty: false,
    },
    borders: [],
    layout: {
      type: 'row',
      weight: 100,
      children: [
        {
          type: 'tabset',
          weight: 60,
          selected: 1,
          id: `${prefix}-ts1`,
          children: [
            {
              type: 'tab',
              name: 'Home',
              component: 'sample',
              id: `${prefix}-home`,
            },
            {
              type: 'tab',
              name: 'main.tsx',
              component: 'sample',
              id: `${prefix}-editor`,
            },
            {
              type: 'tab',
              name: 'Terminal',
              component: 'sample',
              id: `${prefix}-term`,
            },
            {
              type: 'tab',
              name: 'Settings',
              component: 'sample',
              id: `${prefix}-set`,
            },
          ],
        },
        {
          type: 'tabset',
          weight: 40,
          id: `${prefix}-ts2`,
          children: [
            {
              type: 'tab',
              name: 'Output',
              component: 'sample',
              id: `${prefix}-out`,
            },
            {
              type: 'tab',
              name: 'Console',
              component: 'sample',
              id: `${prefix}-con`,
            },
          ],
        },
      ],
    },
  }
}

function SampleContent({ node }: { node: TabNode }) {
  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <LuFile sx={stylexStyles.rule3} />
        <span sx={stylexStyles.rule4}>{node.getName()}</span>
      </div>
    </div>
  )
}

function VariantLayout({
  variant,
  height = 220,
  suffix = '',
  modifiers,
}: {
  variant: TabBarVariant
  height?: number
  suffix?: string
  modifiers?: Record<string, string>
}) {
  const prefix = `${variant}${suffix}`
  const model = useMemo(() => Model.fromJson(makeDemoModel(prefix)), [prefix])
  const renderTab = useCallback(
    (node: TabNode) => <SampleContent node={node} />,
    [],
  )

  const dataAttrs = useMemo(() => {
    const attrs: Record<string, string> = {
      'data-variant': variant,
      'data-testid': `variant-${variant}`,
    }
    if (modifiers) {
      for (const [k, v] of Object.entries(modifiers)) {
        attrs[`data-${k}`] = v
      }
    }
    return attrs
  }, [variant, modifiers])

  return (
    <div
      className="shell-flexlayout"
      sx={[stylexStyles3.rule1, previewFrameStyles.frame]}
      style={{ '--layout-debug-height': `${height}px` }}
      {...dataAttrs}
    >
      <OptimizedLayout model={model} renderTab={renderTab} />
    </div>
  )
}

function VariantPreview({
  id,
  name,
  description,
  children,
  selected,
  onSelect,
}: {
  id: string
  name: string
  description: string
  children: React.ReactNode
  selected: boolean
  onSelect: () => void
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      data-testid={`variant-card-${id}`}
      sx={[
        [stylexStyles2.rule1, groupMarker],
        selected ? stylexStyles2.rule2 : stylexStyles2.rule3,
      ]}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        onSelect()
      }}
    >
      <div sx={stylexStyles.rule5}>
        <div>
          <h3 sx={stylexStyles.rule6}>{name}</h3>
          <p sx={stylexStyles.rule7}>{description}</p>
        </div>
        {selected && <span sx={stylexStyles.rule8}>Selected</span>}
      </div>
      <div sx={stylexStyles.rule9}>{children}</div>
    </div>
  )
}

const VARIANTS: {
  id: TabBarVariant
  name: string
  description: string
}[] = [
  {
    id: 'frosted-glass',
    name: 'Frosted Glass',
    description:
      'Translucent blur on inactive, opaque on active. Warm neutral frost.',
  },
  {
    id: 'wire-outline',
    name: 'Wire Outline',
    description:
      'Thin border outlines, transparent fill. Brand-colored active border.',
  },
  {
    id: 'brand-strip',
    name: 'Brand Strip',
    description:
      'Clean flat tabs with a brand-colored top accent stripe on active.',
  },
  {
    id: 'soft-blend',
    name: 'Soft Blend',
    description: 'No borders, subtle filled backgrounds, smooth transitions.',
  },
  {
    id: 'elevated',
    name: 'Elevated Card',
    description: 'Raised active tab with shadow and depth, card-like lift.',
  },
  {
    id: 'flat-segment',
    name: 'Flat Segment',
    description: 'Connected segments — no gaps, sharp edges, divider lines.',
  },
]

function Section({
  title,
  subtitle,
  children,
}: {
  title: string
  subtitle?: string
  children: React.ReactNode
}) {
  return (
    <section sx={stylexStyles.rule10}>
      <div>
        <h2 sx={stylexStyles.rule11}>{title}</h2>
        {subtitle && <p sx={stylexStyles.rule12}>{subtitle}</p>}
      </div>
      {children}
    </section>
  )
}

interface ModifierOption {
  label: string
  value: string
  description: string
}

interface ModifierSection {
  title: string
  subtitle: string
  attrKey: string
  options: ModifierOption[]
}

const MODIFIER_SECTIONS: ModifierSection[] = [
  {
    title: 'Density',
    subtitle: 'Tab height, padding, and font sizing.',
    attrKey: 'density',
    options: [
      {
        label: 'Compact',
        value: 'compact',
        description: '18px height, tighter padding',
      },
      {
        label: 'Default',
        value: 'default',
        description: 'Variant native sizing (22px)',
      },
      {
        label: 'Spacious',
        value: 'spacious',
        description: '28px height, wider padding',
      },
    ],
  },
  {
    title: 'Tab Gaps',
    subtitle: 'Spacing between tab buttons.',
    attrKey: 'gaps',
    options: [
      { label: 'Flush', value: 'flush', description: 'No gap — tabs touching' },
      { label: 'Tight', value: 'tight', description: '1px hairline gap' },
      { label: 'Default', value: 'default', description: 'Variant native gap' },
      { label: 'Wide', value: 'wide', description: '6px gap between tabs' },
    ],
  },
  {
    title: 'Border Radius',
    subtitle: 'Corner rounding on tab buttons.',
    attrKey: 'radius',
    options: [
      { label: 'Sharp', value: 'sharp', description: 'Square corners (0px)' },
      {
        label: 'Soft',
        value: 'soft',
        description: 'Slight rounding (3px top)',
      },
      {
        label: 'Default',
        value: 'default',
        description: 'Variant native radius',
      },
      {
        label: 'Round',
        value: 'round',
        description: 'Heavy rounding (8px top)',
      },
    ],
  },
  {
    title: 'Active Emphasis',
    subtitle: 'How strongly the selected tab stands out.',
    attrKey: 'emphasis',
    options: [
      {
        label: 'Subtle',
        value: 'subtle',
        description: 'Barely distinguishable active state',
      },
      {
        label: 'Default',
        value: 'default',
        description: 'Variant native emphasis',
      },
      {
        label: 'Bold',
        value: 'bold',
        description: 'Strong glow, brighter text, shadow',
      },
    ],
  },
]

export function LayoutDebug({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  const [selectedVariant, setSelectedVariant] =
    useState<TabBarVariant>('frosted-glass')
  const [selectedModifiers, setSelectedModifiers] = useState<
    Record<string, string>
  >({
    density: 'default',
    gaps: 'default',
    radius: 'default',
    emphasis: 'default',
  })

  const candidateModifiers = useMemo(() => {
    const mods: Record<string, string> = {}
    for (const [k, v] of Object.entries(selectedModifiers)) {
      if (v !== 'default') mods[k] = v
    }
    return Object.keys(mods).length > 0 ? mods : undefined
  }, [selectedModifiers])

  const config = useMemo(() => {
    const obj: Record<string, string> = { variant: selectedVariant }
    for (const [k, v] of Object.entries(selectedModifiers)) {
      if (v !== 'default') obj[k] = v
    }
    return obj
  }, [selectedVariant, selectedModifiers])

  return (
    <div sx={[stylexStyles.rule13, sx]}>
      <div sx={stylexStyles.rule14}>
        <button type="button" onClick={goBack} sx={stylexStyles.rule15}>
          <LuArrowLeft sx={stylexStyles.rule16} />
          Back to home
        </button>

        <h1 sx={stylexStyles.rule17}>Layout UI Lab</h1>
        <p sx={stylexStyles.rule18}>
          Compare tab styling variants using real FlexLayout instances. Click to
          select. Each preview uses the baked Heavy Frost base with
          variant-specific CSS overrides.
        </p>
        <p sx={stylexStyles.rule19}>
          Split view with two tabsets and a splitter. Compare tab shape, color,
          frost/transparency, borders, shadows, and spacing across variants.
        </p>

        <div sx={stylexStyles.rule20}>
          <Section title="Tab Bar Variants">
            <div sx={stylexStyles.rule21}>
              {VARIANTS.map((v) => (
                <VariantPreview
                  key={v.id}
                  id={v.id}
                  name={v.name}
                  description={v.description}
                  selected={selectedVariant === v.id}
                  onSelect={() => setSelectedVariant(v.id)}
                >
                  <VariantLayout variant={v.id} />
                </VariantPreview>
              ))}
            </div>
          </Section>

          <Section title="Selected Variant — Enlarged">
            <div sx={stylexStyles.rule22}>
              {(() => {
                const v = VARIANTS.find((x) => x.id === selectedVariant)
                if (!v) return null
                return (
                  <div sx={stylexStyles.rule23}>
                    <div sx={stylexStyles.rule24}>
                      <span sx={stylexStyles.rule6}>{v.name}</span>
                      <span sx={stylexStyles.rule25}>{v.description}</span>
                    </div>
                    <VariantLayout
                      key={`enlarged-${v.id}`}
                      variant={v.id}
                      height={300}
                      suffix="-lg"
                    />
                  </div>
                )
              })()}
            </div>
          </Section>

          {MODIFIER_SECTIONS.map((section) => (
            <Section
              key={section.attrKey}
              title={`${section.title} — ${VARIANTS.find((v) => v.id === selectedVariant)?.name ?? selectedVariant}`}
              subtitle={section.subtitle}
            >
              <div sx={stylexStyles.rule21}>
                {section.options.map((opt) => {
                  const picked =
                    selectedModifiers[section.attrKey] === opt.value
                  return (
                    <div
                      role="button"
                      tabIndex={0}
                      key={opt.value}
                      sx={[
                        stylexStyles.rule26,
                        picked ? stylexStyles.rule27 : stylexStyles.rule28,
                      ]}
                      onClick={() =>
                        setSelectedModifiers((prev) => ({
                          ...prev,
                          [section.attrKey]: opt.value,
                        }))
                      }
                      onKeyDown={(event) => {
                        if (event.key !== 'Enter' && event.key !== ' ') return
                        event.preventDefault()
                        setSelectedModifiers((prev) => ({
                          ...prev,
                          [section.attrKey]: opt.value,
                        }))
                      }}
                    >
                      <div sx={stylexStyles.rule29}>
                        <div sx={stylexStyles.rule30}>
                          <span sx={stylexStyles.rule31}>{opt.label}</span>
                          <span sx={stylexStyles.rule32}>
                            {opt.description}
                          </span>
                        </div>
                        {picked && (
                          <span sx={stylexStyles.rule33}>Selected</span>
                        )}
                      </div>
                      <div sx={stylexStyles.rule9}>
                        <VariantLayout
                          key={`${section.attrKey}-${opt.value}-${selectedVariant}`}
                          variant={selectedVariant}
                          height={160}
                          suffix={`-${section.attrKey}-${opt.value}`}
                          modifiers={
                            opt.value === 'default'
                              ? undefined
                              : { [section.attrKey]: opt.value }
                          }
                        />
                      </div>
                    </div>
                  )
                })}
              </div>
            </Section>
          ))}

          <Section
            title="Candidate Preview"
            subtitle={`${VARIANTS.find((v) => v.id === selectedVariant)?.name ?? selectedVariant} with your selected modifiers applied.`}
          >
            <div sx={stylexStyles.rule22}>
              <div sx={stylexStyles.rule23}>
                <div sx={stylexStyles.rule24}>
                  <span sx={stylexStyles.rule6}>
                    {VARIANTS.find((v) => v.id === selectedVariant)?.name ??
                      selectedVariant}
                  </span>
                  <span sx={stylexStyles.rule25}>
                    {Object.entries(selectedModifiers)
                      .flatMap(([k, v]) =>
                        v !== 'default' ? [`${k}: ${v}`] : [],
                      )
                      .join(', ') || 'all defaults'}
                  </span>
                </div>
                <VariantLayout
                  key={`candidate-${selectedVariant}-${JSON.stringify(selectedModifiers)}`}
                  variant={selectedVariant}
                  height={300}
                  suffix="-candidate"
                  modifiers={candidateModifiers}
                />
              </div>
            </div>
          </Section>

          <Section title="Configuration">
            <pre sx={stylexStyles.rule34}>
              {JSON.stringify(config, null, 2)}
            </pre>
          </Section>

          <div sx={stylexStyles.rule35} />
        </div>
      </div>
    </div>
  )
}

const previewFrameStyles = stylex.create({
  frame: { position: 'relative', height: 'var(--layout-debug-height)' },
})
