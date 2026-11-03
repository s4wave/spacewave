import { useCallback, useMemo, useState } from 'react'
import { OptimizedLayout, Model, TabNode, IJsonModel } from '@aptre/flex-layout'
import { LuArrowLeft, LuFile } from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  utilityState as themeUtilityState,
  radii as themeRadii,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

import { groupMarker } from '../style/relations.stylex.js'

const stylexStyles3 = stylex.create({
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

const stylexStyles2 = stylex.create({
  rule1: {
    backgroundColor: 'oklch(22% 0 0)',
    backgroundImage: 'none',
    backgroundPositionX: '0%',
    backgroundPositionY: '0%',
    backgroundSize: 'auto',
    backgroundRepeat: 'repeat',
    backgroundAttachment: 'scroll',
    backgroundOrigin: 'padding-box',
    backgroundClip: 'border-box',
    borderRadius: themeRadii['--radius-lg'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    position: 'relative',
    height: 'var(--layout-colors-height)',
    overflow: 'hidden',
  },
  rule3: {
    backgroundColor: 'var(--layout-color-swatch)',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
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
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
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
  rule7: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule9: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
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
  rule12: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-window-border'],
  },
  rule13: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule14: {
    containerType: 'inline-size',
    display: 'flex',
    width: '100%',
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background'],
  },
  rule15: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule16: {
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
  rule17: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule18: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-3xl)',
    lineHeight: 'var(--tw-leading, var(--text-3xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule19: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule20: {
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule21: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule22: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule23: {
    overflow: 'hidden',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule24: { display: 'flex', flexDirection: 'column' },
  rule25: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule26: {
    marginLeft: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule27: {
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
  rule28: { paddingBottom: `calc(${themeLayout['--spacing']} * 8)` },
})

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
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <LuFile sx={stylexStyles.rule3} />
        <span sx={stylexStyles.rule4}>{node.getName()}</span>
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
  const applyVars = useColorSchemeVars(vars)
  const renderTab = useCallback(
    (node: TabNode) => <SampleContent node={node} />,
    [],
  )

  return (
    <div ref={applyVars} sx={stylexStyles2.rule1}>
      <div
        sx={stylexStyles2.rule2}
        style={{ '--layout-colors-height': `${height}px` }}
      >
        <OptimizedLayout model={model} renderTab={renderTab} />
      </div>
    </div>
  )
}

const noColorSchemeVars: Record<string, string> = {}

// useColorSchemeVars scopes the debug palette to one preview surface. The
// values are already CSS custom properties; assigning them through the DOM
// keeps the palette data-driven without injecting ordinary inline styles.
function useColorSchemeVars(vars: Record<string, string> = noColorSchemeVars) {
  return useCallback(
    (element: HTMLDivElement | null) => {
      if (!element) return
      for (const [name, value] of Object.entries(vars)) {
        element.style.setProperty(name, value)
      }
    },
    [vars],
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
    <section sx={stylexStyles.rule5}>
      <div>
        <h2 sx={stylexStyles.rule6}>{title}</h2>
        {subtitle && <p sx={stylexStyles.rule7}>{subtitle}</p>}
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
      role="button"
      tabIndex={0}
      data-testid={`scheme-card-${scheme.id}`}
      sx={[
        [stylexStyles3.rule1, groupMarker],
        selected ? stylexStyles3.rule2 : stylexStyles3.rule3,
      ]}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        onSelect()
      }}
    >
      <div sx={stylexStyles.rule8}>
        <div>
          <h3 sx={stylexStyles.rule9}>{scheme.name}</h3>
          <p sx={stylexStyles.rule10}>{scheme.description}</p>
        </div>
        {selected && <span sx={stylexStyles.rule11}>Selected</span>}
      </div>
      <div ref={useColorSchemeVars(scheme.vars)} sx={stylexStyles.rule12}>
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
    <div sx={stylexStyles.rule13}>
      {SWATCH_KEYS.map((s) => {
        const value = scheme.vars[s.key]
        return (
          <div key={s.key} sx={stylexStyles.rule2}>
            <div
              sx={stylexStyles2.rule3}
              style={{ '--layout-color-swatch': value || `var(${s.key})` }}
            />
            <span sx={stylexStyles.rule10}>{s.label}</span>
          </div>
        )
      })}
    </div>
  )
}

// LayoutColorsDebug renders the Monokai Spectrum comparison page.
export function LayoutColorsDebug({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  // Return to the application through the active router.
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  // Retain the chosen scheme while showing its layout preview.
  const [selectedId, setSelectedId] = useState(SCHEMES[0].id)

  // Resolve a stored choice against the schemes currently available.
  const selected = useMemo(
    () => SCHEMES.find((s) => s.id === selectedId) ?? SCHEMES[0],
    [selectedId],
  )

  // Render the current scheme and its live layout preview.
  return (
    <div sx={[stylexStyles.rule14, sx]}>
      <div sx={stylexStyles.rule15}>
        <button type="button" onClick={goBack} sx={stylexStyles.rule16}>
          <LuArrowLeft sx={stylexStyles.rule17} />
          Back to home
        </button>

        <h1 sx={stylexStyles.rule18}>Monokai Spectrum Lab</h1>
        <p sx={stylexStyles.rule19}>
          Compare the current warm theme against Monokai Spectrum-inspired
          variants. Each isolates a different axis: backgrounds, brand color,
          text tint, or the full combination.
        </p>
        <p sx={stylexStyles.rule20}>
          Click a variant to select it. The enlarged preview and swatches update
          to show the selected scheme.
        </p>

        <div sx={stylexStyles.rule21}>
          <Section title="Monokai Variants">
            <div sx={stylexStyles.rule22}>
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
            <div sx={stylexStyles.rule23}>
              <div sx={stylexStyles.rule24}>
                <div sx={stylexStyles.rule25}>
                  <span sx={stylexStyles.rule9}>{selected.name}</span>
                  <span sx={stylexStyles.rule26}>{selected.description}</span>
                </div>
                <div ref={useColorSchemeVars(selected.vars)}>
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
            <pre sx={stylexStyles.rule27}>
              {JSON.stringify(
                { id: selected.id, name: selected.name, vars: selected.vars },
                null,
                2,
              )}
            </pre>
          </Section>

          <div sx={stylexStyles.rule28} />
        </div>
      </div>
    </div>
  )
}
