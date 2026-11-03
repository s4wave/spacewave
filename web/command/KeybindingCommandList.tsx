import { useEffect, useRef } from 'react'

import {
  LuChevronDown,
  LuSearch,
  LuSettings2,
} from '@s4wave/web/images/icons.js'

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

const stylexStyles3 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: `calc(${themeLayout['--spacing']} * 11)`,
    width: '100%',
  },
  rule2: {
    borderColor: '#0000',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: `calc(${themeLayout['--spacing']} * 11)`,
    width: '100%',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
})

const childSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card-alt']} 40%, transparent)`,
    flexDirection: 'column',
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    minHeight: 0,
  },
  rule2: { display: { default: 'none', '@media (min-width: 40rem)': 'flex' } },
  rule3: { display: 'flex' },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule5: {
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
  },
  rule6: {
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule7: {
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    backgroundColor: themeColors['--color-background-tertiary'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-foreground'],
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
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 3.5)`,
    },
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
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    listStyleType: 'none',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule10: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule11: { paddingTop: `calc(${themeLayout['--spacing']} * 3)` },
  rule12: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    minHeight: `calc(${themeLayout['--spacing']} * 11)`,
  },
  rule13: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule14: {
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
  rule15: {
    '--tw-outline-style': 'none',
    flex: 1,
    backgroundColor: '#0000',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    minWidth: 0,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule16: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    flex: 1,
    overflow: 'auto',
    minHeight: 0,
  },
  rule17: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 8)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
  rule18: {
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand-highlight'],
      },
    },
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    width: '100%',
  },
  rule19: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule20: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: themeColors['--color-background-tertiary'],
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    fontSize: '.65rem',
    lineHeight: 1.5,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    maxWidth: `calc(${themeLayout['--spacing']} * 28)`,
  },
})

const scopes: KeybindingEditorScope[] = ['local', 'account', 'space']

export function KeybindingCommandList({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
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
        [
          stylexStyles.rule1,
          selectedCommandId ? stylexStyles.rule2 : stylexStyles.rule3,
        ],
        sx,
      ]}
    >
      <div sx={stylexStyles2.rule1}>
        <div sx={childSpacing.s3}>
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
        <details sx={[groupMarker, childSpacing.s3]}>
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
            sx={
              row.commandId === selectedRow?.commandId
                ? stylexStyles3.rule1
                : stylexStyles3.rule2
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
    rotate: {
      default: null,
      [stylex.when.ancestor(':is([open],:popover-open,:open)', groupMarker)]:
        '180deg',
    },
    transitionDuration: 'var(--tw-duration,var(--default-transition-duration))',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction:
      'var(--tw-ease,var(--default-transition-timing-function))',
    height: 12,
    marginLeft: 'auto',
    width: 12,
  },
})
