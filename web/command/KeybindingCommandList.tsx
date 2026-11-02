import { useEffect, useRef } from 'react'

import {
  LuChevronDown,
  LuSearch,
  LuSettings2,
} from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

import { formatKeybindingHint } from './CommandPalette.js'
import { KeybindingDiscoverySettings } from './KeybindingDiscoverySettings.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'
import { scopeLabels, type KeybindingEditorScope } from './component.js'

import * as stylex from '@stylexjs/stylex'
import { groupMarker } from '../style/relations.stylex.js'
import {
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    minHeight: 0,
    flexDirection: 'column',
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card-alt']} 40%, transparent)`,
  },
  rule2: { display: { default: 'none', '@media (min-width: 40rem)': 'flex' } },
  rule3: { display: 'flex' },
  rule4: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    display: 'grid',
    gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule6: {
    minHeight: `calc(${themeLayout['--spacing']} * 8)`,
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule7: {
    backgroundColor: themeColors['--color-background-tertiary'],
    color: themeColors['--color-foreground'],
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule8: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule9: {
    display: 'flex',
    cursor: 'pointer',
    listStyleType: 'none',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    paddingBlock: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 3.5)`,
    },
  },
  rule10: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule11: { paddingTop: `calc(${themeLayout['--spacing']} * 3)` },
  rule12: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 11)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule13: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-brand'],
  },
  rule14: {
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
  rule15: {
    minWidth: 0,
    flex: 1,
    backgroundColor: '#0000',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule16: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule17: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule18: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'block',
    width: '100%',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand-highlight'],
      },
    },
  },
  rule19: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule20: {
    maxWidth: `calc(${themeLayout['--spacing']} * 28)`,
    flexShrink: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: themeColors['--color-background-tertiary'],
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '.65rem',
    lineHeight: 1.5,
    color: themeColors['--color-foreground-alt'],
  },
})

const scopes: KeybindingEditorScope[] = ['local', 'account', 'space']

export function KeybindingCommandList() {
  const {
    accountOverridesAvailable,
    spaceOverridesAvailable,
    rows,
    query,
    selectedCommandId,
    selectedRow,
    selectedScope,
    setQuery,
    setSelectedCommandId,
    setSelectedScope,
  } = useKeybindingEditorContext()
  const searchRef = useRef<HTMLInputElement>(null)

  // Focus the search field when the editor opens.
  useEffect(() => {
    searchRef.current?.focus()
  }, [])

  return (
    <aside
      sx={[
        stylexStyles.rule1,
        selectedCommandId ? stylexStyles.rule2 : stylexStyles.rule3,
      ]}
    >
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'border-foreground/8 space-y-3 border-b p-3'
        }
      >
        <div>
          <div sx={stylexStyles.rule4}>Save changes to</div>
          <div sx={stylexStyles.rule5}>
            {scopes.map((scope) => {
              const available =
                scope === 'local' ||
                (scope === 'account'
                  ? accountOverridesAvailable
                  : spaceOverridesAvailable)
              return (
                <button
                  key={scope}
                  type="button"
                  sx={[
                    stylexStyles.rule6,
                    scope === selectedScope
                      ? stylexStyles.rule7
                      : stylexStyles.rule8,
                  ]}
                  disabled={!available}
                  title={
                    available
                      ? undefined
                      : scope === 'account'
                        ? 'Sign in to save shortcuts to your account'
                        : 'Open a Space to save shortcuts there'
                  }
                  onClick={() => setSelectedScope(scope)}
                >
                  {scope === 'local' ? 'This device' : scopeLabels[scope]}
                </button>
              )
            })}
          </div>
        </div>
        <details sx={groupMarker}>
          <summary sx={stylexStyles.rule9}>
            <LuSettings2 sx={stylexStyles.rule10} />
            Discovery settings
            <LuChevronDown sx={disclosureIconStyles.icon} />
          </summary>
          <div sx={stylexStyles.rule11}>
            <KeybindingDiscoverySettings />
          </div>
        </details>
      </div>
      <label sx={stylexStyles.rule12}>
        <LuSearch sx={stylexStyles.rule13} />
        <span sx={stylexStyles.rule14}>Search commands or shortcuts</span>
        <input
          ref={searchRef}
          sx={stylexStyles.rule15}
          placeholder="Search commands or shortcuts…"
          value={query}
          onChange={(event) => setQuery(event.currentTarget.value)}
        />
      </label>
      <div sx={stylexStyles.rule16}>
        {rows.length === 0 && (
          <div sx={stylexStyles.rule17}>
            {query
              ? `No shortcuts match “${query}”.`
              : 'No commands are available.'}
            {query && (
              <button
                type="button"
                sx={stylexStyles.rule18}
                onClick={() => setQuery('')}
              >
                Clear search
              </button>
            )}
          </div>
        )}
        {rows.map((row) => (
          <button
            key={row.commandId}
            type="button"
            className={
              /* TODO(stylex): conditional property composition: backgroundColor */ cn(
                'hover:bg-foreground/5 flex min-h-11 w-full items-center gap-3 rounded-md border border-transparent px-2.5 py-2 text-left transition-colors',
                row.commandId === selectedRow?.commandId &&
                  'border-brand/20 bg-brand/10 text-foreground',
              )
            }
            onClick={() => setSelectedCommandId(row.commandId)}
          >
            <span sx={stylexStyles.rule19}>{row.label}</span>
            {row.displayBindings.length > 0 && (
              <kbd sx={stylexStyles.rule20}>
                {formatKeybindingHint(row.displayBindings)}
              </kbd>
            )}
          </button>
        ))}
      </div>
    </aside>
  )
}

const disclosureIconStyles = stylex.create({
  icon: {
    marginLeft: 'auto',
    width: 12,
    height: 12,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction:
      'var(--tw-ease,var(--default-transition-timing-function))',
    transitionDuration: 'var(--tw-duration,var(--default-transition-duration))',
    rotate: {
      default: null,
      [stylex.when.ancestor(':is([open],:popover-open,:open)', groupMarker)]:
        '180deg',
    },
  },
})
