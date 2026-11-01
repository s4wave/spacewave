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

const stylexStyles = stylex.create({
  rule1: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule3: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: 'dashed',
    borderWidth: '1px',
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 5)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 12)`,
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card-alt']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: { minWidth: 0 },
  rule6: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-brand'],
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
  },
  rule9: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-brand'],
    textTransform: 'uppercase',
  },
  rule10: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    minHeight: `calc(${themeLayout['--spacing']} * 12)`,
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule11: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule12: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
  rule13: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule15: {
    marginTop: themeLayout['--spacing'],
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    color: themeColors['--color-brand'],
  },
  rule16: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
  rule17: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule18: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
  rule19: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export function KeybindingBindingsSection() {
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
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-4 */ 'space-y-4'
      }
    >
      <div>
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
          <div
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
            }
          >
            {selectedBindings.map((binding) => (
              <div
                key={`${binding.sourceLayer}:${binding.bindingId}:${binding.display}`}
                sx={stylexStyles.rule4}
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
          sx={stylexStyles.rule8}
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
        <div sx={stylexStyles.rule13}>
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

      {captureError && <div sx={stylexStyles.rule18}>{captureError}</div>}

      <div sx={stylexStyles.rule19}>
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
