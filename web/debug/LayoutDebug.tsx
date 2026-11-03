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
    display: 'flex',
    cursor: 'pointer',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
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
    display: 'flex',
    height: '100%',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-background'],
    opacity: 0.25,
  },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-brand'],
  },
  rule9: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-window-border'],
  },
  rule10: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule11: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    paddingBottom: themeLayout['--spacing'],
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule12: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule13: {
    containerType: 'inline-size',
    display: 'flex',
    width: '100%',
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background'],
  },
  rule14: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule15: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
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
  rule16: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule17: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-3xl)',
    lineHeight: 'var(--tw-leading, var(--text-3xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule18: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule19: {
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule20: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule21: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule22: {
    overflow: 'hidden',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule23: { display: 'flex', flexDirection: 'column' },
  rule24: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule25: {
    marginLeft: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule26: {
    display: 'flex',
    cursor: 'pointer',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule27: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
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
    display: 'flex',
    alignItems: 'baseline',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule30: {
    display: 'flex',
    alignItems: 'baseline',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule31: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule32: {
    fontSize: '10px',
    lineHeight: 1.5,
    color: themeColors['--color-foreground-alt'],
  },
  rule33: {
    flexShrink: 0,
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-brand'],
  },
  rule34: {
    overflowX: 'auto',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
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
      className="layout-debug-frame shell-flexlayout"
      sx={stylexStyles3.rule1}
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
