import { LuPlus, LuRotateCcw, LuTrash2 } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'

import { formatKeybinding } from './CommandPalette.js'
import { focusContextLabel } from './KeybindingResolver.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'
import {
  bindingDisplay,
  canLayerOverrideBinding,
} from './keybinding-editor-helpers.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const childSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: 'dashed',
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card-alt']} 30%, transparent)`,
    display: 'flex',
    justifyContent: 'space-between',
    minHeight: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule5: { minWidth: 0 },
  rule6: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-brand'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule7: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    textAlign: 'center',
  },
  rule9: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    color: themeColors['--color-brand'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    minHeight: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule11: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule12: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
  rule13: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
  },
  rule14: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-brand'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    marginTop: themeLayout['--spacing'],
  },
  rule16: {
    color: themeColors['--color-warning'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule17: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule18: {
    color: themeColors['--color-warning'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule19: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export function KeybindingBindingsSection({
  sx,
}: { sx?: stylex.StyleXStyles } = {}) {
  const {
    cancelCapture,
    capture,
    captureError,
    clearSelectedBindings,
    pendingBinding,
    pendingConflict,
    pendingConflictReplaceable,
    pendingReplace,
    removeBinding,
    replacePendingConflict,
    resetSelectedCommand,
    savePendingBinding,
    selectedBindings,
    selectedLayerEditable,
    selectedRow,
    selectedScope,
    startCapture,
  } = useKeybindingEditorContext()

  return (
    <div sx={sx}>
      <div sx={childSpacing.s4}>
        <div sx={stylexStyles.rule1}>
          <h3 sx={stylexStyles.rule2}>Shortcuts</h3>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!selectedLayerEditable || Boolean(capture)}
            onClick={() => startCapture('combo', selectedBindings.length > 0)}
          >
            <LuPlus />
            Record shortcut
          </Button>
        </div>
        {selectedBindings.length === 0 ? (
          <div sx={stylexStyles.rule3}>
            No keyboard shortcut. You can still run this command from the
            command palette or menu.
          </div>
        ) : (
          <div>
            {selectedBindings.map((binding) => (
              <div
                key={`${binding.sourceLayer}:${binding.bindingId}:${binding.display}`}
                sx={[stylexStyles.rule4, childSpacing.s2]}
              >
                <div sx={stylexStyles.rule5}>
                  <div sx={stylexStyles.rule6}>
                    {binding.display.split(' ').map(formatKeybinding).join(' ')}
                  </div>
                  <div sx={stylexStyles.rule7}>
                    {binding.sourceLayerLabel} ·{' '}
                    {focusContextLabel(binding.context)}
                  </div>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={
                    !selectedLayerEditable ||
                    !canLayerOverrideBinding(selectedScope, binding.sourceLayer)
                  }
                  onClick={() => removeBinding(binding)}
                >
                  Remove
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>

      {capture && (
        <div
          data-keybinding-recorder
          role="status"
          tabIndex={-1}
          aria-live="polite"
          sx={[stylexStyles.rule8, childSpacing.s4]}
        >
          <div sx={stylexStyles.rule9}>Recording shortcut</div>
          <div sx={stylexStyles.rule10}>
            {capture.steps.length > 0 &&
              `${capture.steps.map(formatKeybinding).join(' ')} `}
            {capture.heldModifiers.length > 0
              ? formatKeybinding(`${capture.heldModifiers.join('+')}+…`)
              : 'Press the keys you want to use'}
          </div>
          <div sx={stylexStyles.rule11}>
            Escape cancels. The first non-modifier key completes the shortcut.
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            sx={stylexStyles.rule12}
            onClick={cancelCapture}
          >
            Cancel
          </Button>
        </div>
      )}

      {pendingBinding && (
        <div sx={[stylexStyles.rule13, childSpacing.s4]}>
          <div sx={stylexStyles.rule14}>
            {pendingReplace ? 'Replace with' : 'Add'}
          </div>
          <div sx={stylexStyles.rule15}>
            {bindingDisplay(pendingBinding)
              .split(' ')
              .map(formatKeybinding)
              .join(' ')}
          </div>
          {pendingConflict && (
            <div sx={stylexStyles.rule16}>
              This shortcut is already used by{' '}
              {pendingConflict.bindings
                .flatMap((binding) =>
                  binding.commandId === selectedRow?.commandId
                    ? []
                    : [binding.label],
                )
                .join(', ')}
              .
            </div>
          )}
          <div sx={stylexStyles.rule17}>
            {!pendingConflict && (
              <Button type="button" size="sm" onClick={savePendingBinding}>
                Save shortcut
              </Button>
            )}
            {pendingConflict && (
              <Button
                type="button"
                size="sm"
                disabled={!pendingConflictReplaceable}
                onClick={replacePendingConflict}
              >
                {pendingConflictReplaceable
                  ? 'Replace existing shortcut'
                  : 'Choose another scope or shortcut'}
              </Button>
            )}
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={cancelCapture}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}

      {captureError && (
        <div sx={[stylexStyles.rule18, childSpacing.s4]}>{captureError}</div>
      )}

      <div sx={[stylexStyles.rule19, childSpacing.s4]}>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!selectedLayerEditable}
          onClick={() => startCapture('sequence', false)}
        >
          Add key sequence
        </Button>
        <Button
          type="button"
          variant="errorGhost"
          size="sm"
          disabled={!selectedLayerEditable}
          onClick={resetSelectedCommand}
        >
          <LuRotateCcw />
          Use inherited shortcuts
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={!selectedLayerEditable || selectedBindings.length === 0}
          onClick={clearSelectedBindings}
        >
          <LuTrash2 />
          Disable shortcuts
        </Button>
      </div>
    </div>
  )
}
