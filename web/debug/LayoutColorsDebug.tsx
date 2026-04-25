import { useCallback, useMemo, useState } from 'react'
import { OptimizedLayout, Model, TabNode, IJsonModel } from '@aptre/flex-layout'
import { LuArrowLeft, LuFile } from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

interface ColorScheme {
  id: string
  name: string
  description: string
  vars: Record<string, string>
}

// Monokai Spectrum palette reference (doom-monokai-spectrum-theme.el):
// bg:     oklch(0.232 0 0)        — neutral gray
// bg-alt: oklch(0.188 0 0)        — deeper
// base2:  oklch(0.279 0.001 295)  — selection, faint purple
// base3:  oklch(0.314 0.003 300)  — mode-line
// base4:  oklch(0.420 0.005 310)  — line numbers
// base6:  oklch(0.497 0.006 290)  — comments
// base7:  oklch(0.624 0.007 295)  — docs
// base8:  oklch(0.765 0.008 295)  — bright secondary
// fg:     oklch(0.963 0.02 300)   — purple-warm white
// red:    oklch(0.673 0.224 6.5)  — pink brand

const SCHEMES: ColorScheme[] = [
  {
    id: 'baseline',
    name: 'Current Baseline',
    description: 'Current warm theme. Hue 16.5 backgrounds, warm brand.',
    vars: {},
  },
  {
    id: 'full-monokai',
    name: 'Full Monokai',
    description:
      'Complete Monokai Spectrum. Neutral backgrounds, pink brand, purple-white text.',
    vars: {
      // Brand & accents
      '--color-brand': 'oklch(0.673 0.224 6.5)',
      '--color-brand-highlight': 'oklch(0.72 0.18 6.5)',
      '--color-primary': 'oklch(0.673 0.224 6.5)',
      // Backgrounds — achromatic
      '--color-background': 'oklch(0.232 0 0)',
      '--color-background-dark': 'oklch(0.156 0 0)',
      '--color-background-card': 'oklch(0.22 0 0)',
      '--color-background-card-alt': 'oklch(0.20 0 0)',
      '--color-background-primary': 'oklch(0.232 0 0)',
      '--color-background-secondary': 'oklch(0.25 0 0)',
      '--color-background-tertiary': 'oklch(0.28 0 0)',
      '--color-background-deep': 'oklch(0.188 0 0)',
      '--color-editor-border': 'oklch(0.17 0 0)',
      // Text — purple-warm white
      '--color-foreground-alt': 'oklch(0.90 0.01 300)',
      '--color-text-primary': 'oklch(0.963 0.02 300)',
      '--color-text-secondary': 'oklch(0.765 0.008 295)',
      '--color-text-muted': 'oklch(0.624 0.007 295)',
      '--color-text-header': 'oklch(0.963 0.02 300)',
      // Editor tabs
      '--color-editor-tab-active': 'oklch(0.279 0.001 295)',
      '--color-editor-tab-unfocused': 'oklch(0.25 0 0)',
      '--color-editor-tab-text': 'oklch(0.624 0.007 295)',
      '--color-editor-tab-text-active': 'oklch(0.963 0.02 300)',
      '--color-editor-tab-text-unfocused': 'oklch(0.765 0.008 295)',
      // Selection — faint purple (Monokai base2)
      '--color-ui-selected': 'oklch(0.279 0.001 295)',
      '--color-outliner-selected-highlight': 'oklch(0.26 0.001 295)',
      // File browser
      '--color-file-back': 'oklch(0.232 0 0)',
      '--color-file-sidebar': 'oklch(0.28 0 0)',
      '--color-file-sidebar-section': 'oklch(0.25 0 0)',
      '--color-file-sidebar-item-hover': 'oklch(0.673 0.10 6.5)',
      '--color-panel-header': 'oklch(0.232 0 0)',
      '--color-file-path-bar': 'oklch(0.28 0 0)',
      '--color-file-search-box': 'oklch(0.20 0 0)',
      '--color-file-folder-icon': 'oklch(0.673 0.224 6.5)',
    },
  },
  {
    id: 'neutral-bg',
    name: 'Neutral Backgrounds',
    description:
      'Achromatic backgrounds (no warm hue). Keep current brand and text.',
    vars: {
      '--color-background': 'oklch(0.232 0 0)',
      '--color-background-dark': 'oklch(0.156 0 0)',
      '--color-background-card': 'oklch(0.22 0 0)',
      '--color-background-card-alt': 'oklch(0.20 0 0)',
      '--color-background-primary': 'oklch(0.232 0 0)',
      '--color-background-secondary': 'oklch(0.25 0 0)',
      '--color-background-tertiary': 'oklch(0.28 0 0)',
      '--color-background-deep': 'oklch(0.188 0 0)',
      '--color-editor-border': 'oklch(0.17 0 0)',
      '--color-editor-tab-active': 'oklch(0.27 0 0)',
      '--color-editor-tab-unfocused': 'oklch(0.24 0 0)',
      '--color-file-back': 'oklch(0.232 0 0)',
      '--color-file-sidebar': 'oklch(0.28 0 0)',
      '--color-file-sidebar-section': 'oklch(0.25 0 0)',
      '--color-panel-header': 'oklch(0.232 0 0)',
      '--color-file-path-bar': 'oklch(0.28 0 0)',
      '--color-file-search-box': 'oklch(0.20 0 0)',
    },
  },
  {
    id: 'pink-brand',
    name: 'Pink Brand',
    description:
      'Monokai pink as brand/primary. Keep current warm backgrounds.',
    vars: {
      '--color-brand': 'oklch(0.673 0.224 6.5)',
      '--color-brand-highlight': 'oklch(0.72 0.18 6.5)',
      '--color-primary': 'oklch(0.673 0.224 6.5)',
      '--color-file-folder-icon': 'oklch(0.673 0.224 6.5)',
      '--color-file-sidebar-item-hover': 'oklch(0.673 0.10 6.5)',
    },
  },
  {
    id: 'purple-text',
    name: 'Purple Text',
    description: 'Monokai purple-white text on current warm backgrounds.',
    vars: {
      '--color-foreground-alt': 'oklch(0.90 0.01 300)',
      '--color-text-primary': 'oklch(0.963 0.02 300)',
      '--color-text-secondary': 'oklch(0.765 0.008 295)',
      '--color-text-muted': 'oklch(0.624 0.007 295)',
      '--color-text-header': 'oklch(0.963 0.02 300)',
      '--color-editor-tab-text': 'oklch(0.624 0.007 295)',
      '--color-editor-tab-text-active': 'oklch(0.963 0.02 300)',
      '--color-editor-tab-text-unfocused': 'oklch(0.765 0.008 295)',
    },
  },
  {
    id: 'hybrid',
    name: 'Hybrid',
    description:
      'Neutral backgrounds + warm brand + purple-tinted text. Best of both.',
    vars: {
      // Backgrounds — achromatic
      '--color-background': 'oklch(0.232 0 0)',
      '--color-background-dark': 'oklch(0.156 0 0)',
      '--color-background-card': 'oklch(0.22 0 0)',
      '--color-background-card-alt': 'oklch(0.20 0 0)',
      '--color-background-primary': 'oklch(0.232 0 0)',
      '--color-background-secondary': 'oklch(0.25 0 0)',
      '--color-background-tertiary': 'oklch(0.28 0 0)',
      '--color-background-deep': 'oklch(0.188 0 0)',
      '--color-editor-border': 'oklch(0.17 0 0)',
      // Text — subtle purple tint
      '--color-foreground-alt': 'oklch(0.90 0.01 300)',
      '--color-text-primary': 'oklch(0.963 0.02 300)',
      '--color-text-secondary': 'oklch(0.765 0.008 295)',
      '--color-text-muted': 'oklch(0.624 0.007 295)',
      '--color-text-header': 'oklch(0.963 0.02 300)',
      // Tabs
      '--color-editor-tab-active': 'oklch(0.27 0 0)',
      '--color-editor-tab-unfocused': 'oklch(0.24 0 0)',
      '--color-editor-tab-text': 'oklch(0.624 0.007 295)',
      '--color-editor-tab-text-active': 'oklch(0.963 0.02 300)',
      '--color-editor-tab-text-unfocused': 'oklch(0.765 0.008 295)',
      // File browser
      '--color-file-back': 'oklch(0.232 0 0)',
      '--color-file-sidebar': 'oklch(0.28 0 0)',
      '--color-file-sidebar-section': 'oklch(0.25 0 0)',
      '--color-panel-header': 'oklch(0.232 0 0)',
      '--color-file-path-bar': 'oklch(0.28 0 0)',
      '--color-file-search-box': 'oklch(0.20 0 0)',
    },
  },
]

// makeDemoModel creates a split FlexLayout model with sample tabs.
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

// SampleContent renders placeholder content inside a demo tab.
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

// DemoLayout renders a FlexLayout instance with optional CSS variable overrides.
function DemoLayout({
  prefix,
  height,
  vars,
}: {
  prefix: string
  height: number
  vars?: Record<string, string>
}) {
  const model = useMemo(() => Model.fromJson(makeDemoModel(prefix)), [prefix])
  const renderTab = useCallback(
    (node: TabNode) => <SampleContent node={node} />,
    [],
  )

  return (
    <div
      className="rounded-lg p-4"
      style={{ backgroundColor: 'oklch(0.22 0 0)', ...vars }}
    >
      <div className="overflow-hidden" style={{ position: 'relative', height }}>
        <OptimizedLayout model={model} renderTab={renderTab} />
      </div>
    </div>
  )
}

// Section renders a titled section with optional subtitle.
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

// SchemePreviewCard renders a clickable card containing a DemoLayout
// wrapped in color scope overrides.
function SchemePreviewCard({
  scheme,
  selected,
  onSelect,
}: {
  scheme: ColorScheme
  selected: boolean
  onSelect: () => void
}) {
  return (
    <div
      data-testid={`scheme-card-${scheme.id}`}
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
          <h3 className="text-foreground text-sm font-semibold">
            {scheme.name}
          </h3>
          <p className="text-foreground-alt text-xs">{scheme.description}</p>
        </div>
        {selected && (
          <span className="bg-brand/20 text-brand rounded-full px-2 py-0.5 text-[10px] font-semibold">
            Selected
          </span>
        )}
      </div>
      <div
        className="border-window-border overflow-hidden rounded-lg border"
        style={scheme.vars}
      >
        <DemoLayout
          prefix={`card-${scheme.id}`}
          height={160}
          vars={scheme.vars}
        />
      </div>
    </div>
  )
}

// SwatchRow renders a row of color swatches for a given scheme.
const SWATCH_KEYS = [
  { key: '--color-brand', label: 'brand' },
  { key: '--color-primary', label: 'primary' },
  { key: '--color-text-primary', label: 'text' },
  { key: '--color-text-muted', label: 'muted' },
  { key: '--color-editor-border', label: 'border' },
  { key: '--color-background', label: 'bg' },
  { key: '--color-file-folder-icon', label: 'folder' },
  { key: '--color-ui-selected', label: 'selected' },
]

function SwatchRow({ scheme }: { scheme: ColorScheme }) {
  return (
    <div className="flex flex-wrap gap-3">
      {SWATCH_KEYS.map((s) => {
        const value = scheme.vars[s.key]
        return (
          <div key={s.key} className="flex flex-col items-center gap-1">
            <div
              className="h-8 w-8 rounded border border-white/10"
              style={{ backgroundColor: value || `var(${s.key})` }}
            />
            <span className="text-foreground-alt text-[10px]">{s.label}</span>
          </div>
        )
      })}
    </div>
  )
}

// LayoutColorsDebug renders the Monokai Spectrum comparison page.
export function LayoutColorsDebug() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  const [selectedId, setSelectedId] = useState(SCHEMES[0].id)

  const selected = useMemo(
    () => SCHEMES.find((s) => s.id === selectedId) ?? SCHEMES[0],
    [selectedId],
  )

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
          Monokai Spectrum Lab
        </h1>
        <p className="text-foreground-alt mb-2 text-sm">
          Compare the current warm theme against Monokai Spectrum-inspired
          variants. Each isolates a different axis: backgrounds, brand color,
          text tint, or the full combination.
        </p>
        <p className="text-text-muted mb-8 text-xs">
          Click a variant to select it. The enlarged preview and swatches update
          to show the selected scheme.
        </p>

        <div className="flex flex-col gap-10">
          <Section title="Monokai Variants">
            <div className="grid gap-4 @lg:grid-cols-2">
              {SCHEMES.map((s) => (
                <SchemePreviewCard
                  key={s.id}
                  scheme={s}
                  selected={selectedId === s.id}
                  onSelect={() => setSelectedId(s.id)}
                />
              ))}
            </div>
          </Section>

          <Section title="Selected Variant — Enlarged">
            <div className="border-foreground/8 overflow-hidden rounded-xl border">
              <div className="flex flex-col">
                <div className="bg-background-card/50 border-b border-white/5 px-4 py-2">
                  <span className="text-foreground text-sm font-semibold">
                    {selected.name}
                  </span>
                  <span className="text-foreground-alt ml-2 text-xs">
                    {selected.description}
                  </span>
                </div>
                <div style={selected.vars}>
                  <DemoLayout
                    prefix={`enlarged-${selected.id}`}
                    height={260}
                    vars={selected.vars}
                  />
                </div>
              </div>
            </div>
          </Section>

          <Section title="Palette Swatches">
            <SwatchRow scheme={selected} />
          </Section>

          <Section title="Configuration">
            <pre className="bg-background-card/50 text-foreground-alt overflow-x-auto rounded-lg border border-white/5 p-4 font-mono text-xs leading-relaxed">
              {JSON.stringify(
                { id: selected.id, name: selected.name, vars: selected.vars },
                null,
                2,
              )}
            </pre>
          </Section>

          <div className="pb-8" />
        </div>
      </div>
    </div>
  )
}
