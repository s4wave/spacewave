import { useCallback, useMemo, useState } from 'react'
import { OptimizedLayout, Model, TabNode, IJsonModel } from '@aptre/flex-layout'
import { LuArrowLeft, LuFile } from 'react-icons/lu'
import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
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
    <div className="bg-background flex h-full w-full items-center justify-center opacity-25">
      <div className="flex flex-col items-center gap-1">
        <LuFile className="h-5 w-5" />
        <span className="text-[10px] font-medium">{node.getName()}</span>
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
      className="shell-flexlayout bg-editor-border overflow-hidden"
      style={{ position: 'relative', height }}
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
      data-testid={`variant-card-${id}`}
      className={cn(
        'group flex cursor-pointer flex-col gap-3 rounded-xl border p-4 transition-all',
        selected ?
          'border-brand/50 bg-brand/5 ring-brand/20 ring-1'
        : 'border-foreground/8 hover:border-foreground/15 bg-background-card/30',
      )}
      onClick={onSelect}
    >
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-foreground text-sm font-semibold">{name}</h3>
          <p className="text-foreground-alt text-xs">{description}</p>
        </div>
        {selected && (
          <span className="bg-brand/20 text-brand rounded-full px-2 py-0.5 text-[10px] font-semibold">
            Selected
          </span>
        )}
      </div>
      <div className="border-window-border overflow-hidden rounded-lg border">
        {children}
      </div>
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
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-foreground border-b border-white/10 pb-1 text-lg font-semibold">
          {title}
        </h2>
        {subtitle && (
          <p className="text-foreground-alt mt-1 text-xs">{subtitle}</p>
        )}
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

export function LayoutDebug() {
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
    <div className="bg-background @container flex w-full flex-1 flex-col overflow-y-auto">
      <div className="mx-auto w-full max-w-5xl px-4 py-6 @lg:px-8">
        <button
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground mb-6 flex cursor-pointer items-center gap-2 transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          Back to home
        </button>

        <h1 className="text-foreground mb-2 text-3xl font-bold">
          Layout UI Lab
        </h1>
        <p className="text-foreground-alt mb-2 text-sm">
          Compare tab styling variants using real FlexLayout instances. Click to
          select. Each preview uses the baked Heavy Frost base with
          variant-specific CSS overrides.
        </p>
        <p className="text-text-muted mb-8 text-xs">
          Split view with two tabsets and a splitter. Compare tab shape, color,
          frost/transparency, borders, shadows, and spacing across variants.
        </p>

        <div className="flex flex-col gap-10">
          <Section title="Tab Bar Variants">
            <div className="grid gap-4 @lg:grid-cols-2">
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
            <div className="border-foreground/8 overflow-hidden rounded-xl border">
              {(() => {
                const v = VARIANTS.find((x) => x.id === selectedVariant)
                if (!v) return null
                return (
                  <div className="flex flex-col">
                    <div className="bg-background-card/50 border-b border-white/5 px-4 py-2">
                      <span className="text-foreground text-sm font-semibold">
                        {v.name}
                      </span>
                      <span className="text-foreground-alt ml-2 text-xs">
                        {v.description}
                      </span>
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
              <div className="grid gap-4 @lg:grid-cols-2">
                {section.options.map((opt) => {
                  const picked =
                    selectedModifiers[section.attrKey] === opt.value
                  return (
                    <div
                      key={opt.value}
                      className={cn(
                        'flex cursor-pointer flex-col gap-1.5 rounded-xl border p-3 transition-all',
                        picked ?
                          'border-brand/50 bg-brand/5 ring-brand/20 ring-1'
                        : 'border-foreground/8 hover:border-foreground/15',
                      )}
                      onClick={() =>
                        setSelectedModifiers((prev) => ({
                          ...prev,
                          [section.attrKey]: opt.value,
                        }))
                      }
                    >
                      <div className="flex items-baseline justify-between gap-2">
                        <div className="flex items-baseline gap-2">
                          <span className="text-foreground text-xs font-semibold">
                            {opt.label}
                          </span>
                          <span className="text-foreground-alt text-[10px]">
                            {opt.description}
                          </span>
                        </div>
                        {picked && (
                          <span className="bg-brand/20 text-brand shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold">
                            Selected
                          </span>
                        )}
                      </div>
                      <div className="border-window-border overflow-hidden rounded-lg border">
                        <VariantLayout
                          key={`${section.attrKey}-${opt.value}-${selectedVariant}`}
                          variant={selectedVariant}
                          height={160}
                          suffix={`-${section.attrKey}-${opt.value}`}
                          modifiers={
                            opt.value === 'default' ?
                              undefined
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
            <div className="border-foreground/8 overflow-hidden rounded-xl border">
              <div className="flex flex-col">
                <div className="bg-background-card/50 border-b border-white/5 px-4 py-2">
                  <span className="text-foreground text-sm font-semibold">
                    {VARIANTS.find((v) => v.id === selectedVariant)?.name ??
                      selectedVariant}
                  </span>
                  <span className="text-foreground-alt ml-2 text-xs">
                    {Object.entries(selectedModifiers)
                      .filter(([, v]) => v !== 'default')
                      .map(([k, v]) => `${k}: ${v}`)
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
            <pre className="bg-background-card/50 text-foreground-alt overflow-x-auto rounded-lg border border-white/5 p-4 font-mono text-xs leading-relaxed">
              {JSON.stringify(config, null, 2)}
            </pre>
          </Section>

          <div className="pb-8" />
        </div>
      </div>
    </div>
  )
}
