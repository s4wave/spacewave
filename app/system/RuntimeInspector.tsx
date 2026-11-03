import { useDeferredValue, useMemo, useState, type ReactNode } from 'react'
import { LuChevronRight, LuSearch } from '@s4wave/web/images/icons.js'

import type { ControllerInfo, StoreInfo } from '@s4wave/sdk/status/status.pb.js'
import { cn } from '@s4wave/web/style/utils.js'

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

const stylexStyles3 = stylex.create({
  rule1: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: themeLayout['--spacing'],
    paddingBottom: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 10)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    display: 'flex',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
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
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule8: {
    display: 'flex',
    minWidth: `calc(${themeLayout['--spacing']} * 48)`,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      ':focus-within': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule9: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
  rule10: {
    minWidth: 0,
    flex: 1,
    backgroundColor: '#0000',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    },
  },
  rule11: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule12: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-foreground'],
  },
  rule13: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule14: {
    display: { default: 'none', '@container (min-width: 42rem)': 'block' },
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule15: {
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule16: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule17: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 3%, transparent)`,
      },
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-foreground']} 4%, transparent)`,
    },
  },
  rule18: { paddingBottom: themeLayout['--spacing'] },
  rule19: {
    paddingLeft: 'calc(1rem + var(--tree-depth) * 1.25rem)',
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule20: {
    width: `calc(${themeLayout['--spacing']} * 12)`,
    flexShrink: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule21: {
    maxWidth: `calc(${themeLayout['--spacing']} * 80)`,
    flexShrink: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule22: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule23: {
    display: 'flex',
    alignItems: 'baseline',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 2%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule24: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule25: {
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule26: {
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    color: themeColors['--color-warning'],
  },
  rule27: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: themeLayout['--spacing'],
    paddingBottom: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 10)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 48rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule28: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule29: {
    display: { default: 'none', '@container (min-width: 36rem)': 'block' },
    height: themeLayout['--spacing'],
    width: `calc(${themeLayout['--spacing']} * 24)`,
    flexShrink: 0,
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule30: {
    width: 'var(--progress-width)',
    display: 'block',
    height: '100%',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule31: {
    width: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    textAlign: 'right',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule32: { wordBreak: 'break-all' },
  rule33: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule34: {
    display: 'flex',
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule35: {
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule36: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule37: {
    maxWidth: `calc(${themeLayout['--spacing']} * 64)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-foreground'],
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule38: {
    marginLeft: 'auto',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
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
            {pluginHosts.map((group) => (
              <PluginHostGroup key={group.spaceId} group={group} />
            ))}
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
            {visibleControllers.map((group) => (
              <ControllerRow key={group.key} group={group} />
            ))}
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
          {directiveTree && (
            <li>
              <DirectiveTreeView
                tree={directiveTree}
                query={query}
                configKey={rootKey}
              />
            </li>
          )}
        </Listing>
      ) : (
        <Listing
          loading={!directives}
          empty={query ? 'No directives match.' : 'No directives are active.'}
          count={visibleDirectives.length}
        >
          {visibleDirectives.map((group) => (
            <DirectiveRow
              key={group.name}
              group={group}
              largest={directives?.[0]?.idents.length ?? 1}
            />
          ))}
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
function ControllerRow({ group }: { group: ControllerGroup }) {
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
    return <li sx={stylexStyles2.rule16}>{label}</li>
  }
  return (
    <li>
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
  children: ReactNode
}) {
  if (loading) {
    return <p sx={stylexStyles2.rule22}>Reading the runtime…</p>
  }
  if (!count) {
    return <p sx={stylexStyles2.rule22}>{empty}</p>
  }
  return (
    <ul
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: divide-y */ 'divide-foreground/6 divide-y'
      }
    >
      {children}
    </ul>
  )
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
function PluginHostGroup({ group }: { group: PluginHost }) {
  const running = group.plugins.filter(
    (plugin) => plugin.state === 'running',
  ).length

  return (
    <li>
      <h4 sx={stylexStyles2.rule23}>
        <span sx={stylexStyles2.rule24}>{group.host}</span>
        <span sx={stylexStyles2.rule25}>
          {formatCount(running)}/{formatCount(group.plugins.length)} running
        </span>
      </h4>
      <ul
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: divide-y */ 'divide-foreground/6 divide-y'
        }
      >
        {group.plugins.map((plugin) => (
          <PluginRow key={plugin.key} plugin={plugin} />
        ))}
      </ul>
    </li>
  )
}

// pluginStateClass colors the plugin lifecycle: running is healthy and
// requested is still loading.
const pluginStateClass: Record<string, string> = {
  running: 'bg-success/10 text-success',
  requested: 'bg-brand/10 text-brand',
}

// PluginRow is one plugin host instance. It expands to the manifest recovery
// and native package facts retained for the plugin.
function PluginRow({ plugin }: { plugin: PluginView }) {
  const [open, setOpen] = useState(false)
  const manifest = plugin.manifest
  const nativePackage = plugin.nativePackage
  const flagged =
    !!manifest?.quarantinedCandidateCount || !!nativePackage?.lastError

  return (
    <li>
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
          className={
            /* TODO(stylex): fallback class expression */ cn(
              'rounded px-1.5 py-0.5',
              pluginStateClass[plugin.state] ?? 'bg-warning/10 text-warning',
            )
          }
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
  group,
  largest,
}: {
  group: DirectiveGroup
  largest: number
}) {
  const [open, setOpen] = useState(false)

  return (
    <li>
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
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
