import { Button } from '@s4wave/web/ui/button.js'

import { useKeybindingEditorContext } from './KeybindingEditorContext.js'
import { scopeLabels } from './component.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { display: 'grid', gap: `calc(${themeLayout['--spacing']} * 3)` },
  rule2: {
    display: 'grid',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: { color: themeColors['--color-foreground-alt'] },
  rule4: {
    minHeight: `calc(${themeLayout['--spacing']} * 9)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: themeColors['--color-background'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-brand'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    minHeight: `calc(${themeLayout['--spacing']} * 9)`,
    minWidth: 0,
    flex: 1,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: themeColors['--color-background'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule7: { justifyContent: 'flex-start' },
})

export function KeybindingDiscoverySettings({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const {
    selectedController,
    selectedSettingsEditable,
    selectedScope,
    updateLeaderCombo,
    updateWhichKeyDelay,
  } = useKeybindingEditorContext()

  return (
    <div sx={[stylexStyles.rule1, sx]}>
      <label sx={stylexStyles.rule2}>
        <span sx={stylexStyles.rule3}>Leader key</span>
        <input
          aria-label="Leader combo"
          sx={stylexStyles.rule4}
          placeholder="Ctrl+Space"
          value={selectedController.overrideSet.settings.leaderCombo ?? ''}
          disabled={!selectedSettingsEditable}
          onChange={(event) => updateLeaderCombo(event.currentTarget.value)}
        />
      </label>
      <label sx={stylexStyles.rule2}>
        <span sx={stylexStyles.rule3}>Show key guide after</span>
        <span sx={stylexStyles.rule5}>
          <input
            aria-label="Which-key delay"
            sx={stylexStyles.rule6}
            min={0}
            placeholder="0"
            type="number"
            value={
              selectedController.overrideSet.settings.whichKeyDelayMs ?? ''
            }
            disabled={!selectedSettingsEditable}
            onChange={(event) => updateWhichKeyDelay(event.currentTarget.value)}
          />
          <span sx={stylexStyles.rule3}>ms</span>
        </span>
      </label>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        sx={stylexStyles.rule7}
        disabled={!selectedSettingsEditable}
        onClick={selectedController.resetLayer}
      >
        Reset {scopeLabels[selectedScope]} settings
      </Button>
    </div>
  )
}
