import { utilityState as listUtilityState } from '@s4wave/web/style/tokens.stylex.js'

import { useDeferredValue, useMemo, useState, type ReactNode } from 'react'
import { LuChevronRight, LuSearch } from '@s4wave/web/images/icons.js'

import type { ControllerInfo, StoreInfo } from '@s4wave/sdk/status/status.pb.js'

import { DirectiveTreeView } from './DirectiveTreeView.js'
import type { DirectiveTree } from './directiveTree.js'
import { Facts } from './Facts.js'
import { formatCount } from './format.js'
import type { DirectiveGroup, PluginView } from './useSystemModel.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-success']} 10%, transparent)`,
    color: themeColors['--color-success'],
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    paddingBottom: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 10)`,
    paddingTop: themeLayout['--spacing'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
    minHeight: 0,
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexWrap: 'wrap',
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
  },
  rule4: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: '.25rem',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: { default: null, ':focus-visible': 'none' },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule5: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule6: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule7: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule8: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      ':focus-within': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
    minWidth: `calc(${themeLayout['--spacing']} * 48)`,
  },
  rule9: {
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
  rule10: {
    '--tw-outline-style': 'none',
    flex: 1,
    backgroundColor: '#0000',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    minWidth: 0,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    },
  },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    overflow: 'auto',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    minHeight: 0,
  },
  rule12: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule13: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule14: {
    flex: 1,
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    display: { default: 'none', '@container (min-width: 42rem)': 'block' },
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule15: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule16: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule17: {
    '--tw-outline-style': 'none',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-foreground']} 4%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 3%, transparent)`,
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    textAlign: 'left',
    width: '100%',
  },
  rule18: { paddingBottom: themeLayout['--spacing'] },
  rule19: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    paddingLeft: 'calc(1rem + var(--tree-depth) * 1.25rem)',
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule20: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    width: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule21: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    maxWidth: `calc(${themeLayout['--spacing']} * 80)`,
  },
  rule22: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule23: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'baseline',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 2%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule24: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule25: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule26: {
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    color: themeColors['--color-warning'],
  },
  rule27: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 48rem)': 'repeat(2, minmax(0, 1fr))',
    },
    paddingBottom: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 10)`,
    paddingTop: themeLayout['--spacing'],
  },
  rule28: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule29: {
    borderRadius: '3.40282e38px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    display: { default: 'none', '@container (min-width: 36rem)': 'block' },
    flexShrink: 0,
    height: themeLayout['--spacing'],
    width: `calc(${themeLayout['--spacing']} * 24)`,
  },
  rule30: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'block',
    height: '100%',
    width: 'var(--progress-width)',
  },
  rule31: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    textAlign: 'right',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule32: { wordBreak: 'break-all' },
  rule33: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule34: {
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    display: 'flex',
  },
  rule35: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    outlineStyle: { default: null, ':focus-visible': 'none' },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule36: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    display: 'flex',
  },
  rule37: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    outlineStyle: { default: null, ':focus-visible': 'none' },
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    maxWidth: `calc(${themeLayout['--spacing']} * 64)`,
  },
  rule38: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    marginLeft: 'auto',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    '--tw-duration': '.15s',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    transitionDuration: '.15s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: { rotate: '90deg' },
  rule4: { visibility: 'hidden' },
})

// RuntimeView selects which runtime listing the inspector shows.
export type RuntimeView = 'plugins' | 'controllers' | 'directives'

// DirectiveLayout selects how the directives listing is arranged.
// "tree" draws directives under their origins.
// "type" groups them by directive type.
type DirectiveLayout = 'tree' | 'type'

// RuntimeInspector lists plugin instances grouped by the host that runs them,
// controllers on the bus, and active directives under their origins or grouped
// by type, with one filter across all three.
export function RuntimeInspector({
  plugins,
  controllers,
  directives,
  directiveTree,
  directiveCount,
  view,
  onViewChange,
}: {
  plugins: PluginView[] | null
  controllers: ControllerInfo[] | null
  directives: DirectiveGroup[] | null
  directiveTree: DirectiveTree | null
  directiveCount: number
  view: RuntimeView
  onViewChange: (view: RuntimeView) => void
}) {
  const [filter, setFilter] = useState('')
  const query = useDeferredValue(filter.trim().toLowerCase())

  // Filter each listing by the shared query.
  const visiblePlugins = useMemo(
    () =>
      (plugins ?? []).filter((plugin) =>
        matches(
          query,
          plugin.id,
          plugin.instanceKey,
          plugin.state,
          plugin.host,
        ),
      ),
    [plugins, query],
  )
  const pluginHosts = useMemo(
    () => groupByHost(visiblePlugins),
    [visiblePlugins],
  )
  const visibleControllers = useMemo(
    () =>
      groupControllers(
        (controllers ?? []).filter((controller) =>
          matches(query, controller.id, controller.description),
        ),
      ),
    [controllers, query],
  )
  const tabs: { id: RuntimeView; label: string; count: number | null }[] = [
    { id: 'plugins', label: 'Plugins', count: plugins?.length ?? null },
    {
      id: 'controllers',
      label: 'Controllers',
      count: controllers?.length ?? null,
    },
    {
      id: 'directives',
      label: 'Directives',
      count: directives ? directiveCount : null,
    },
  ]

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div
          role="tablist"
          aria-label="Runtime listing"
          sx={stylexStyles2.rule3}
        >
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={view === tab.id}
              onClick={() => onViewChange(tab.id)}
              sx={[
                stylexStyles2.rule4,
                view === tab.id ? stylexStyles2.rule5 : stylexStyles2.rule6,
              ]}
            >
              {tab.label}
              <span sx={stylexStyles2.rule7}>
                {tab.count == null ? '…' : formatCount(tab.count)}
              </span>
            </button>
          ))}
        </div>

        <label sx={stylexStyles2.rule8}>
          <LuSearch sx={stylexStyles.rule1} aria-hidden="true" />
          <span sx={stylexStyles2.rule9}>Filter runtime listing</span>
          <input
            type="search"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            placeholder="Filter by id, name, or state"
            sx={stylexStyles2.rule10}
          />
        </label>
      </div>

      <div role="tabpanel" sx={stylexStyles2.rule11}>
        {view === 'plugins' && (
          <Listing
            loading={!plugins}
            empty={
              query
                ? 'No plugins match.'
                : 'No plugins are running. Open a Space to start its plugins.'
            }
            count={visiblePlugins.length}
          >
            {(childSx) => (
              <>
                {pluginHosts.map((group) => (
                  <PluginHostGroup
                    sx={childSx}
                    key={group.spaceId}
                    group={group}
                  />
                ))}
              </>
            )}
          </Listing>
        )}

        {view === 'controllers' && (
          <Listing
            loading={!controllers}
            empty={
              query ? 'No controllers match.' : 'No controllers are running.'
            }
            count={visibleControllers.length}
          >
            {(childSx) => (
              <>
                {visibleControllers.map((group) => (
                  <ControllerRow sx={childSx} key={group.key} group={group} />
                ))}
              </>
            )}
          </Listing>
        )}

        {view === 'directives' && (
          <DirectivesPanel
            directives={directives}
            directiveTree={directiveTree}
            query={query}
          />
        )}
      </div>
    </div>
  )
}

// DirectivesPanel lists the active directives under their origins or grouped by type.
function DirectivesPanel({
  directives,
  directiveTree,
  query,
}: {
  directives: DirectiveGroup[] | null
  directiveTree: DirectiveTree | null
  query: string
}) {
  const [layout, setLayout] = useState<DirectiveLayout>('tree')
  const [configKey, setConfigKey] = useState('')
  const visibleDirectives = useMemo(
    () =>
      (directives ?? []).filter(
        (group) =>
          matches(query, group.name) ||
          group.idents.some((ident) => matches(query, ident)),
      ),
    [directives, query],
  )
  const rootKey = directiveTree?.configKeys.includes(configKey) ? configKey : ''

  return (
    <>
      {directiveTree && (
        <DirectiveToolbar
          layout={layout}
          onLayoutChange={setLayout}
          configKeys={directiveTree.configKeys}
          configKey={rootKey}
          onConfigKeyChange={setConfigKey}
          unlinked={directiveTree.unlinked.length}
        />
      )}
      {layout === 'tree' ? (
        <Listing
          loading={!directiveTree}
          empty="No directives are active."
          count={directiveTree?.nodes.size ?? 0}
        >
          {(childSx) => (
            <>
              {directiveTree && (
                <li sx={childSx}>
                  <DirectiveTreeView
                    tree={directiveTree}
                    query={query}
                    configKey={rootKey}
                  />
                </li>
              )}
            </>
          )}
        </Listing>
      ) : (
        <Listing
          loading={!directives}
          empty={query ? 'No directives match.' : 'No directives are active.'}
          count={visibleDirectives.length}
        >
          {(childSx) => (
            <>
              {visibleDirectives.map((group) => (
                <DirectiveRow
                  sx={childSx}
                  key={group.name}
                  group={group}
                  largest={directives?.[0]?.idents.length ?? 1}
                />
              ))}
            </>
          )}
        </Listing>
      )}
    </>
  )
}

// ControllerGroup is one or more running controllers with the same id,
// version, and description.
interface ControllerGroup {
  key: string
  controller: ControllerInfo
  count: number
}

// groupControllers folds identical controllers, such as the bus bridge each
// Space runtime adds, into one row with a count, keeping first-seen order. A
// controller holding a store keeps its own row.
function groupControllers(controllers: ControllerInfo[]): ControllerGroup[] {
  const groups = new Map<string, ControllerGroup>()
  for (const [index, controller] of controllers.entries()) {
    const key =
      `${controller.id}@${controller.version}:${controller.description}` +
      (controller.store ? `#${index}` : '')
    const group = groups.get(key)
    if (group) {
      group.count++
    } else {
      groups.set(key, { key, controller, count: 1 })
    }
  }
  return [...groups.values()]
}

// ControllerRow is one controller, or several identical ones. A controller
// holding a block store it built itself opens into that store's parts.
function ControllerRow({
  sx,
  group,
}: { group: ControllerGroup } & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const [open, setOpen] = useState(false)
  const { controller, count } = group
  const store = controller.store
  const label = (
    <>
      <LuChevronRight
        sx={[
          stylexStyles.rule2,
          !!open && stylexStyles.rule3,
          !store && stylexStyles.rule4,
        ]}
        aria-hidden="true"
      />
      <span sx={stylexStyles2.rule12}>
        {controller.id}
        {count > 1 && (
          <span sx={stylexStyles2.rule13}> ×{formatCount(count)}</span>
        )}
      </span>
      {controller.description && (
        <span sx={stylexStyles2.rule14}>{controller.description}</span>
      )}
      <span sx={stylexStyles2.rule15}>{controller.version}</span>
    </>
  )

  if (!store) {
    return <li sx={[stylexStyles2.rule16, sx]}>{label}</li>
  }
  return (
    <li sx={sx}>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        sx={stylexStyles2.rule17}
      >
        {label}
      </button>
      {open && (
        <ul sx={stylexStyles2.rule18}>
          <StoreRow store={store} depth={1} />
        </ul>
      )}
    </li>
  )
}

// StoreRow is one block store and, below it, the stores it is built from.
function StoreRow({ store, depth }: { store: StoreInfo; depth: number }) {
  return (
    <li>
      <div style={{ '--tree-depth': depth }} sx={stylexStyles2.rule19}>
        <span sx={stylexStyles2.rule20}>{store.label || 'store'}</span>
        <span sx={stylexStyles2.rule12}>{store.type || 'unknown'}</span>
        {store.id && <span sx={stylexStyles2.rule21}>{store.id}</span>}
      </div>
      {!!store.parts?.length && (
        <ul>
          {store.parts.map((part) => (
            <StoreRow
              key={`${part.label}/${part.type}/${part.id}`}
              store={part}
              depth={depth + 1}
            />
          ))}
        </ul>
      )}
    </li>
  )
}

// matches reports whether any field contains the lowercase query.
function matches(query: string, ...fields: (string | undefined)[]): boolean {
  return !query || fields.some((field) => field?.toLowerCase().includes(query))
}

// Listing renders a list body with its loading and empty states.
function Listing({
  loading,
  empty,
  count,
  children,
}: {
  loading: boolean
  empty: string
  count: number
  children: (childSx: stylex.StyleXStyles) => ReactNode
}) {
  if (loading) {
    return <p sx={stylexStyles2.rule22}>Reading the runtime…</p>
  }
  if (!count) {
    return <p sx={stylexStyles2.rule22}>{empty}</p>
  }
  return <ul sx={false}>{children?.(slotChildren.child)}</ul>
}

// PluginHost is the plugins one host runs: the root plugin host or a Space
// runtime.
interface PluginHost {
  spaceId: string
  host: string
  plugins: PluginView[]
}

// groupByHost splits host-ordered plugins into one group per host.
function groupByHost(plugins: PluginView[]): PluginHost[] {
  const groups: PluginHost[] = []
  for (const plugin of plugins) {
    const last = groups.at(-1)
    if (last?.spaceId === plugin.spaceId) {
      last.plugins.push(plugin)
    } else {
      groups.push({
        spaceId: plugin.spaceId,
        host: plugin.host,
        plugins: [plugin],
      })
    }
  }
  return groups
}

// PluginHostGroup is one host's heading followed by its plugin rows.
function PluginHostGroup({
  sx,
  group,
}: { group: PluginHost } & { sx?: stylex.StyleXStyles; className?: never }) {
  const running = group.plugins.filter(
    (plugin) => plugin.state === 'running',
  ).length

  return (
    <li sx={sx}>
      <h4 sx={stylexStyles2.rule23}>
        <span sx={stylexStyles2.rule24}>{group.host}</span>
        <span sx={stylexStyles2.rule25}>
          {formatCount(running)}/{formatCount(group.plugins.length)} running
        </span>
      </h4>
      <ul>
        {group.plugins.map((plugin) => (
          <PluginRow sx={nativeDividers.t6} key={plugin.key} plugin={plugin} />
        ))}
      </ul>
    </li>
  )
}

// pluginStateStyles colors the plugin lifecycle: running is healthy and
// requested is still loading.
const pluginStateStyles: Record<string, stylex.StyleXStyles> = {
  running: stylexStyles4.rule1,
  requested: stylexStyles4.rule2,
}

// PluginRow is one plugin host instance. It expands to the manifest recovery
// and native package facts retained for the plugin.
function PluginRow({
  sx,
  plugin,
}: { plugin: PluginView } & { sx?: stylex.StyleXStyles; className?: never }) {
  const [open, setOpen] = useState(false)
  const manifest = plugin.manifest
  const nativePackage = plugin.nativePackage
  const flagged =
    !!manifest?.quarantinedCandidateCount || !!nativePackage?.lastError

  return (
    <li sx={sx}>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        sx={stylexStyles2.rule17}
      >
        <LuChevronRight
          sx={[stylexStyles.rule2, !!open && stylexStyles.rule3]}
          aria-hidden="true"
        />
        <span sx={stylexStyles2.rule12}>
          {plugin.id || 'unknown'}
          {plugin.instanceKey && (
            <span sx={stylexStyles2.rule13}> {plugin.instanceKey}</span>
          )}
        </span>
        {flagged && <span sx={stylexStyles2.rule26}>Needs attention</span>}
        <span
          sx={[
            [stylexStyles5.rule1],
            [pluginStateStyles[plugin.state] ?? pluginFallbackStyles.pending],
          ]}
        >
          {plugin.state}
        </span>
      </button>

      {open && (
        <div sx={stylexStyles2.rule27}>
          <div>
            <h4 sx={stylexStyles2.rule28}>Manifest</h4>
            <Facts
              empty="No manifest recovery facts retained."
              facts={[
                {
                  label: 'Execute manifest',
                  value: manifest?.executeManifestRef,
                  mono: true,
                },
                {
                  label: 'Download manifest',
                  value: manifest?.downloadManifestRef,
                  mono: true,
                },
                {
                  label: 'Skipped',
                  value: countFact(
                    manifest?.skippedCandidateCount,
                    manifest?.skippedCandidateSummary,
                  ),
                },
                {
                  label: 'Ignored',
                  value: countFact(
                    manifest?.ignoredCandidateCount,
                    manifest?.ignoredCandidateSummary,
                  ),
                },
                {
                  label: 'Quarantined',
                  value: countFact(
                    manifest?.quarantinedCandidateCount,
                    manifest?.quarantinedCandidateSummary,
                  ),
                  tone: 'warning',
                },
              ]}
            />
          </div>
          <div>
            <h4 sx={stylexStyles2.rule28}>Native package</h4>
            <Facts
              empty="This plugin has no native package."
              facts={
                nativePackage
                  ? [
                      {
                        label: 'Materialized',
                        value: nativePackage.materialized ?? false,
                      },
                      {
                        label: 'Invalidated',
                        value: nativePackage.invalidated ?? false,
                      },
                      {
                        label: 'Last action',
                        value: nativePackage.lastAction,
                      },
                      {
                        label: 'Directory',
                        value: nativePackage.distDir,
                        mono: true,
                      },
                      {
                        label: 'Updated',
                        value: nativePackage.updatedAt,
                        mono: true,
                      },
                      {
                        label: 'Last error',
                        value: nativePackage.lastError,
                        tone: 'error',
                      },
                    ]
                  : []
              }
            />
          </div>
        </div>
      )}
    </li>
  )
}

// countFact formats a candidate count with its summary, or null when zero.
function countFact(count?: number, summary?: string): string | null {
  if (!count) {
    return null
  }
  return summary ? `${count}: ${summary}` : String(count)
}

// DirectiveRow is one directive type with a bar scaled to the largest group.
// It expands to the identity of every active instance.
function DirectiveRow({
  sx,
  group,
  largest,
}: {
  group: DirectiveGroup
  largest: number
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const [open, setOpen] = useState(false)

  return (
    <li sx={sx}>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        sx={stylexStyles2.rule17}
      >
        <LuChevronRight
          sx={[stylexStyles.rule2, !!open && stylexStyles.rule3]}
          aria-hidden="true"
        />
        <span sx={stylexStyles2.rule12}>{group.name}</span>
        <span sx={stylexStyles2.rule29} aria-hidden="true">
          <span
            sx={stylexStyles2.rule30}
            style={{
              '--progress-width': `${(group.idents.length / largest) * 100}%`,
            }}
          />
        </span>
        <span sx={stylexStyles2.rule31}>
          {formatCount(group.idents.length)}
        </span>
      </button>

      {open && (
        <ul sx={stylexStyles3.rule1}>
          {group.idents.map((ident, index) => (
            <li key={index} sx={[nativeChildSpacing.s1, stylexStyles2.rule32]}>
              {ident || '(no identity)'}
            </li>
          ))}
        </ul>
      )}
    </li>
  )
}

// DirectiveToolbar switches the directives layout and, in the tree, roots it
// at one ConfigSet key.
function DirectiveToolbar({
  layout,
  onLayoutChange,
  configKeys,
  configKey,
  onConfigKeyChange,
  unlinked,
}: {
  layout: DirectiveLayout
  onLayoutChange: (layout: DirectiveLayout) => void
  configKeys: string[]
  configKey: string
  onConfigKeyChange: (configKey: string) => void
  unlinked: number
}) {
  const layouts: { id: DirectiveLayout; label: string }[] = [
    { id: 'tree', label: 'By origin' },
    { id: 'type', label: 'By type' },
  ]

  return (
    <div sx={stylexStyles2.rule33}>
      <div role="group" aria-label="Directive layout" sx={stylexStyles2.rule34}>
        {layouts.map((option) => (
          <button
            key={option.id}
            type="button"
            aria-pressed={layout === option.id}
            onClick={() => onLayoutChange(option.id)}
            sx={[
              stylexStyles2.rule35,
              layout === option.id ? stylexStyles2.rule5 : stylexStyles2.rule6,
            ]}
          >
            {option.label}
          </button>
        ))}
      </div>
      {layout === 'tree' && configKeys.length > 0 && (
        <label sx={stylexStyles2.rule36}>
          Config key
          <select
            value={configKey}
            onChange={(event) => onConfigKeyChange(event.target.value)}
            sx={stylexStyles2.rule37}
          >
            <option value="">All</option>
            {configKeys.map((key) => (
              <option key={key} value={key}>
                {key}
              </option>
            ))}
          </select>
        </label>
      )}
      {layout === 'tree' && (
        <span sx={stylexStyles2.rule38}>
          {unlinked
            ? `${formatCount(unlinked)} without an origin`
            : 'Every directive has an origin'}
        </span>
      )}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
  },
})

// pluginFallbackStyles retains the warning tone of an unknown lifecycle state.
const pluginFallbackStyles = stylex.create({
  pending: {
    backgroundColor:
      'color-mix(in oklab, var(--color-warning) 10%, transparent)',
    color: 'var(--color-warning)',
  },
})

// nativeDividers applies the list border only to its nonfinal physical rows.
const nativeDividers = stylex.create({
  t6: {
    '--tw-divide-y-reverse': { default: null, ':not(:last-child)': 0 },
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 6%, transparent)',
    },
    borderBottomStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-y-reverse)))',
    },
    borderTopStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderTopWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-y-reverse))',
    },
  },
})

// slotChildren keeps the optional action in the original physical child sequence.
const slotChildren = stylex.create({
  child: {
    '--tw-divide-y-reverse': { default: null, ':not(:last-child)': 0 },
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 6%, transparent)',
    },
    borderBottomStyle: {
      default: null,
      ':not(:last-child)': listUtilityState['--tw-border-style'],
    },
    borderBottomWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-y-reverse)))',
    },
    borderTopStyle: {
      default: null,
      ':not(:last-child)': listUtilityState['--tw-border-style'],
    },
    borderTopWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-y-reverse))',
    },
  },
})
